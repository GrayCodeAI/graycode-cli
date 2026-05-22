package observability

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Profiler tracks and reports on agent performance metrics including
// response times, token efficiency, and tool call patterns.
type Profiler struct {
	Enabled  bool
	Spans    []ProfileSpan
	Counters map[string]*Counter
	Timers   map[string]*Timer
	mu       sync.RWMutex
}

// ProfileSpan represents a timed span of execution.
type ProfileSpan struct {
	Name     string
	Start    time.Time
	End      time.Time
	Duration time.Duration
	Metadata map[string]string
	Children []ProfileSpan
}

// Counter tracks a named incrementing value.
type Counter struct {
	Name  string
	Value int64
	mu    sync.Mutex
}

// Timer tracks duration samples for a named operation.
type Timer struct {
	Name    string
	Samples []time.Duration
	mu      sync.Mutex
}

// NewProfiler creates a new enabled Profiler with initialized maps.
func NewProfiler() *Profiler {
	return &Profiler{
		Enabled:  true,
		Spans:    make([]ProfileSpan, 0),
		Counters: make(map[string]*Counter),
		Timers:   make(map[string]*Timer),
	}
}

// StartSpan begins a new profiling span with the given name.
func (p *Profiler) StartSpan(name string) *ProfileSpan {
	span := &ProfileSpan{
		Name:     name,
		Start:    time.Now(),
		Metadata: make(map[string]string),
		Children: make([]ProfileSpan, 0),
	}
	return span
}

// EndSpan completes a span and records it.
func (p *Profiler) EndSpan(span *ProfileSpan) {
	if span == nil {
		return
	}
	span.End = time.Now()
	span.Duration = span.End.Sub(span.Start)

	p.mu.Lock()
	p.Spans = append(p.Spans, *span)
	p.mu.Unlock()

	p.RecordDuration(span.Name, span.Duration)
}

// Increment adds delta to the named counter.
func (p *Profiler) Increment(counter string, delta int64) {
	p.mu.Lock()
	c, ok := p.Counters[counter]
	if !ok {
		c = &Counter{Name: counter}
		p.Counters[counter] = c
	}
	p.mu.Unlock()

	c.mu.Lock()
	c.Value += delta
	c.mu.Unlock()
}

// RecordDuration adds a duration sample to the named timer.
func (p *Profiler) RecordDuration(timer string, d time.Duration) {
	p.mu.Lock()
	t, ok := p.Timers[timer]
	if !ok {
		t = &Timer{Name: timer, Samples: make([]time.Duration, 0)}
		p.Timers[timer] = t
	}
	p.mu.Unlock()

	t.mu.Lock()
	t.Samples = append(t.Samples, d)
	t.mu.Unlock()
}

// GetP50 returns the 50th percentile duration for a timer.
func (p *Profiler) GetP50(timer string) time.Duration {
	return p.getPercentile(timer, 0.50)
}

// GetP95 returns the 95th percentile duration for a timer.
func (p *Profiler) GetP95(timer string) time.Duration {
	return p.getPercentile(timer, 0.95)
}

// GetP99 returns the 99th percentile duration for a timer.
func (p *Profiler) GetP99(timer string) time.Duration {
	return p.getPercentile(timer, 0.99)
}

