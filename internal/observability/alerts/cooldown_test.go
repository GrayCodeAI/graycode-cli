package alerts

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultCooldownConfig(t *testing.T) {
	c := DefaultCooldownConfig()
	if c.Period != 24*time.Hour {
		t.Errorf("Period = %v, want 24h", c.Period)
	}
	if c.MaxPending != 100 {
		t.Errorf("MaxPending = %d, want 100", c.MaxPending)
	}
	if c.DrainInterval != 5*time.Minute {
		t.Errorf("DrainInterval = %v, want 5m", c.DrainInterval)
	}
	if c.SendDelay != time.Second {
		t.Errorf("SendDelay = %v, want 1s", c.SendDelay)
	}
}

func TestEnqueueBasic(t *testing.T) {
	q := NewAlertQueue(DefaultCooldownConfig(), nil)
	ok := q.Enqueue(&Alert{ID: "1", Type: "error", Entity: "svc"})
	if !ok {
		t.Fatal("expected enqueue to succeed")
	}
	if q.PendingCount() != 1 {
		t.Fatalf("PendingCount = %d, want 1", q.PendingCount())
	}
}

func TestEnqueueCooldown(t *testing.T) {
	cfg := CooldownConfig{
		Period:     time.Hour,
		MaxPending: 100,
	}
	handler := func(a *Alert) error { return nil }
	q := NewAlertQueue(cfg, handler)

	// First enqueue succeeds.
	if !q.Enqueue(&Alert{ID: "1", Type: "error", Entity: "svc"}) {
		t.Fatal("first enqueue should succeed")
	}

	// Drain to trigger cooldown tracking.
	q.drain()

	// Second enqueue for same entity:type should be rejected (cooldown).
	if q.Enqueue(&Alert{ID: "2", Type: "error", Entity: "svc"}) {
		t.Fatal("second enqueue should be rejected by cooldown")
	}

	// Different type should succeed.
	if !q.Enqueue(&Alert{ID: "3", Type: "warn", Entity: "svc"}) {
		t.Fatal("enqueue for different type should succeed")
	}
}

func TestEnqueueMaxPending(t *testing.T) {
	cfg := CooldownConfig{
		Period:     time.Hour,
		MaxPending: 2,
	}
	q := NewAlertQueue(cfg, nil)

	if !q.Enqueue(&Alert{ID: "1", Type: "a", Entity: "e1"}) {
		t.Fatal("enqueue 1 should succeed")
	}
	if !q.Enqueue(&Alert{ID: "2", Type: "a", Entity: "e2"}) {
		t.Fatal("enqueue 2 should succeed")
	}
	if q.Enqueue(&Alert{ID: "3", Type: "a", Entity: "e3"}) {
		t.Fatal("enqueue 3 should fail (max pending)")
	}
}

func TestDrain(t *testing.T) {
	cfg := CooldownConfig{
		Period:     time.Hour,
		MaxPending: 100,
		SendDelay:  0,
	}
	var delivered atomic.Int32
	handler := func(a *Alert) error {
		delivered.Add(1)
		return nil
	}
	q := NewAlertQueue(cfg, handler)

	q.Enqueue(&Alert{ID: "1", Type: "t", Entity: "e"})
	q.Enqueue(&Alert{ID: "2", Type: "t", Entity: "e2"})
	q.drain()

	if delivered.Load() != 2 {
		t.Errorf("delivered = %d, want 2", delivered.Load())
	}
	if q.PendingCount() != 0 {
		t.Errorf("PendingCount = %d, want 0 after drain", q.PendingCount())
	}
}

func TestDrainHandlerError(t *testing.T) {
	cfg := CooldownConfig{
		Period:     time.Hour,
		MaxPending: 100,
		SendDelay:  0,
	}
	handler := func(a *Alert) error {
		return errors.New("send failed")
	}
	q := NewAlertQueue(cfg, handler)

	q.Enqueue(&Alert{ID: "1", Type: "t", Entity: "e"})
	q.drain()

	// Failed delivery should not set cooldown, so a second enqueue should succeed.
	if !q.Enqueue(&Alert{ID: "2", Type: "t", Entity: "e"}) {
		t.Error("second enqueue should succeed since cooldown was not set for failed delivery")
	}
}

func TestStartStop(t *testing.T) {
	cfg := CooldownConfig{
		Period:        time.Hour,
		MaxPending:    100,
		DrainInterval: 10 * time.Millisecond,
		SendDelay:     0,
	}
	var delivered atomic.Int32
	handler := func(a *Alert) error {
		delivered.Add(1)
		return nil
	}
	q := NewAlertQueue(cfg, handler)
	q.Enqueue(&Alert{ID: "1", Type: "t", Entity: "e"})

	q.Start()
	time.Sleep(50 * time.Millisecond)
	q.Stop()

	if delivered.Load() < 1 {
		t.Error("expected at least 1 delivery after start")
	}
}
