package tool

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCronCreateTool(t *testing.T) {
	// Reset scheduler
	globalCronScheduler = &CronScheduler{jobs: make(map[string]*CronJob)}

	input, _ := json.Marshal(map[string]any{
		"schedule":  "*/5 * * * *",
		"prompt":    "check the build status",
		"recurring": true,
	})
	result, err := (CronCreateTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		ID       string `json:"id"`
		Schedule string `json:"schedule"`
		Type     string `json:"type"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.ID == "" {
		t.Fatal("expected job ID")
	}
	if resp.Schedule != "*/5 * * * *" {
		t.Fatalf("expected schedule '*/5 * * * *', got %q", resp.Schedule)
	}
	if resp.Type != "recurring" {
		t.Fatalf("expected type 'recurring', got %q", resp.Type)
	}
}

func TestCronCreateTool_OneShot(t *testing.T) {
	globalCronScheduler = &CronScheduler{jobs: make(map[string]*CronJob)}

	recurring := false
	input, _ := json.Marshal(map[string]any{
		"schedule":  "30 14 * * *",
		"prompt":    "remind me",
		"recurring": recurring,
	})
	result, err := (CronCreateTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Type string `json:"type"`
	}
	json.Unmarshal([]byte(result), &resp)
	if resp.Type != "one-shot" {
		t.Fatalf("expected type 'one-shot', got %q", resp.Type)
	}
}

func TestCronListTool(t *testing.T) {
	globalCronScheduler = &CronScheduler{jobs: make(map[string]*CronJob)}
	globalCronScheduler.Create("0 9 * * *", "morning check", true, false)
	globalCronScheduler.Create("*/10 * * * *", "poll status", true, false)

	result, err := (CronListTool{}).Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	json.Unmarshal([]byte(result), &resp)
	if len(resp.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(resp.Jobs))
	}
}

func TestCronDeleteTool(t *testing.T) {
	globalCronScheduler = &CronScheduler{jobs: make(map[string]*CronJob)}
	job, _ := globalCronScheduler.Create("0 9 * * *", "test", true, false)

	input, _ := json.Marshal(map[string]string{"id": job.ID})
	result, err := (CronDeleteTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}

	// Verify deletion
	jobs := globalCronScheduler.List()
	if len(jobs) != 0 {
		t.Fatal("expected 0 jobs after delete")
	}
}

func TestCronDeleteTool_NotFound(t *testing.T) {
	globalCronScheduler = &CronScheduler{jobs: make(map[string]*CronJob)}
	input, _ := json.Marshal(map[string]string{"id": "bad_id"})
	_, err := (CronDeleteTool{}).Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestCronFieldMatches(t *testing.T) {
	tests := []struct {
		field string
		value int
		want  bool
	}{
		{"*", 5, true},
		{"5", 5, true},
		{"6", 5, false},
		{"*/5", 10, true},
		{"*/5", 7, false},
		{"1-5", 3, true},
		{"1-5", 6, false},
		{"1,3,5", 3, true},
		{"1,3,5", 4, false},
	}
	for _, tt := range tests {
		got := fieldMatches(tt.field, tt.value)
		if got != tt.want {
			t.Errorf("fieldMatches(%q, %d) = %v, want %v", tt.field, tt.value, got, tt.want)
		}
	}
}
