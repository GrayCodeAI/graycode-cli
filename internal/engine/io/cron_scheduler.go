package io

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CronJob represents a scheduled recurring task.
type CronJob struct {
	ID         string
	Name       string
	Schedule   string
	Command    string
	Enabled    bool
	LastRun    *time.Time
	NextRun    *time.Time
	RunCount   int
	LastResult string
	LastError  string
	CreatedAt  time.Time
}

// CronScheduler manages scheduled cron jobs and executes them when due.
type CronScheduler struct {
	Jobs    map[string]*CronJob
	Running bool
	done    chan struct{}
	mu      sync.RWMutex
	nextID  int
}

// CronExpr holds parsed cron expression fields as integer slices.
type CronExpr struct {
	Minute     []int
	Hour       []int
	DayOfMonth []int
	Month      []int
	DayOfWeek  []int
}

// NewCronScheduler creates an initialized CronScheduler.
func NewCronScheduler() *CronScheduler {
	return &CronScheduler{
		Jobs: make(map[string]*CronJob),
	}
}

// AddJob parses the schedule expression and registers a new job.
func (cs *CronScheduler) AddJob(name, schedule, command string) (*CronJob, error) {
	expr, err := ParseCron(schedule)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.nextID++
	id := fmt.Sprintf("job_%d", cs.nextID)

	now := time.Now()
	next := NextRunTime(expr, now)

	job := &CronJob{
		ID:        id,
		Name:      name,
		Schedule:  schedule,
		Command:   command,
		Enabled:   true,
		NextRun:   &next,
		CreatedAt: now,
	}

	cs.Jobs[id] = job
	return job, nil
}

// RemoveJob deletes a job by ID.
func (cs *CronScheduler) RemoveJob(id string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if _, exists := cs.Jobs[id]; !exists {
		return fmt.Errorf("job not found: %s", id)
	}
	delete(cs.Jobs, id)
	return nil
}

// Start launches a background goroutine that checks for due jobs every minute.
func (cs *CronScheduler) Start(ctx context.Context, execFn func(string) (string, error)) {
	cs.mu.Lock()
	if cs.Running {
		cs.mu.Unlock()
		return
	}
	cs.Running = true
	cs.done = make(chan struct{})
	cs.mu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				cs.mu.Lock()
				cs.Running = false
				cs.mu.Unlock()
				close(cs.done)
				return
			case <-cs.done:
				return
			case now := <-ticker.C:
				cs.runDueJobs(now, execFn)
			}
		}
	}()
}

// Stop halts the scheduler.
func (cs *CronScheduler) Stop() {
	cs.mu.Lock()
	if !cs.Running {
		cs.mu.Unlock()
		return
	}
	cs.Running = false
	cs.mu.Unlock()
	close(cs.done)
}

// PauseJob disables a job so it won't be executed.
func (cs *CronScheduler) PauseJob(id string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	job, exists := cs.Jobs[id]
	if !exists {
		return fmt.Errorf("job not found: %s", id)
	}
	job.Enabled = false
	return nil
}

// ResumeJob re-enables a paused job and recalculates its next run time.
func (cs *CronScheduler) ResumeJob(id string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	job, exists := cs.Jobs[id]
	if !exists {
		return fmt.Errorf("job not found: %s", id)
	}
	job.Enabled = true

	expr, err := ParseCron(job.Schedule)
	if err == nil {
		next := NextRunTime(expr, time.Now())
		job.NextRun = &next
	}
	return nil
}

// ListJobs returns all jobs as a slice.
func (cs *CronScheduler) ListJobs() []*CronJob {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	jobs := make([]*CronJob, 0, len(cs.Jobs))
	for _, job := range cs.Jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// FormatJobs returns a human-readable representation of all scheduled jobs.
func (cs *CronScheduler) FormatJobs() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if len(cs.Jobs) == 0 {
		return "No scheduled jobs."
	}

	var b strings.Builder
	b.WriteString("Scheduled Jobs:\n")
	b.WriteString("─────────────────────────────────\n")

	i := 0
	for _, job := range cs.Jobs {
		i++
		status := "active"
		if !job.Enabled {
			status = "paused"
		}

		b.WriteString(fmt.Sprintf("%d. [%s] %q (%s)\n", i, status, job.Name, job.Schedule))

		if !job.Enabled {
			b.WriteString("   Paused\n")
		} else {
			nextStr := "never"
			if job.NextRun != nil {
				nextStr = job.NextRun.Format("Mon 15:04")
			}
			lastStr := "never"
			if job.LastRun != nil {
				result := "success"
				if job.LastError != "" {
					result = "error"
				} else if job.LastResult != "" {
					result = job.LastResult
				}
				lastStr = fmt.Sprintf("%s (%s)", job.LastRun.Format("Mon 15:04"), result)
			}
			b.WriteString(fmt.Sprintf("   Next: %s, Last: %s\n", nextStr, lastStr))
		}

		if i < len(cs.Jobs) {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// IsDue returns true if the given job should run at the specified time.
func IsDue(job *CronJob, now time.Time) bool {
	if !job.Enabled {
		return false
	}
	if job.NextRun == nil {
		return false
	}
	return !now.Before(*job.NextRun)
}

// runDueJobs checks all jobs and runs those that are due.
func (cs *CronScheduler) runDueJobs(now time.Time, execFn func(string) (string, error)) {
	cs.mu.RLock()
	var dueJobs []*CronJob
	for _, job := range cs.Jobs {
		if IsDue(job, now) {
			dueJobs = append(dueJobs, job)
		}
	}
	cs.mu.RUnlock()

	for _, job := range dueJobs {
		result, err := execFn(job.Command)

		cs.mu.Lock()
		runTime := now
		job.LastRun = &runTime
		job.RunCount++
		job.LastResult = result
		if err != nil {
			job.LastError = err.Error()
		} else {
			job.LastError = ""
		}

		// Calculate next run
		expr, parseErr := ParseCron(job.Schedule)
		if parseErr == nil {
			next := NextRunTime(expr, now)
			job.NextRun = &next
		}
		cs.mu.Unlock()
	}
}

// ParseCron parses a standard 5-field cron expression.
// Fields: minute hour day-of-month month day-of-week
// Supports: * (any), */N (step), N-M (range), N,M,O (list)
func ParseCron(expression string) (*CronExpr, error) {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return nil, fmt.Errorf("expected 5 fields, got %d", len(fields))
	}

	minute, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute field: %w", err)
	}

	hour, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}

	dom, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day-of-month field: %w", err)
	}

	month, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month field: %w", err)
	}

	dow, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("day-of-week field: %w", err)
	}

	return &CronExpr{
		Minute:     minute,
		Hour:       hour,
		DayOfMonth: dom,
		Month:      month,
		DayOfWeek:  dow,
	}, nil
}

