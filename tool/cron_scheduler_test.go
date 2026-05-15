package tool

import (
	"testing"
	"time"
)

func TestCronScheduler_Create(t *testing.T) {
	t.Parallel()
	s := &CronScheduler{jobs: make(map[string]*CronJob)}

	job, err := s.Create("*/5 * * * *", "check status", true, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if job == nil {
		t.Fatal("Create returned nil")
	}
	if job.ID == "" {
		t.Error("job ID empty")
	}
	if job.Prompt != "check status" {
		t.Errorf("Prompt = %q", job.Prompt)
	}
	if !job.Recurring {
		t.Error("should be recurring")
	}
}

func TestCronScheduler_List(t *testing.T) {
	t.Parallel()
	s := &CronScheduler{jobs: make(map[string]*CronJob)}
	_, _ = s.Create("* * * * *", "a", false, false)
	_, _ = s.Create("* * * * *", "b", false, false)

	jobs := s.List()
	if len(jobs) != 2 {
		t.Errorf("List = %d, want 2", len(jobs))
	}
}

func TestCronScheduler_Delete(t *testing.T) {
	t.Parallel()
	s := &CronScheduler{jobs: make(map[string]*CronJob)}
	job, _ := s.Create("* * * * *", "x", false, false)

	if !s.Delete(job.ID) {
		t.Error("Delete should return true")
	}
	if s.Delete(job.ID) {
		t.Error("Delete of missing should return false")
	}
}

func TestCronScheduler_Get(t *testing.T) {
	t.Parallel()
	s := &CronScheduler{jobs: make(map[string]*CronJob)}
	job, _ := s.Create("* * * * *", "x", false, false)

	got, ok := s.Get(job.ID)
	if !ok || got == nil {
		t.Error("Get should find job")
	}
	_, ok = s.Get("missing")
	if ok {
		t.Error("Get should not find missing")
	}
}

func TestFieldMatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		field string
		value int
		want  bool
	}{
		{"*", 5, true},
		{"5", 5, true},
		{"5", 6, false},
		{"*/5", 10, true},
		{"*/5", 7, false},
		{"1,5,10", 5, true},
		{"1,5,10", 3, false},
		{"1-5", 3, true},
		{"1-5", 6, false},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			t.Parallel()
			got := fieldMatches(tt.field, tt.value)
			if got != tt.want {
				t.Errorf("fieldMatches(%q, %d) = %v, want %v", tt.field, tt.value, got, tt.want)
			}
		})
	}
}

func TestCronMatches(t *testing.T) {
	t.Parallel()
	// "0 12 * * *" = noon every day
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	if !cronMatches([]string{"0", "12", "*", "*", "*"}, now) {
		t.Error("should match noon")
	}
	notNoon := time.Date(2026, 5, 15, 13, 0, 0, 0, time.UTC)
	if cronMatches([]string{"0", "12", "*", "*", "*"}, notNoon) {
		t.Error("should not match 1pm")
	}
}

func TestNextCronTime(t *testing.T) {
	t.Parallel()
	next, err := nextCronTime("* * * * *")
	if err != nil {
		t.Fatalf("nextCronTime: %v", err)
	}
	if next.IsZero() {
		t.Error("next should not be zero")
	}
	if time.Until(next) > 2*time.Minute {
		t.Errorf("next = %v, too far in future", next)
	}
}

func TestNextCronTime_Invalid(t *testing.T) {
	t.Parallel()
	_, err := nextCronTime("invalid")
	if err == nil {
		t.Error("should error on invalid cron expression")
	}
}
