package compact

import (
	"testing"
	"time"
)

func TestNewCompactionTrigger(t *testing.T) {
	t.Parallel()
	ct := NewCompactionTrigger(200000)
	if ct == nil {
		t.Fatal("NewCompactionTrigger returned nil")
	}
	if ct.WindowSize != 200000 {
		t.Errorf("expected WindowSize 200000, got %d", ct.WindowSize)
	}
	if ct.Threshold != 0.75 {
		t.Errorf("expected Threshold 0.75, got %f", ct.Threshold)
	}
	if ct.MinInterval != 30*time.Second {
		t.Errorf("expected MinInterval 30s, got %v", ct.MinInterval)
	}
}

func TestCompactionTrigger_ShouldCompact_AboveThreshold(t *testing.T) {
	t.Parallel()
	ct := NewCompactionTrigger(1000)
	ct.MinInterval = 0 // disable interval check

	// 800/1000 = 0.80 >= 0.75
	if !ct.ShouldCompact(800) {
		t.Error("expected ShouldCompact=true at 80% usage")
	}
}

func TestCompactionTrigger_ShouldCompact_BelowThreshold(t *testing.T) {
	t.Parallel()
	ct := NewCompactionTrigger(1000)
	ct.MinInterval = 0

	// 500/1000 = 0.50 < 0.75
	if ct.ShouldCompact(500) {
		t.Error("expected ShouldCompact=false at 50% usage")
	}
}

func TestCompactionTrigger_ShouldCompact_ExactlyAtThreshold(t *testing.T) {
	t.Parallel()
	ct := NewCompactionTrigger(1000)
	ct.MinInterval = 0

	// 750/1000 = 0.75 == 0.75
	if !ct.ShouldCompact(750) {
		t.Error("expected ShouldCompact=true at exactly 75% usage")
	}
}

func TestCompactionTrigger_ShouldCompact_ZeroWindow(t *testing.T) {
	t.Parallel()
	ct := NewCompactionTrigger(0)
	ct.MinInterval = 0

	if ct.ShouldCompact(100) {
		t.Error("expected ShouldCompact=false with zero window size")
	}
}

func TestCompactionTrigger_ShouldCompact_MinInterval(t *testing.T) {
	t.Parallel()
	ct := NewCompactionTrigger(1000)
	ct.MinInterval = 1 * time.Hour

	// First call should be allowed
	ct.LastCompact = time.Now().Add(-2 * time.Hour)
	if !ct.ShouldCompact(900) {
		t.Error("expected ShouldCompact=true after min interval elapsed")
	}

	// Recent compact should prevent re-compaction
	ct.LastCompact = time.Now()
	if ct.ShouldCompact(900) {
		t.Error("expected ShouldCompact=false within min interval")
	}
}

func TestCompactionTrigger_MarkCompacted(t *testing.T) {
	t.Parallel()
	ct := NewCompactionTrigger(1000)

	before := time.Now()
	ct.MarkCompacted()
	after := time.Now()

	if ct.LastCompact.Before(before) || ct.LastCompact.After(after) {
		t.Error("MarkCompacted should set LastCompact to current time")
	}
}

func TestCompactionTrigger_FullCycle(t *testing.T) {
	t.Parallel()
	ct := NewCompactionTrigger(100000)
	ct.MinInterval = 0

	// Under threshold - no compaction
	if ct.ShouldCompact(50000) {
		t.Error("should not compact at 50%")
	}

	// Above threshold - should compact
	if !ct.ShouldCompact(80000) {
		t.Error("should compact at 80%")
	}

	ct.MarkCompacted()

	// With min interval, should not compact again immediately
	ct.MinInterval = 1 * time.Hour
	if ct.ShouldCompact(80000) {
		t.Error("should not compact again within min interval")
	}
}
