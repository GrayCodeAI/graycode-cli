package cron

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/eventlog"
)

type ScheduleKind string

const (
	ScheduleAt    ScheduleKind = "at"
	ScheduleEvery ScheduleKind = "every"
)

type Schedule struct {
	Kind  ScheduleKind  `json:"kind"`
	At    *time.Time    `json:"at,omitempty"`
	Every time.Duration `json:"every,omitempty"`
}

type SessionTarget string

const (
	SessionMain     SessionTarget = "main"
	SessionIsolated SessionTarget = "isolated"
)

type Job struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	Description       string        `json:"description,omitempty"`
	Enabled           bool          `json:"enabled"`
	Schedule          Schedule      `json:"schedule"`
	SessionTarget     SessionTarget `json:"session_target"`
	Payload           string        `json:"payload"`
	DeleteAfterRun    bool          `json:"delete_after_run,omitempty"`
	ConsecutiveErrors int           `json:"consecutive_errors"`
	LastRunAt         *time.Time    `json:"last_run_at,omitempty"`
	LastStatus        RunStatus     `json:"last_status,omitempty"`
	NextRunAt         *time.Time    `json:"next_run_at,omitempty"`
	MaxRetries        int           `json:"max_retries"`
	CreatedAt         time.Time     `json:"created_at"`
}

type RunStatus string

const (
	StatusOK      RunStatus = "ok"
	StatusError   RunStatus = "error"
	StatusSkipped RunStatus = "skipped"
)

type RunRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	JobID      string    `json:"job_id"`
	Status     RunStatus `json:"status"`
	DurationMs int64     `json:"duration_ms"`
	Error      string    `json:"error,omitempty"`
}

type JobHandler func(job *Job) error

type Engine struct {
	mu            sync.RWMutex
	jobs          map[string]*Job
	handler       JobHandler
	running       bool
	stopCh        chan struct{}
	maxConcurrent int
	inFlight      int
	runs          []RunRecord
	// inFlightWG tracks in-flight job goroutines so Stop() can drain them.
	inFlightWG sync.WaitGroup
	// journal is optional; when set, job lifecycle changes emit schedule.change
	// events (DSH schedule.change seam).
	journal *eventlog.Log
}

// SetJournal attaches the append-only event spine for schedule.change emission.
// Nil-safe; calling with nil is a no-op.
func (e *Engine) SetJournal(j *eventlog.Log) {
	if e == nil {
		return
	}
	e.journal = j
}

// scheduleString renders a job's schedule as a descriptive string for logging.
func scheduleString(job *Job) string {
	switch job.Schedule.Kind {
	case ScheduleEvery:
		return "every " + job.Schedule.Every.String()
	case ScheduleAt:
		if job.Schedule.At != nil {
			return "at " + job.Schedule.At.Format(time.RFC3339)
		}
		return "at <none>"
	default:
		return "unknown"
	}
}

// emitScheduleChange emits a schedule.change event if a journal is attached.
func (e *Engine) emitScheduleChange(job *Job, action string) {
	if e == nil || e.journal == nil || job == nil {
		return
	}
	e.journal.AppendScheduleChange(job.Name + " (" + action + ") " + scheduleString(job))
}

func NewEngine(handler JobHandler, maxConcurrent int) *Engine {
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}
	return &Engine{
		jobs:          make(map[string]*Job),
		handler:       handler,
		stopCh:        make(chan struct{}),
		maxConcurrent: maxConcurrent,
		runs:          make([]RunRecord, 0),
	}
}

func (e *Engine) AddJob(job *Job) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if job.ID == "" {
		return fmt.Errorf("job ID required")
	}
	job.CreatedAt = time.Now()
	e.computeNextRun(job)
	e.jobs[job.ID] = job
	e.emitScheduleChange(job, "added")
	return nil
}

func (e *Engine) RemoveJob(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if job, ok := e.jobs[id]; ok {
		e.emitScheduleChange(job, "removed")
		delete(e.jobs, id)
	}
}