func (p *Profiler) getPercentile(timer string, pct float64) time.Duration {
	p.mu.RLock()
	t, ok := p.Timers[timer]
	p.mu.RUnlock()
	if !ok || len(t.Samples) == 0 {
		return 0
	}

	t.mu.Lock()
	sorted := make([]time.Duration, len(t.Samples))
	copy(sorted, t.Samples)
	t.mu.Unlock()

	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	idx := int(float64(len(sorted)-1) * pct)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// Report generates a human-readable performance profile report.
func (p *Profiler) Report() string {
	var b strings.Builder

	b.WriteString("Performance Profile:\n")
	b.WriteString("═══════════════════════════════════════════\n\n")

	// Timing section
	p.mu.RLock()
	timerNames := make([]string, 0, len(p.Timers))
	for name := range p.Timers {
		timerNames = append(timerNames, name)
	}
	sort.Strings(timerNames)
	p.mu.RUnlock()

	if len(timerNames) > 0 {
		b.WriteString("Timing:\n")
		maxNameLen := 0
		for _, name := range timerNames {
			if len(name) > maxNameLen {
				maxNameLen = len(name)
			}
		}
		for _, name := range timerNames {
			p50 := p.GetP50(name)
			p95 := p.GetP95(name)
			p99 := p.GetP99(name)

			p.mu.RLock()
			t := p.Timers[name]
			p.mu.RUnlock()

			t.mu.Lock()
			n := len(t.Samples)
			t.mu.Unlock()

			padding := strings.Repeat(" ", maxNameLen-len(name))
			b.WriteString(fmt.Sprintf("  %s:%s    P50=%s  P95=%s  P99=%s  (n=%d)\n",
				name, padding,
				formatDuration(p50), formatDuration(p95), formatDuration(p99), n))
		}
		b.WriteString("\n")
	}

	// Counters section
	p.mu.RLock()
	counterNames := make([]string, 0, len(p.Counters))
	for name := range p.Counters {
		counterNames = append(counterNames, name)
	}
	sort.Strings(counterNames)
	p.mu.RUnlock()

	if len(counterNames) > 0 {
		b.WriteString("Counters:\n")
		maxNameLen := 0
		for _, name := range counterNames {
			if len(name) > maxNameLen {
				maxNameLen = len(name)
			}
		}

		for _, name := range counterNames {
			p.mu.RLock()
			c := p.Counters[name]
			p.mu.RUnlock()

			c.mu.Lock()
			val := c.Value
			c.mu.Unlock()

			padding := strings.Repeat(" ", maxNameLen-len(name))
			extra := p.counterExtra(name, val)
			if extra != "" {
				b.WriteString(fmt.Sprintf("  %s:%s   %s %s\n", name, padding, formatInt(val), extra))
			} else {
				b.WriteString(fmt.Sprintf("  %s:%s   %s\n", name, padding, formatInt(val)))
			}
		}
		b.WriteString("\n")
	}

	// Hot Paths section
	hotPaths := p.HotPaths()
	if len(hotPaths) > 0 {
		b.WriteString("Hot Paths:\n")
		for i, path := range hotPaths {
			if i >= 5 {
				break
			}
			avg := p.averagePathDuration(path)
			b.WriteString(fmt.Sprintf("  %d. %s (avg %s)\n", i+1, strings.Join(path, " → "), formatDuration(avg)))
		}
		b.WriteString("\n")
	}

	// Recommendations section
	recs := p.Recommendations()
	if len(recs) > 0 {
		b.WriteString("Recommendations:\n")
		for _, rec := range recs {
			b.WriteString(fmt.Sprintf("  - %s\n", rec))
		}
	}

	return b.String()
}

// HotPaths finds the most time-consuming span sequences.
func (p *Profiler) HotPaths() [][]string {
	p.mu.RLock()
	spans := make([]ProfileSpan, len(p.Spans))
	copy(spans, p.Spans)
	p.mu.RUnlock()

	if len(spans) < 2 {
		return nil
	}

	// Sort spans by start time
	sort.Slice(spans, func(i, j int) bool {
		return spans[i].Start.Before(spans[j].Start)
	})

	// Find consecutive span sequences and track their total durations
	type pathInfo struct {
		path     []string
		totalDur time.Duration
		count    int
	}
	pathMap := make(map[string]*pathInfo)

	// Look at pairs and triples of consecutive spans
	for i := 0; i < len(spans)-1; i++ {
		// Pair
		pair := []string{spans[i].Name, spans[i+1].Name}
		key := strings.Join(pair, "->")
		if pi, ok := pathMap[key]; ok {
			pi.totalDur += spans[i].Duration + spans[i+1].Duration
			pi.count++
		} else {
			pathMap[key] = &pathInfo{
				path:     pair,
				totalDur: spans[i].Duration + spans[i+1].Duration,
				count:    1,
			}
		}

		// Triple
		if i < len(spans)-2 {
			triple := []string{spans[i].Name, spans[i+1].Name, spans[i+2].Name}
			key := strings.Join(triple, "->")
			if pi, ok := pathMap[key]; ok {
				pi.totalDur += spans[i].Duration + spans[i+1].Duration + spans[i+2].Duration
				pi.count++
			} else {
				pathMap[key] = &pathInfo{
					path:     triple,
					totalDur: spans[i].Duration + spans[i+1].Duration + spans[i+2].Duration,
					count:    1,
				}
			}
		}
	}

	// Sort by total duration descending
	paths := make([]*pathInfo, 0, len(pathMap))
	for _, pi := range pathMap {
		paths = append(paths, pi)
	}
	sort.Slice(paths, func(i, j int) bool {
		return paths[i].totalDur > paths[j].totalDur
	})

	// Return unique paths (deduplicate subpaths contained in longer paths)
	result := make([][]string, 0)
	seen := make(map[string]bool)
	for _, pi := range paths {
		if pi.count < 1 {
			continue
		}
		key := strings.Join(pi.path, "->")
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, pi.path)
		if len(result) >= 5 {
			break
		}
	}

	return result
}

