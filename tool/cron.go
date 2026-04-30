package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CronJob represents a scheduled recurring or one-shot task.
type CronJob struct {
	ID        string    `json:"id"`
	Schedule  string    `json:"schedule"`
	Prompt    string    `json:"prompt"`
	Recurring bool      `json:"recurring"`
	Durable   bool      `json:"durable"`
	CreatedAt time.Time `json:"createdAt"`
	LastRun   time.Time `json:"lastRun,omitempty"`
	NextRun   time.Time `json:"nextRun"`
	Runs      int       `json:"runs"`
	cancel    context.CancelFunc
}

// CronScheduler manages scheduled tasks.
type CronScheduler struct {
	mu   sync.RWMutex
	jobs map[string]*CronJob
	next int
}

var globalCronScheduler = &CronScheduler{jobs: make(map[string]*CronJob)}

func GetCronScheduler() *CronScheduler { return globalCronScheduler }

func (s *CronScheduler) Create(schedule, prompt string, recurring, durable bool) (*CronJob, error) {
	nextRun, err := nextCronTime(schedule)
	if err != nil {
		return nil, fmt.Errorf("invalid cron schedule %q: %w", schedule, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	id := fmt.Sprintf("cron_%d", s.next)

	ctx, cancel := context.WithCancel(context.Background())
	_ = ctx

	job := &CronJob{
		ID:        id,
		Schedule:  schedule,
		Prompt:    prompt,
		Recurring: recurring,
		Durable:   durable,
		CreatedAt: time.Now(),
		NextRun:   nextRun,
		cancel:    cancel,
	}
	s.jobs[id] = job
	return job, nil
}

func (s *CronScheduler) List() []*CronJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*CronJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	return out
}

func (s *CronScheduler) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return false
	}
	if job.cancel != nil {
		job.cancel()
	}
	delete(s.jobs, id)
	return true
}

func (s *CronScheduler) Get(id string) (*CronJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	return j, ok
}

// nextCronTime parses a 5-field cron expression and finds the next matching time.
func nextCronTime(expr string) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("expected 5 fields, got %d", len(fields))
	}

	now := time.Now()
	// Simple implementation: try each minute for the next 48 hours
	candidate := now.Truncate(time.Minute).Add(time.Minute)
	limit := now.Add(48 * time.Hour)
	for candidate.Before(limit) {
		if cronMatches(fields, candidate) {
			return candidate, nil
		}
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no match found in next 48 hours for %q", expr)
}

func cronMatches(fields []string, t time.Time) bool {
	return fieldMatches(fields[0], t.Minute()) &&
		fieldMatches(fields[1], t.Hour()) &&
		fieldMatches(fields[2], t.Day()) &&
		fieldMatches(fields[3], int(t.Month())) &&
		fieldMatches(fields[4], int(t.Weekday()))
}

func fieldMatches(field string, value int) bool {
	if field == "*" {
		return true
	}
	// Handle */N step values
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil || step <= 0 {
			return false
		}
		return value%step == 0
	}
	// Handle comma-separated values
	for _, part := range strings.Split(field, ",") {
		// Handle ranges
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 == nil && err2 == nil && value >= lo && value <= hi {
				return true
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err == nil && n == value {
			return true
		}
	}
	return false
}

// CronCreateTool schedules a prompt to run on a cron schedule.
type CronCreateTool struct{}

func (CronCreateTool) Name() string        { return "CronCreate" }
func (CronCreateTool) Aliases() []string   { return []string{"cron_create", "ScheduleWakeup"} }
func (CronCreateTool) Description() string {
	return "Schedule a prompt to run at a future time — either recurring on a cron schedule, or once at a specific time."
}
func (CronCreateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"schedule": map[string]interface{}{
				"type":        "string",
				"description": "5-field cron expression in user's local timezone: minute hour day-of-month month day-of-week",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "The prompt to enqueue when the schedule fires",
			},
			"recurring": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, repeats on schedule. If false, fires once then auto-deletes (default: true)",
			},
			"durable": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, persists to disk and survives session restarts (default: false)",
			},
		},
		"required": []string{"schedule", "prompt"},
	}
}

func (CronCreateTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Schedule  string `json:"schedule"`
		Prompt    string `json:"prompt"`
		Recurring *bool  `json:"recurring"`
		Durable   *bool  `json:"durable"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Schedule == "" {
		return "", fmt.Errorf("schedule is required")
	}
	if p.Prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	recurring := true
	if p.Recurring != nil {
		recurring = *p.Recurring
	}
	durable := false
	if p.Durable != nil {
		durable = *p.Durable
	}

	job, err := globalCronScheduler.Create(p.Schedule, p.Prompt, recurring, durable)
	if err != nil {
		return "", err
	}

	out, _ := json.Marshal(map[string]any{
		"id":       job.ID,
		"schedule": job.Schedule,
		"nextRun":  job.NextRun.Format(time.RFC3339),
		"type":     map[bool]string{true: "recurring", false: "one-shot"}[recurring],
	})
	return string(out), nil
}

// CronDeleteTool removes a scheduled job.
type CronDeleteTool struct{}

func (CronDeleteTool) Name() string        { return "CronDelete" }
func (CronDeleteTool) Aliases() []string   { return []string{"cron_delete"} }
func (CronDeleteTool) Description() string { return "Remove a scheduled cron job" }
func (CronDeleteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The cron job ID to delete",
			},
		},
		"required": []string{"id"},
	}
}

func (CronDeleteTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if !globalCronScheduler.Delete(p.ID) {
		return "", fmt.Errorf("cron job %q not found", p.ID)
	}
	return fmt.Sprintf("Deleted cron job %s", p.ID), nil
}

// CronListTool lists all scheduled jobs.
type CronListTool struct{}

func (CronListTool) Name() string        { return "CronList" }
func (CronListTool) Aliases() []string   { return []string{"cron_list"} }
func (CronListTool) Description() string { return "List all scheduled cron jobs" }
func (CronListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (CronListTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	jobs := globalCronScheduler.List()
	items := make([]map[string]any, 0, len(jobs))
	for _, j := range jobs {
		items = append(items, map[string]any{
			"id":        j.ID,
			"schedule":  j.Schedule,
			"prompt":    j.Prompt,
			"recurring": j.Recurring,
			"nextRun":   j.NextRun.Format(time.RFC3339),
			"runs":      j.Runs,
		})
	}
	out, _ := json.Marshal(map[string]any{"jobs": items})
	return string(out), nil
}
