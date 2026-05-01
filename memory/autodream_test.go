package memory

import (
	"context"
	"testing"
	"time"
)

func TestDefaultAutoDreamConfig(t *testing.T) {
	cfg := DefaultAutoDreamConfig()
	if !cfg.Enabled {
		t.Error("should be enabled by default")
	}
	if cfg.MinElapsedTime != 24*time.Hour {
		t.Errorf("expected 24h, got %v", cfg.MinElapsedTime)
	}
	if cfg.MinNewSessions != 5 {
		t.Errorf("expected 5 sessions, got %d", cfg.MinNewSessions)
	}
}

func TestAutoDreamState_ShouldDream(t *testing.T) {
	cfg := DefaultAutoDreamConfig()
	state := NewAutoDreamState()

	// Fresh state should not dream
	if state.ShouldDream(cfg) {
		t.Error("fresh state should not trigger dream")
	}

	// After enough sessions but not enough time
	for i := 0; i < 10; i++ {
		state.RecordSession()
	}
	if state.ShouldDream(cfg) {
		t.Error("should not dream without elapsed time")
	}

	// Backdate last dream time
	state.LastDreamTime = time.Now().Add(-25 * time.Hour)
	if !state.ShouldDream(cfg) {
		t.Error("should dream after 25h + 10 sessions")
	}
}

func TestAutoDreamState_ShouldDream_Disabled(t *testing.T) {
	cfg := DefaultAutoDreamConfig()
	cfg.Enabled = false
	state := NewAutoDreamState()
	state.LastDreamTime = time.Now().Add(-48 * time.Hour)
	state.SessionsSince = 100

	if state.ShouldDream(cfg) {
		t.Error("should not dream when disabled")
	}
}

func TestAutoDreamState_MarkComplete(t *testing.T) {
	state := NewAutoDreamState()
	state.SessionsSince = 10
	state.MarkDreamComplete()

	if state.SessionsSince != 0 {
		t.Error("sessions should reset after dream")
	}
	if state.DreamCount != 1 {
		t.Errorf("dream count should be 1, got %d", state.DreamCount)
	}
}

func TestAutoDreamState_MarkFailed(t *testing.T) {
	state := NewAutoDreamState()
	state.MarkDreamFailed(context.DeadlineExceeded)

	if state.LastError == "" {
		t.Error("should record error message")
	}
}

func TestRunDream_NoMemories(t *testing.T) {
	cfg := DefaultAutoDreamConfig()
	_, err := RunDream(context.Background(), cfg, func(ctx context.Context, prompt string) (string, error) {
		return "consolidated", nil
	})
	// Will fail because directory doesn't exist or is empty
	if err == nil {
		t.Skip("test requires empty/missing memory dir")
	}
}