// Recommendations provides optimization suggestions based on profile data.
func (p *Profiler) Recommendations() []string {
	var recs []string

	p.mu.RLock()
	counters := make(map[string]int64)
	for name, c := range p.Counters {
		c.mu.Lock()
		counters[name] = c.Value
		c.mu.Unlock()
	}
	p.mu.RUnlock()

	// Check tool error rate
	toolCalls := counters["tool.calls"]
	toolErrors := counters["tool.errors"]
	if toolCalls > 0 && toolErrors > 0 {
		errorRate := float64(toolErrors) / float64(toolCalls) * 100
		if errorRate > 5.0 {
			recs = append(recs, fmt.Sprintf("Tool error rate above 5%% (%.1f%%) — investigate failing tools", errorRate))
		}
	}

	// Check API latency
	p95API := p.GetP95("api.request")
	if p95API > 3*time.Second {
		recs = append(recs, fmt.Sprintf("P95 API latency (%s) suggests model may be overloaded", formatDuration(p95API)))
	}

	// Check cache hit rate
	cacheHits := counters["cache.hits"]
	cacheTotal := counters["cache.hits"] + counters["cache.misses"]
	if cacheTotal > 0 {
		hitRate := float64(cacheHits) / float64(cacheTotal) * 100
		if hitRate < 20.0 {
			recs = append(recs, fmt.Sprintf("Cache hit rate is low (%.1f%%) — consider adjusting cache strategy", hitRate))
		}
	}

	// Check token efficiency
	inputTokens := counters["tokens.input"]
	outputTokens := counters["tokens.output"]
	if inputTokens > 0 && outputTokens > 0 {
		ratio := float64(inputTokens) / float64(outputTokens)
		if ratio > 10.0 {
			recs = append(recs, fmt.Sprintf("High input/output token ratio (%.1f:1) — consider compacting context", ratio))
		}
	}

	// Check for high P99 variance
	for _, name := range []string{"api.request", "tool.execute"} {
		p50 := p.GetP50(name)
		p99 := p.GetP99(name)
		if p50 > 0 && p99 > 5*p50 {
			recs = append(recs, fmt.Sprintf("High latency variance in %s (P99/P50 = %.1fx) — check for outliers", name, float64(p99)/float64(p50)))
		}
	}

	return recs
}

// Reset clears all profiling data.
func (p *Profiler) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Spans = make([]ProfileSpan, 0)
	p.Counters = make(map[string]*Counter)
	p.Timers = make(map[string]*Timer)
}

