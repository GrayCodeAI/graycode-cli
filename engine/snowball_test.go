package engine

import (
	"strings"
	"testing"
)

func TestNewSnowballDetector(t *testing.T) {
	sd := NewSnowballDetector(100000)
	if sd == nil {
		t.Fatal("expected non-nil detector")
	}
	if sd.maxTokens != 100000 {
		t.Errorf("expected maxTokens=100000, got %d", sd.maxTokens)
	}
	if sd.threshold != 2.0 {
		t.Errorf("expected threshold=2.0, got %f", sd.threshold)
	}
	if len(sd.turnTokens) != 0 {
		t.Error("expected empty turnTokens")
	}
}

func TestSnowballDetector_IsSnowballing(t *testing.T) {
	sd := NewSnowballDetector(100000)

	// First 3 turns: low token usage, good progress
	sd.RecordTurn(100, 0.10)
	sd.RecordTurn(120, 0.12)
	sd.RecordTurn(110, 0.11)

	// Not enough turns yet
	if sd.IsSnowballing() {
		t.Error("should not detect snowball with fewer than 6 turns")
	}

	// Last 3 turns: high token usage, low progress (snowball pattern)
	sd.RecordTurn(200, 0.05)
	sd.RecordTurn(250, 0.03)
	sd.RecordTurn(300, 0.02)

	if !sd.IsSnowballing() {
		t.Error("should detect snowball: tokens doubled while progress declined")
	}
}

func TestSnowballDetector_NotSnowballing(t *testing.T) {
	sd := NewSnowballDetector(100000)

	// Consistent token usage and steady progress
	sd.RecordTurn(100, 0.10)
	sd.RecordTurn(110, 0.11)
	sd.RecordTurn(105, 0.10)
	sd.RecordTurn(100, 0.10)
	sd.RecordTurn(115, 0.12)
	sd.RecordTurn(108, 0.11)

	if sd.IsSnowballing() {
		t.Error("should not detect snowball when usage is stable")
	}
}

func TestSnowballDetector_ShouldAbort_ExceedsMax(t *testing.T) {
	sd := NewSnowballDetector(500)

	sd.RecordTurn(200, 0.3)
	sd.RecordTurn(200, 0.3)
	sd.RecordTurn(200, 0.3)

	if !sd.ShouldAbort() {
		t.Error("should abort when total tokens (600) exceed maxTokens (500)")
	}
}

func TestSnowballDetector_ShouldAbort_GrowthRate3x(t *testing.T) {
	sd := NewSnowballDetector(1000000)

	// First 3 turns: moderate
	sd.RecordTurn(100, 0.10)
	sd.RecordTurn(100, 0.10)
	sd.RecordTurn(100, 0.10)

	// Last 3 turns: 3x growth
	sd.RecordTurn(250, 0.05)
	sd.RecordTurn(300, 0.03)
	sd.RecordTurn(350, 0.02)

	if !sd.ShouldAbort() {
		t.Error("should abort when growth rate is 3x+")
	}
}

func TestSnowballDetector_Summary(t *testing.T) {
	sd := NewSnowballDetector(100000)

	// Record enough turns to get a meaningful summary
	sd.RecordTurn(1000, 0.10)
	sd.RecordTurn(1100, 0.12)
	sd.RecordTurn(1050, 0.11)
	sd.RecordTurn(2000, 0.05)
	sd.RecordTurn(2500, 0.03)
	sd.RecordTurn(3000, 0.02)
	sd.RecordTurn(4000, 0.01)
	sd.RecordTurn(5000, 0.01)

	summary := sd.Summary()

	if !strings.Contains(summary, "Turns: 8") {
		t.Errorf("summary should contain turn count, got: %s", summary)
	}
	if !strings.Contains(summary, "Tokens:") {
		t.Errorf("summary should contain token info, got: %s", summary)
	}
	if !strings.Contains(summary, "Progress:") {
		t.Errorf("summary should contain progress info, got: %s", summary)
	}
	if !strings.Contains(summary, "Recommendation:") {
		t.Errorf("summary should contain recommendation, got: %s", summary)
	}

	// After reset, summary should show 0 turns
	sd.Reset()
	resetSummary := sd.Summary()
	if !strings.Contains(resetSummary, "Turns: 0") {
		t.Errorf("after reset, should show 0 turns, got: %s", resetSummary)
	}
}
