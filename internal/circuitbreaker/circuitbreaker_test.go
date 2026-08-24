package circuitbreaker

import (
	"testing"
	"time"
)

func TestClosedAllowsUntilThreshold(t *testing.T) {
	b := New(3, 5*time.Minute)
	base := time.Now()
	for i := 0; i < 3; i++ {
		d := b.ShouldAllow(base)
		if !d.Allow || d.WasHalfOpen {
			t.Fatalf("call %d: allow=%v halfOpen=%v, want allow", i+1, d.Allow, d.WasHalfOpen)
		}
		b.RecordFailure(base)
	}
	if b.State(base) != Open {
		t.Fatal("expected Open after 3 failures")
	}
}

func TestOpenSkipsUntilCooldownElapses(t *testing.T) {
	b := New(3, time.Minute)
	base := time.Now()
	for i := 0; i < 3; i++ {
		b.RecordFailure(base)
	}
	// Within cooldown: skipped.
	if d := b.ShouldAllow(base.Add(30 * time.Second)); d.Allow {
		t.Fatal("expected skip while open")
	}
	// After cooldown: half-open probe admitted.
	d := b.ShouldAllow(base.Add(2 * time.Minute))
	if !d.Allow || !d.WasHalfOpen {
		t.Fatalf("expected half-open probe, got allow=%v halfOpen=%v", d.Allow, d.WasHalfOpen)
	}
	if d.EffectiveConsecutiveFailures != 2 {
		t.Fatalf("effective failures = %d, want 2", d.EffectiveConsecutiveFailures)
	}
}

func TestHalfOpenSingleFailureReopens(t *testing.T) {
	b := New(3, time.Minute)
	base := time.Now()
	for i := 0; i < 3; i++ {
		b.RecordFailure(base)
	}
	// Half-open probe admitted after cooldown, then fails once.
	b.ShouldAllow(base.Add(2 * time.Minute))
	b.RecordFailure(base.Add(2 * time.Minute))
	if b.State(base.Add(2*time.Minute+1)) != Open {
		t.Fatal("expected Open again after half-open failure")
	}
}

func TestHalfOpenSuccessCloses(t *testing.T) {
	b := New(3, time.Minute)
	base := time.Now()
	for i := 0; i < 3; i++ {
		b.RecordFailure(base)
	}
	b.ShouldAllow(base.Add(2 * time.Minute))
	b.RecordSuccess()
	if b.State(base.Add(3*time.Minute)) != Closed {
		t.Fatal("expected Closed after half-open success")
	}
	if b.ConsecutiveFailures() != 0 {
		t.Fatalf("consecutive failures = %d, want 0", b.ConsecutiveFailures())
	}
}

func TestMinThresholdFloor(t *testing.T) {
	if New(0, time.Minute).maxConsecutiveFailures != 1 {
		t.Fatal("threshold should floor to 1")
	}
}

func TestNegativeCooldownTreatedAsZero(t *testing.T) {
	b := New(1, -time.Second)
	base := time.Now()
	b.RecordFailure(base)
	// Zero cooldown: next call is a half-open probe immediately.
	d := b.ShouldAllow(base)
	if !d.Allow || !d.WasHalfOpen {
		t.Fatalf("expected immediate half-open probe, got %+v", d)
	}
}