// ExportJSON returns a JSON representation of all profiling data.
func (p *Profiler) ExportJSON() string {
	p.mu.RLock()

	type timerExport struct {
		Name    string  `json:"name"`
		Samples int     `json:"samples"`
		P50Ms   float64 `json:"p50_ms"`
		P95Ms   float64 `json:"p95_ms"`
		P99Ms   float64 `json:"p99_ms"`
	}

	type counterExport struct {
		Name  string `json:"name"`
		Value int64  `json:"value"`
	}

	type spanExport struct {
		Name       string            `json:"name"`
		Start      time.Time         `json:"start"`
		End        time.Time         `json:"end"`
		DurationMs float64           `json:"duration_ms"`
		Metadata   map[string]string `json:"metadata,omitempty"`
	}

	type profileExport struct {
		Timers          []timerExport   `json:"timers"`
		Counters        []counterExport `json:"counters"`
		Spans           []spanExport    `json:"spans"`
		HotPaths        [][]string      `json:"hot_paths"`
		Recommendations []string        `json:"recommendations"`
	}

	export := profileExport{
		Timers:   make([]timerExport, 0),
		Counters: make([]counterExport, 0),
		Spans:    make([]spanExport, 0),
	}

	for name, t := range p.Timers {
		t.mu.Lock()
		n := len(t.Samples)
		t.mu.Unlock()
		_ = n
		export.Timers = append(export.Timers, timerExport{
			Name:    name,
			Samples: n,
			P50Ms:   float64(p.GetP50(name)) / float64(time.Millisecond),
			P95Ms:   float64(p.GetP95(name)) / float64(time.Millisecond),
			P99Ms:   float64(p.GetP99(name)) / float64(time.Millisecond),
		})
	}

	for name, c := range p.Counters {
		c.mu.Lock()
		val := c.Value
		c.mu.Unlock()
		export.Counters = append(export.Counters, counterExport{
			Name:  name,
			Value: val,
		})
	}

	for _, s := range p.Spans {
		export.Spans = append(export.Spans, spanExport{
			Name:       s.Name,
			Start:      s.Start,
			End:        s.End,
			DurationMs: float64(s.Duration) / float64(time.Millisecond),
			Metadata:   s.Metadata,
		})
	}

	p.mu.RUnlock()

	export.HotPaths = p.HotPaths()
	export.Recommendations = p.Recommendations()

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

// counterExtra provides contextual information for specific counters.
func (p *Profiler) counterExtra(name string, val int64) string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	switch name {
	case "tool.errors":
		if tc, ok := p.Counters["tool.calls"]; ok {
			tc.mu.Lock()
			total := tc.Value
			tc.mu.Unlock()
			if total > 0 {
				pct := float64(val) / float64(total) * 100
				return fmt.Sprintf("(%.1f%%)", pct)
			}
		}
	case "cache.hits":
		if cm, ok := p.Counters["cache.misses"]; ok {
			cm.mu.Lock()
			misses := cm.Value
			cm.mu.Unlock()
			total := val + misses
			if total > 0 {
				pct := float64(val) / float64(total) * 100
				return fmt.Sprintf("(%.1f%%)", pct)
			}
		}
	}
	return ""
}

// averagePathDuration calculates the average duration for a span sequence.
func (p *Profiler) averagePathDuration(path []string) time.Duration {
	p.mu.RLock()
	spans := make([]ProfileSpan, len(p.Spans))
	copy(spans, p.Spans)
	p.mu.RUnlock()

	if len(path) == 0 || len(spans) < len(path) {
		return 0
	}

	sort.Slice(spans, func(i, j int) bool {
		return spans[i].Start.Before(spans[j].Start)
	})

	var totalDur time.Duration
	var count int

	for i := 0; i <= len(spans)-len(path); i++ {
		match := true
		for j, name := range path {
			if spans[i+j].Name != name {
				match = false
				break
			}
		}
		if match {
			var dur time.Duration
			for j := range path {
				dur += spans[i+j].Duration
			}
			totalDur += dur
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return totalDur / time.Duration(count)
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fµs", float64(d)/float64(time.Microsecond))
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	}
	return fmt.Sprintf("%.1fs", float64(d)/float64(time.Second))
}

// formatInt formats an integer with commas for thousands.
func formatInt(n int64) string {
	if n < 0 {
		return "-" + formatInt(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
		if len(s) > remainder {
			result.WriteString(",")
		}
	}
	for i := remainder; i < len(s); i += 3 {
		result.WriteString(s[i : i+3])
		if i+3 < len(s) {
			result.WriteString(",")
		}
	}
	return result.String()
}
