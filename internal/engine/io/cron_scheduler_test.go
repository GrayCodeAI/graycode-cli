package io

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewCronScheduler(t *testing.T) {
	cs := NewCronScheduler()
	if cs == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if cs.Jobs == nil {
		t.Fatal("expected initialized Jobs map")
	}
	if cs.Running {
		t.Fatal("expected scheduler not running initially")
	}
}

func TestAddJob(t *testing.T) {
	cs := NewCronScheduler()

	job, err := cs.AddJob("Run tests", "*/5 * * * *", "go test ./...")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.ID == "" {
		t.Error("expected non-empty job ID")
	}
	if job.Name != "Run tests" {
		t.Errorf("expected name 'Run tests', got %q", job.Name)
	}
	if job.Schedule != "*/5 * * * *" {
		t.Errorf("expected schedule '*/5 * * * *', got %q", job.Schedule)
	}
	if job.Command != "go test ./..." {
		t.Errorf("expected command 'go test ./...', got %q", job.Command)
	}
	if !job.Enabled {
		t.Error("expected job to be enabled by default")
	}
	if job.NextRun == nil {
		t.Error("expected NextRun to be set")
	}
	if job.RunCount != 0 {
		t.Errorf("expected RunCount 0, got %d", job.RunCount)
	}
}

func TestAddJob_InvalidExpression(t *testing.T) {
	cs := NewCronScheduler()

	_, err := cs.AddJob("Bad job", "invalid", "echo hi")
	if err == nil {
		t.Fatal("expected error for invalid cron expression")
	}
}

func TestRemoveJob(t *testing.T) {
	cs := NewCronScheduler()

	job, _ := cs.AddJob("Temp job", "0 * * * *", "echo temp")
	err := cs.RemoveJob(job.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cs.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(cs.Jobs))
	}
}

