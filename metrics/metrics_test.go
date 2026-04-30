package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestCounter(t *testing.T) {
	c := &Counter{}

	c.Inc()
	if c.Value() != 1 {
		t.Fatalf("expected 1, got %d", c.Value())
	}

	c.Add(5)
	if c.Value() != 6 {
		t.Fatalf("expected 6, got %d", c.Value())
	}
}

func TestGauge(t *testing.T) {
	g := &Gauge{}

	g.Set(10)
	if g.Value() != 10 {
		t.Fatalf("expected 10, got %d", g.Value())
	}

	g.Inc()
	if g.Value() != 11 {
		t.Fatalf("expected 11, got %d", g.Value())
	}

	g.Dec()
	if g.Value() != 10 {
		t.Fatalf("expected 10, got %d", g.Value())
	}
}

func TestTimer(t *testing.T) {
	tm := NewTimer()

	tm.Record(100 * time.Millisecond)
	tm.Record(200 * time.Millisecond)

	stats := tm.Stats()
	if stats.Count != 2 {
		t.Fatalf("expected count 2, got %d", stats.Count)
	}
	if stats.Mean != 150*time.Millisecond {
		t.Fatalf("expected mean 150ms, got %v", stats.Mean)
	}
	if stats.Min != 100*time.Millisecond {
		t.Fatalf("expected min 100ms, got %v", stats.Min)
	}
	if stats.Max != 200*time.Millisecond {
		t.Fatalf("expected max 200ms, got %v", stats.Max)
	}
}

func TestTimerTime(t *testing.T) {
	tm := NewTimer()

	d := tm.Time(func() {
		time.Sleep(10 * time.Millisecond)
	})

	if d < 5*time.Millisecond {
		t.Fatalf("expected at least 5ms, got %v", d)
	}

	stats := tm.Stats()
	if stats.Count != 1 {
		t.Fatalf("expected 1 recording, got %d", stats.Count)
	}
}

func TestTimerEmpty(t *testing.T) {
	tm := NewTimer()
	stats := tm.Stats()
	if stats.Count != 0 {
		t.Fatalf("expected 0 count, got %d", stats.Count)
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	c := r.Counter("requests")
	c.Inc()
	c.Inc()

	c2 := r.Counter("requests")
	if c2.Value() != 2 {
		t.Fatalf("expected shared counter with value 2, got %d", c2.Value())
	}

	g := r.Gauge("connections")
	g.Set(5)

	tm := r.Timer("latency")
	tm.Record(10 * time.Millisecond)

	snap := r.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(snap))
	}
}

func TestRegistryFormat(t *testing.T) {
	r := NewRegistry()
	r.Counter("test").Inc()

	fmt := r.Format()
	if !strings.Contains(fmt, "test") {
		t.Fatal("expected formatted output to contain metric name")
	}
}

func TestRegistryEmptyFormat(t *testing.T) {
	r := NewRegistry()
	if r.Format() != "No metrics collected." {
		t.Fatalf("unexpected format: %q", r.Format())
	}
}
