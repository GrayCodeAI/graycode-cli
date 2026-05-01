package analytics

import (
	"strings"
	"testing"
	"time"
)

func TestActivityTracker_UserTime(t *testing.T) {
	tracker := NewActivityTracker()
	tracker.StartUserInput()
	time.Sleep(20 * time.Millisecond)
	tracker.EndUserInput()

	if tracker.UserTime() < 10*time.Millisecond {
		t.Fatalf("expected at least 10ms user time, got %v", tracker.UserTime())
	}
}

func TestActivityTracker_ExecTime(t *testing.T) {
	tracker := NewActivityTracker()
	tracker.StartExecution()
	time.Sleep(20 * time.Millisecond)
	tracker.EndExecution()

	if tracker.ExecTime() < 10*time.Millisecond {
		t.Fatalf("expected at least 10ms exec time, got %v", tracker.ExecTime())
	}
}

func TestActivityTracker_Summary(t *testing.T) {
	tracker := NewActivityTracker()
	tracker.StartUserInput()
	time.Sleep(10 * time.Millisecond)
	tracker.EndUserInput()
	tracker.StartExecution()
	time.Sleep(20 * time.Millisecond)
	tracker.EndExecution()

	summary := tracker.Summary()
	if !strings.Contains(summary, "User:") {
		t.Fatalf("expected User: in summary, got %q", summary)
	}
	if !strings.Contains(summary, "Agent:") {
		t.Fatalf("expected Agent: in summary, got %q", summary)
	}
	if !strings.Contains(summary, "Ratio:") {
		t.Fatalf("expected Ratio: in summary, got %q", summary)
	}
}

func TestActivityTracker_AutoEndOnSwitch(t *testing.T) {
	tracker := NewActivityTracker()
	tracker.StartUserInput()
	time.Sleep(10 * time.Millisecond)
	// Starting execution should auto-end user input
	tracker.StartExecution()
	time.Sleep(10 * time.Millisecond)
	tracker.EndExecution()

	if tracker.UserTime() < 5*time.Millisecond {
		t.Fatal("user time should be accumulated when switching to execution")
	}
}

func TestFormatDurationCompact(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m30s"},
		{3661 * time.Second, "1h1m"},
	}
	for _, tt := range tests {
		got := formatDurationCompact(tt.d)
		if got != tt.want {
			t.Errorf("formatDurationCompact(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