func TestRemoveJob_NotFound(t *testing.T) {
	cs := NewCronScheduler()

	err := cs.RemoveJob("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestPauseResumeJob(t *testing.T) {
	cs := NewCronScheduler()

	job, _ := cs.AddJob("Pausable", "*/10 * * * *", "echo hi")

	err := cs.PauseJob(job.ID)
	if err != nil {
		t.Fatalf("unexpected pause error: %v", err)
	}
	if job.Enabled {
		t.Error("expected job to be disabled after pause")
	}

	err = cs.ResumeJob(job.ID)
	if err != nil {
		t.Fatalf("unexpected resume error: %v", err)
	}
	if !job.Enabled {
		t.Error("expected job to be enabled after resume")
	}
}

func TestPauseJob_NotFound(t *testing.T) {
	cs := NewCronScheduler()
	err := cs.PauseJob("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResumeJob_NotFound(t *testing.T) {
	cs := NewCronScheduler()
	err := cs.ResumeJob("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListJobs(t *testing.T) {
	cs := NewCronScheduler()

	cs.AddJob("Job A", "0 * * * *", "echo a")
	cs.AddJob("Job B", "0 0 * * *", "echo b")

	jobs := cs.ListJobs()
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestFormatJobs_Empty(t *testing.T) {
	cs := NewCronScheduler()
	output := cs.FormatJobs()
	if output != "No scheduled jobs." {
		t.Errorf("unexpected output for empty: %q", output)
	}
}

func TestFormatJobs_WithJobs(t *testing.T) {
	cs := NewCronScheduler()

	cs.AddJob("Run tests", "*/30 * * * *", "go test")
	job2, _ := cs.AddJob("Backup", "0 0 * * *", "backup.sh")
	cs.PauseJob(job2.ID)

	output := cs.FormatJobs()
	if !strings.Contains(output, "Scheduled Jobs:") {
		t.Error("expected header in output")
	}
	if !strings.Contains(output, "[active]") {
		t.Error("expected [active] status")
	}
	if !strings.Contains(output, "[paused]") {
		t.Error("expected [paused] status")
	}
	if !strings.Contains(output, "Paused") {
		t.Error("expected Paused line for disabled job")
	}
}

func TestIsDue(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 30, 0, 0, time.UTC)

	// Job with NextRun in the past: should be due
	pastTime := now.Add(-time.Minute)
	jobDue := &CronJob{
		Enabled: true,
		NextRun: &pastTime,
	}
	if !IsDue(jobDue, now) {
		t.Error("expected job to be due")
	}

	// Job with NextRun in the future: should not be due
	futureTime := now.Add(time.Minute)
	jobNotDue := &CronJob{
		Enabled: true,
		NextRun: &futureTime,
	}
	if IsDue(jobNotDue, now) {
		t.Error("expected job to not be due")
	}

	// Disabled job: never due
	jobDisabled := &CronJob{
		Enabled: false,
		NextRun: &pastTime,
	}
	if IsDue(jobDisabled, now) {
		t.Error("expected disabled job to not be due")
	}

	// Job with nil NextRun: not due
	jobNil := &CronJob{
		Enabled: true,
		NextRun: nil,
	}
	if IsDue(jobNil, now) {
		t.Error("expected job with nil NextRun to not be due")
	}
}

func TestParseCron_EveryFiveMinutes(t *testing.T) {
	expr, err := ParseCron("*/5 * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 12 minute values: 0, 5, 10, ..., 55
	if len(expr.Minute) != 12 {
		t.Errorf("expected 12 minute values, got %d: %v", len(expr.Minute), expr.Minute)
	}
	if expr.Minute[0] != 0 || expr.Minute[11] != 55 {
		t.Errorf("unexpected minute values: %v", expr.Minute)
	}

	// Hour should have all 24
	if len(expr.Hour) != 24 {
		t.Errorf("expected 24 hour values, got %d", len(expr.Hour))
	}
}

func TestParseCron_EveryTwoHours(t *testing.T) {
	expr, err := ParseCron("0 */2 * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(expr.Minute) != 1 || expr.Minute[0] != 0 {
		t.Errorf("expected minute [0], got %v", expr.Minute)
	}

	// 0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22
	if len(expr.Hour) != 12 {
		t.Errorf("expected 12 hour values, got %d: %v", len(expr.Hour), expr.Hour)
	}
}

func TestParseCron_Weekdays9AM(t *testing.T) {
	expr, err := ParseCron("0 9 * * 1-5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(expr.Minute) != 1 || expr.Minute[0] != 0 {
		t.Errorf("expected minute [0], got %v", expr.Minute)
	}
	if len(expr.Hour) != 1 || expr.Hour[0] != 9 {
		t.Errorf("expected hour [9], got %v", expr.Hour)
	}
	if len(expr.DayOfWeek) != 5 {
		t.Errorf("expected 5 weekday values, got %d: %v", len(expr.DayOfWeek), expr.DayOfWeek)
	}
	// 1=Mon through 5=Fri
	expected := []int{1, 2, 3, 4, 5}
	for i, v := range expected {
		if expr.DayOfWeek[i] != v {
			t.Errorf("expected day %d, got %d", v, expr.DayOfWeek[i])
		}
	}
}

func TestParseCron_Lists(t *testing.T) {
	expr, err := ParseCron("0,15,30,45 * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Minute) != 4 {
		t.Errorf("expected 4 minute values, got %d: %v", len(expr.Minute), expr.Minute)
	}
}

func TestParseCron_Invalid(t *testing.T) {
	tests := []string{
		"",
		"* * *",
		"60 * * * *",
		"* 25 * * *",
		"* * * * 8",
		"abc * * * *",
	}

	for _, tc := range tests {
		_, err := ParseCron(tc)
		if err == nil {
			t.Errorf("expected error for expression %q", tc)
		}
	}
}

func TestNextRunTime_EveryFiveMinutes(t *testing.T) {
	expr, _ := ParseCron("*/5 * * * *")

	// If we're at 10:03, next should be 10:05
	after := time.Date(2026, 5, 13, 10, 3, 0, 0, time.UTC)
	next := NextRunTime(expr, after)

	expected := time.Date(2026, 5, 13, 10, 5, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestNextRunTime_SpecificHour(t *testing.T) {
	expr, _ := ParseCron("0 9 * * *")

	// At 10:00, next 9:00 should be tomorrow
	after := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	next := NextRunTime(expr, after)

	expected := time.Date(2026, 5, 14, 9, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestNextRunTime_Weekday(t *testing.T) {
	// 2026-05-13 is a Wednesday (weekday=3)
	expr, _ := ParseCron("0 9 * * 1") // Monday only

	after := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC) // Wednesday 10:00
	next := NextRunTime(expr, after)

	// Next Monday is May 18
	expected := time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestStartStop(t *testing.T) {
	cs := NewCronScheduler()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	execFn := func(cmd string) (string, error) {
		return "ok", nil
	}

	cs.Start(ctx, execFn)
	if !cs.Running {
		t.Error("expected scheduler to be running")
	}

	cs.Stop()
	// Give goroutine time to finish
	time.Sleep(10 * time.Millisecond)

	cs.mu.RLock()
	running := cs.Running
	cs.mu.RUnlock()
	if running {
		t.Error("expected scheduler to be stopped")
	}
}

func TestStartStop_ContextCancel(t *testing.T) {
	cs := NewCronScheduler()

	ctx, cancel := context.WithCancel(context.Background())

	execFn := func(cmd string) (string, error) {
		return "ok", nil
	}

	cs.Start(ctx, execFn)
	cancel()

	// Give goroutine time to process context cancellation
	time.Sleep(100 * time.Millisecond)

	cs.mu.RLock()
	running := cs.Running
	cs.mu.RUnlock()
	if running {
		t.Error("expected scheduler to stop on context cancel")
	}
}

func TestRunDueJobs(t *testing.T) {
	cs := NewCronScheduler()

	var mu sync.Mutex
	var executed []string

	execFn := func(cmd string) (string, error) {
		mu.Lock()
		executed = append(executed, cmd)
		mu.Unlock()
		return "done", nil
	}

	// Add a job and set its NextRun to the past
	job, _ := cs.AddJob("Test job", "*/5 * * * *", "echo hello")
	past := time.Now().Add(-time.Minute)
	cs.mu.Lock()
	job.NextRun = &past
	cs.mu.Unlock()

	// Trigger due jobs
	cs.runDueJobs(time.Now(), execFn)

	mu.Lock()
	defer mu.Unlock()
	if len(executed) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(executed))
	}
	if executed[0] != "echo hello" {
		t.Errorf("expected 'echo hello', got %q", executed[0])
	}
	if job.RunCount != 1 {
		t.Errorf("expected RunCount 1, got %d", job.RunCount)
	}
	if job.LastResult != "done" {
		t.Errorf("expected LastResult 'done', got %q", job.LastResult)
	}
	if job.LastRun == nil {
		t.Error("expected LastRun to be set")
	}
}

func TestRunDueJobs_SkipsPausedJobs(t *testing.T) {
	cs := NewCronScheduler()

	var executed int
	execFn := func(cmd string) (string, error) {
		executed++
		return "ok", nil
	}

	job, _ := cs.AddJob("Paused job", "*/5 * * * *", "echo hi")
	past := time.Now().Add(-time.Minute)
	cs.mu.Lock()
	job.NextRun = &past
	cs.mu.Unlock()

	cs.PauseJob(job.ID)
	cs.runDueJobs(time.Now(), execFn)

	if executed != 0 {
		t.Errorf("expected 0 executions for paused job, got %d", executed)
	}
}

func TestRunDueJobs_ErrorHandling(t *testing.T) {
	cs := NewCronScheduler()

	execFn := func(cmd string) (string, error) {
		return "", fmt.Errorf("command failed")
	}

	job, _ := cs.AddJob("Failing job", "*/5 * * * *", "bad command")
	past := time.Now().Add(-time.Minute)
	cs.mu.Lock()
	job.NextRun = &past
	cs.mu.Unlock()

	cs.runDueJobs(time.Now(), execFn)

	if job.LastError != "command failed" {
		t.Errorf("expected LastError 'command failed', got %q", job.LastError)
	}
	if job.RunCount != 1 {
		t.Errorf("expected RunCount 1, got %d", job.RunCount)
	}
}

func TestMultipleJobs_UniqueIDs(t *testing.T) {
	cs := NewCronScheduler()

	job1, _ := cs.AddJob("Job 1", "0 * * * *", "cmd1")
	job2, _ := cs.AddJob("Job 2", "0 * * * *", "cmd2")
	job3, _ := cs.AddJob("Job 3", "0 * * * *", "cmd3")

	if job1.ID == job2.ID || job2.ID == job3.ID || job1.ID == job3.ID {
		t.Errorf("job IDs should be unique: %s, %s, %s", job1.ID, job2.ID, job3.ID)
	}
}
