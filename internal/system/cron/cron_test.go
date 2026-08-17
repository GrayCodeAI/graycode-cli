package cron

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/eventlog"
)

func TestNewEngineDefaults(t *testing.T) {
	e := NewEngine(nil, 0)
	if e.maxConcurrent != 3 {
		t.Errorf("maxConcurrent = %d, want 3 (default)", e.maxConcurrent)
	}
}

func TestAddJobRequiresID(t *testing.T) {
	e := NewEngine(nil, 3)
	if err := e.AddJob(&Job{}); err == nil {
		t.Error("AddJob with empty ID should return error")
	}
}

func TestAddAndListJobs(t *testing.T) {
	e := NewEngine(nil, 3)
	e.AddJob(&Job{ID: "j1", Name: "Job 1", Enabled: true})
	e.AddJob(&Job{ID: "j2", Name: "Job 2", Enabled: false})

	jobs := e.ListJobs()
	if len(jobs) != 2 {
		t.Fatalf("ListJobs returned %d, want 2", len(jobs))
	}
}

func TestRemoveJob(t *testing.T) {
	e := NewEngine(nil, 3)
	e.AddJob(&Job{ID: "j1", Name: "Job 1"})
	e.RemoveJob("j1")

	if len(e.ListJobs()) != 0 {
		t.Error("expected 0 jobs after remove")
	}
}

func TestEnableJob(t *testing.T) {
	e := NewEngine(nil, 3)
	e.AddJob(&Job{
		ID:       "j1",
		Enabled:  false,
		Schedule: Schedule{Kind: ScheduleEvery, Every: time.Hour},
	})

	e.EnableJob("j1", true)
	jobs := e.ListJobs()
	if len(jobs) != 1 || !jobs[0].Enabled {
		t.Error("job should be enabled after EnableJob")
	}

	e.EnableJob("j1", false)
	jobs = e.ListJobs()
	if len(jobs) != 1 || jobs[0].Enabled {
		t.Error("job should be disabled after EnableJob(false)")
	}
}

func TestStatus(t *testing.T) {
	e := NewEngine(nil, 3)
	e.AddJob(&Job{ID: "j1", Enabled: true})
	e.AddJob(&Job{ID: "j2", Enabled: false})

	s := e.Status()
	if s["total_jobs"] != 2 {
		t.Errorf("total_jobs = %v, want 2", s["total_jobs"])
	}
	if s["enabled_jobs"] != 1 {
		t.Errorf("enabled_jobs = %v, want 1", s["enabled_jobs"])
	}
	if s["running"] != false {
		t.Errorf("running = %v, want false", s["running"])
	}
}

func TestStartStop(t *testing.T) {
	e := NewEngine(func(job *Job) error { return nil }, 3)
	e.AddJob(&Job{
		ID:       "j1",
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleEvery, Every: time.Hour},
	})

	e.Start()
	s := e.Status()
	if s["running"] != true {
		t.Error("engine should be running after Start")
	}

	// Starting again should be a no-op.
	e.Start()

	e.Stop()
	s = e.Status()
	if s["running"] != false {
		t.Error("engine should not be running after Stop")
	}
}

func TestExecuteAtSchedule(t *testing.T) {
	var mu sync.Mutex
	var executed []string

	handler := func(job *Job) error {
		mu.Lock()
		executed = append(executed, job.ID)
		mu.Unlock()
		return nil
	}

	e := NewEngine(handler, 3)

	// Add a job with an "at" schedule set far in the future so it stays enabled.
	future := time.Now().Add(time.Hour)
	e.AddJob(&Job{
		ID:       "j1",
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleAt, At: &future},
	})

	// Manually set NextRunAt to now so it triggers on the next tick.
	e.mu.Lock()
	j := e.jobs["j1"]
	now := time.Now()
	j.NextRunAt = &now
	e.mu.Unlock()

	e.Start()
	time.Sleep(2 * time.Second)
	e.Stop()

	mu.Lock()
	defer mu.Unlock()
	if len(executed) != 1 || executed[0] != "j1" {
		t.Errorf("executed = %v, want [j1]", executed)
	}
}

