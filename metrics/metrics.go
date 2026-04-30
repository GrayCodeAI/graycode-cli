// Package metrics provides basic metrics collection (counters, timers, gauges).
package metrics

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Counter is a monotonically increasing counter.
type Counter struct {
	value int64
}

// Inc increments the counter by 1.
func (c *Counter) Inc() {
	atomic.AddInt64(&c.value, 1)
}

// Add adds n to the counter.
func (c *Counter) Add(n int64) {
	atomic.AddInt64(&c.value, n)
}

// Value returns the current value.
func (c *Counter) Value() int64 {
	return atomic.LoadInt64(&c.value)
}

// Gauge is a value that can go up and down.
type Gauge struct {
	value int64
}

// Set sets the gauge value.
func (g *Gauge) Set(v int64) {
	atomic.StoreInt64(&g.value, v)
}

// Inc increments the gauge by 1.
func (g *Gauge) Inc() {
	atomic.AddInt64(&g.value, 1)
}

// Dec decrements the gauge by 1.
func (g *Gauge) Dec() {
	atomic.AddInt64(&g.value, -1)
}

// Value returns the current value.
func (g *Gauge) Value() int64 {
	return atomic.LoadInt64(&g.value)
}

// Timer tracks timing information.
type Timer struct {
	mu        sync.RWMutex
	count     int64
	totalTime int64
	minTime   int64
	maxTime   int64
}

// NewTimer creates a new timer.
func NewTimer() *Timer {
	return &Timer{
		minTime: -1, // sentinel for unset
	}
}

// Record records a duration.
func (t *Timer) Record(d time.Duration) {
	ms := d.Milliseconds()
	atomic.AddInt64(&t.count, 1)
	atomic.AddInt64(&t.totalTime, ms)

	t.mu.Lock()
	if t.minTime == -1 || ms < t.minTime {
		t.minTime = ms
	}
	if ms > t.maxTime {
		t.maxTime = ms
	}
	t.mu.Unlock()
}

// Time executes fn and records its duration.
func (t *Timer) Time(fn func()) time.Duration {
	start := time.Now()
	fn()
	d := time.Since(start)
	t.Record(d)
	return d
}

// Stats returns timer statistics.
func (t *Timer) Stats() TimerStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := atomic.LoadInt64(&t.count)
	if count == 0 {
		return TimerStats{}
	}

	total := atomic.LoadInt64(&t.totalTime)
	return TimerStats{
		Count: count,
		Total: time.Duration(total) * time.Millisecond,
		Mean:  time.Duration(total/count) * time.Millisecond,
		Min:   time.Duration(t.minTime) * time.Millisecond,
		Max:   time.Duration(t.maxTime) * time.Millisecond,
	}
}

// TimerStats represents timer statistics.
type TimerStats struct {
	Count int64         `json:"count"`
	Total time.Duration `json:"total"`
	Mean  time.Duration `json:"mean"`
	Min   time.Duration `json:"min"`
	Max   time.Duration `json:"max"`
}

// Registry manages named metrics.
type Registry struct {
	mu       sync.RWMutex
	counters map[string]*Counter
	gauges   map[string]*Gauge
	timers   map[string]*Timer
}

// NewRegistry creates a new metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		counters: make(map[string]*Counter),
		gauges:   make(map[string]*Gauge),
		timers:   make(map[string]*Timer),
	}
}

// Counter returns or creates a counter.
func (r *Registry) Counter(name string) *Counter {
	r.mu.RLock()
	c, ok := r.counters[name]
	r.mu.RUnlock()
	if ok {
		return c
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[name]; ok {
		return c
	}
	c = &Counter{}
	r.counters[name] = c
	return c
}

// Gauge returns or creates a gauge.
func (r *Registry) Gauge(name string) *Gauge {
	r.mu.RLock()
	g, ok := r.gauges[name]
	r.mu.RUnlock()
	if ok {
		return g
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.gauges[name]; ok {
		return g
	}
	g = &Gauge{}
	r.gauges[name] = g
	return g
}

// Timer returns or creates a timer.
func (r *Registry) Timer(name string) *Timer {
	r.mu.RLock()
	t, ok := r.timers[name]
	r.mu.RUnlock()
	if ok {
		return t
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.timers[name]; ok {
		return t
	}
	t = NewTimer()
	r.timers[name] = t
	return t
}

// Snapshot returns a snapshot of all metrics.
func (r *Registry) Snapshot() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]interface{})
	for name, c := range r.counters {
		out[name] = map[string]int64{"value": c.Value()}
	}
	for name, g := range r.gauges {
		out[name] = map[string]int64{"value": g.Value()}
	}
	for name, t := range r.timers {
		out[name] = t.Stats()
	}
	return out
}

// Format returns a formatted string of all metrics.
func (r *Registry) Format() string {
	snap := r.Snapshot()
	if len(snap) == 0 {
		return "No metrics collected."
	}

	out := "Metrics:\n"
	for name, val := range snap {
		out += fmt.Sprintf("  %s: %v\n", name, val)
	}
	return out
}