func (e *Engine) EnableJob(id string, enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if job, ok := e.jobs[id]; ok {
		action := "disabled"
		if enabled {
			action = "enabled"
		}
		job.Enabled = enabled
		if enabled {
			e.computeNextRun(job)
		}
		e.emitScheduleChange(job, action)
	}
}

func (e *Engine) ListJobs() []*Job {
	e.mu.RLock()
	defer e.mu.RUnlock()
	jobs := make([]*Job, 0, len(e.jobs))
	for _, j := range e.jobs {
		jobs = append(jobs, j)
	}
	return jobs
}

func (e *Engine) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.stopCh = make(chan struct{})
	e.mu.Unlock()

	go e.loop()
}

func (e *Engine) Stop() {
	e.mu.Lock()
	if e.running {
		close(e.stopCh)
		e.running = false
	}
	e.mu.Unlock()
	// Wait for in-flight jobs to finish so handlers are not killed mid-
	// execution (which would leave state mutations incomplete).
	e.inFlightWG.Wait()
}

func (e *Engine) Status() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	enabled := 0
	for _, j := range e.jobs {
		if j.Enabled {
			enabled++
		}
	}

	return map[string]interface{}{
		"running":      e.running,
		"total_jobs":   len(e.jobs),
		"enabled_jobs": enabled,
		"in_flight":    e.inFlight,
	}
}

func (e *Engine) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case now := <-ticker.C:
			e.tick(now)
		}
	}
}

func (e *Engine) tick(now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, job := range e.jobs {
		if !job.Enabled || job.NextRunAt == nil {
			continue
		}
		if now.Before(*job.NextRunAt) {
			continue
		}
		if e.inFlight >= e.maxConcurrent {
			break
		}
		if job.ConsecutiveErrors >= job.MaxRetries && job.MaxRetries > 0 {
			job.Enabled = false
			continue
		}

		e.inFlight++
		e.inFlightWG.Add(1)
		go e.executeJob(job)
	}
}

func (e *Engine) executeJob(job *Job) {
	defer e.inFlightWG.Done()
	start := time.Now()
	err := e.handler(job)
	duration := time.Since(start)

	e.mu.Lock()
	defer e.mu.Unlock()

	e.inFlight--
	now := time.Now()
	job.LastRunAt = &now

	record := RunRecord{
		Timestamp:  now,
		JobID:      job.ID,
		DurationMs: duration.Milliseconds(),
	}

	if err != nil {
		job.LastStatus = StatusError
		job.ConsecutiveErrors++
		record.Status = StatusError
		record.Error = err.Error()
	} else {
		job.LastStatus = StatusOK
		job.ConsecutiveErrors = 0
		record.Status = StatusOK
	}

	e.runs = append(e.runs, record)
	if len(e.runs) > 1000 {
		e.runs = e.runs[len(e.runs)-500:]
	}

	if job.DeleteAfterRun {
		delete(e.jobs, job.ID)
	} else {
		e.computeNextRun(job)
	}
}

func (e *Engine) computeNextRun(job *Job) {
	now := time.Now()
	switch job.Schedule.Kind {
	case ScheduleAt:
		if job.Schedule.At != nil && job.Schedule.At.After(now) {
			job.NextRunAt = job.Schedule.At
		} else {
			job.NextRunAt = nil
			job.Enabled = false
		}
	case ScheduleEvery:
		jitter := time.Duration(rand.IntN(5)) * time.Second // #nosec G404 -- non-cryptographic use (jitter for cron schedule spacing)
		next := now.Add(job.Schedule.Every + jitter)
		job.NextRunAt = &next
	}
}

func (e *Engine) MarshalJSON() ([]byte, error) {
	e.mu.RLock()
	jobs := make([]*Job, 0, len(e.jobs))
	for _, j := range e.jobs {
		jobs = append(jobs, j)
	}
	e.mu.RUnlock()
	return json.Marshal(struct {
		Jobs []*Job `json:"jobs"`
	}{Jobs: jobs})
}