func TestDeleteAfterRun(t *testing.T) {
	handler := func(job *Job) error { return nil }

	e := NewEngine(handler, 3)

	future := time.Now().Add(time.Hour)
	e.AddJob(&Job{
		ID:             "j1",
		Enabled:        true,
		DeleteAfterRun: true,
		Schedule:       Schedule{Kind: ScheduleAt, At: &future},
	})

	// Manually set NextRunAt to now so it triggers on the next tick.
	e.mu.Lock()
	j := e.jobs["j1"]
	now := time.Now()
	j.NextRunAt = &now
	e.mu.Unlock()

	e.Start()
	time.Sleep(2 * time.Second)
	e.Stop()

	if len(e.ListJobs()) != 0 {
		t.Errorf("expected 0 jobs after delete-after-run, got %d", len(e.ListJobs()))
	}
}

func TestConsecutiveErrorsDisable(t *testing.T) {
	handler := func(job *Job) error { return nil }
	e := NewEngine(handler, 3)

	future := time.Now().Add(time.Hour)
	e.AddJob(&Job{
		ID:                "j1",
		Enabled:           true,
		ConsecutiveErrors: 5,
		MaxRetries:        3,
		Schedule:          Schedule{Kind: ScheduleAt, At: &future},
	})

	// Manually set NextRunAt to now so it triggers on the next tick.
	e.mu.Lock()
	j := e.jobs["j1"]
	now := time.Now()
	j.NextRunAt = &now
	e.mu.Unlock()

	e.Start()
	time.Sleep(2 * time.Second)
	e.Stop()

	jobs := e.ListJobs()
	if len(jobs) != 1 || jobs[0].Enabled {
		t.Error("job with too many errors should be disabled")
	}
}

func TestMarshalJSON(t *testing.T) {
	e := NewEngine(nil, 3)
	e.AddJob(&Job{ID: "j1", Name: "Job 1"})

	data, err := e.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if len(data) == 0 {
		t.Error("MarshalJSON returned empty data")
	}
}

// TestScheduleChangeLog verifies that AddJob/RemoveJob/EnableJob emit
// schedule.change events through an attached eventlog.Log.
func TestScheduleChangeLog(t *testing.T) {
	e := NewEngine(nil, 3)

	var observed []eventlog.Event
	journal := eventlog.New(func(ev eventlog.Event) {
		observed = append(observed, ev)
	})
	e.SetJournal(journal)

	// AddJob should emit schedule.change.
	e.AddJob(&Job{
		ID:       "j1",
		Name:     "TestJob",
		Enabled:  true,
		Schedule: Schedule{Kind: ScheduleEvery, Every: time.Hour},
	})
	if len(observed) != 1 {
		t.Fatalf("expected 1 schedule.change after AddJob, got %d", len(observed))
	}
	if observed[0].Type != eventlog.ScheduleChange {
		t.Fatalf("expected ScheduleChange type, got %s", observed[0].Type)
	}
	f, ok := observed[0].Data.(eventlog.ScheduleChangeFact)
	if !ok {
		t.Fatalf("expected ScheduleChangeFact, got %T", observed[0].Data)
	}
	if f.Cron == "" {
		t.Fatal("expected non-empty cron string")
	}
	if !strings.Contains(f.Cron, "added") {
		t.Fatalf("expected cron to contain 'added', got %q", f.Cron)
	}

	// EnableJob(false) should emit schedule.change.
	e.EnableJob("j1", false)
	if len(observed) != 2 {
		t.Fatalf("expected 2 schedule.change events, got %d", len(observed))
	}
	f2, _ := observed[1].Data.(eventlog.ScheduleChangeFact)
	if !strings.Contains(f2.Cron, "disabled") {
		t.Fatalf("expected cron to contain 'disabled', got %q", f2.Cron)
	}

	// RemoveJob should emit schedule.change.
	e.RemoveJob("j1")
	if len(observed) != 3 {
		t.Fatalf("expected 3 schedule.change events, got %d", len(observed))
	}
	f3, _ := observed[2].Data.(eventlog.ScheduleChangeFact)
	if !strings.Contains(f3.Cron, "removed") {
		t.Fatalf("expected cron to contain 'removed', got %q", f3.Cron)
	}
}

// TestScheduleChangeNoJournal verifies no panic when journal is nil.
func TestScheduleChangeNoJournal(t *testing.T) {
	e := NewEngine(nil, 3)
	// Should not panic without a journal.
	e.AddJob(&Job{ID: "j1", Name: "NoJournal", Enabled: true})
	e.EnableJob("j1", false)
	e.RemoveJob("j1")
}