// parseCronField parses a single cron field into a list of valid integers.
func parseCronField(field string, min, max int) ([]int, error) {
	var result []int

	parts := strings.Split(field, ",")
	for _, part := range parts {
		vals, err := parseCronPart(part, min, max)
		if err != nil {
			return nil, err
		}
		result = append(result, vals...)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("empty field")
	}

	return result, nil
}

// parseCronPart handles a single segment: *, */N, N-M, N-M/S, or a plain number.
func parseCronPart(part string, min, max int) ([]int, error) {
	// Handle step notation
	var stepStr string
	if idx := strings.Index(part, "/"); idx >= 0 {
		stepStr = part[idx+1:]
		part = part[:idx]
	}

	var rangeStart, rangeEnd int

	if part == "*" {
		rangeStart = min
		rangeEnd = max
	} else if strings.Contains(part, "-") {
		bounds := strings.SplitN(part, "-", 2)
		var err error
		rangeStart, err = strconv.Atoi(bounds[0])
		if err != nil {
			return nil, fmt.Errorf("invalid number: %s", bounds[0])
		}
		rangeEnd, err = strconv.Atoi(bounds[1])
		if err != nil {
			return nil, fmt.Errorf("invalid number: %s", bounds[1])
		}
		if rangeStart < min || rangeEnd > max || rangeStart > rangeEnd {
			return nil, fmt.Errorf("range %d-%d out of bounds [%d,%d]", rangeStart, rangeEnd, min, max)
		}
	} else {
		// Plain number
		val, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid number: %s", part)
		}
		if val < min || val > max {
			return nil, fmt.Errorf("value %d out of bounds [%d,%d]", val, min, max)
		}
		if stepStr == "" {
			return []int{val}, nil
		}
		rangeStart = val
		rangeEnd = max
	}

	step := 1
	if stepStr != "" {
		var err error
		step, err = strconv.Atoi(stepStr)
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step: %s", stepStr)
		}
	}

	var result []int
	for i := rangeStart; i <= rangeEnd; i += step {
		result = append(result, i)
	}
	return result, nil
}

// NextRunTime calculates the next time after `after` that matches the cron expression.
func NextRunTime(expr *CronExpr, after time.Time) time.Time {
	// Start from the next minute
	t := after.Truncate(time.Minute).Add(time.Minute)

	// Search up to 366 days ahead to avoid infinite loop
	limit := t.Add(366 * 24 * time.Hour)

	for t.Before(limit) {
		if matchesCron(expr, t) {
			return t
		}
		t = t.Add(time.Minute)
	}

	// Fallback: should not happen with valid expressions
	return after.Add(time.Hour)
}

// matchesCron checks if a given time matches all fields of the cron expression.
func matchesCron(expr *CronExpr, t time.Time) bool {
	if !containsInt(expr.Minute, t.Minute()) {
		return false
	}
	if !containsInt(expr.Hour, t.Hour()) {
		return false
	}
	if !containsInt(expr.Month, int(t.Month())) {
		return false
	}
	// Day matching: standard cron uses OR logic between day-of-month and day-of-week
	// when both are restricted (not *). We simplify: both must match.
	if !containsInt(expr.DayOfMonth, t.Day()) {
		return false
	}
	dow := int(t.Weekday()) // Sunday=0
	if !containsInt(expr.DayOfWeek, dow) {
		return false
	}
	return true
}

// containsInt checks if a value exists in a slice.
func containsInt(s []int, val int) bool {
	for _, v := range s {
		if v == val {
			return true
		}
	}
	return false
}
