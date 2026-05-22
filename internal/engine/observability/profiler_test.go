package observability

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewProfiler(t *testing.T) {
	p := NewProfiler()
	if p == nil {
		t.Fatal("NewProfiler returned nil")
	}
	if !p.Enabled {
		t.Error("expected Enabled to be true")
	}
	if p.Counters == nil {
		t.Error("expected Counters to be initialized")
	}
	if p.Timers == nil {
		t.Error("expected Timers to be initialized")
	}
	if p.Spans == nil {
		t.Error("expected Spans to be initialized")
	}
}

func TestStartAndEndSpan(t *testing.T) {
	p := NewProfiler()

	span := p.StartSpan("test.operation")
	if span == nil {
		t.Fatal("StartSpan returned nil")
	}
	if span.Name != "test.operation" {
		t.Errorf("expected span name 'test.operation', got '%s'", span.Name)
	}
	if span.Start.IsZero() {
		t.Error("expected non-zero start time")
	}
	if span.Metadata == nil {
		t.Error("expected Metadata to be initialized")
	}

	time.Sleep(10 * time.Millisecond)
	p.EndSpan(span)

	if span.End.IsZero() {
		t.Error("expected non-zero end time")
	}
	if span.Duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", span.Duration)
	}

	if len(p.Spans) != 1 {
		t.Errorf("expected 1 span, got %d", len(p.Spans))
	}
	if p.Spans[0].Name != "test.operation" {
		t.Errorf("expected recorded span name 'test.operation', got '%s'", p.Spans[0].Name)
	}
}

func TestEndSpanNil(t *testing.T) {
	p := NewProfiler()
	// Should not panic
	p.EndSpan(nil)
}

func TestIncrement(t *testing.T) {
	p := NewProfiler()

	p.Increment("api.requests", 1)
	p.Increment("api.requests", 1)
	p.Increment("api.requests", 1)
	p.Increment("tokens.input", 1000)

	p.mu.RLock()
	apiC := p.Counters["api.requests"]
	tokC := p.Counters["tokens.input"]
	p.mu.RUnlock()

	apiC.mu.Lock()
	if apiC.Value != 3 {
		t.Errorf("expected api.requests=3, got %d", apiC.Value)
	}
	apiC.mu.Unlock()

	tokC.mu.Lock()
	if tokC.Value != 1000 {
		t.Errorf("expected tokens.input=1000, got %d", tokC.Value)
	}
	tokC.mu.Unlock()
}

func TestIncrementConcurrent(t *testing.T) {
	p := NewProfiler()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Increment("concurrent.counter", 1)
		}()
	}
	wg.Wait()

	p.mu.RLock()
	c := p.Counters["concurrent.counter"]
	p.mu.RUnlock()

	c.mu.Lock()
	if c.Value != 100 {
		t.Errorf("expected 100, got %d", c.Value)
	}
	c.mu.Unlock()
}

func TestRecordDuration(t *testing.T) {
	p := NewProfiler()

	p.RecordDuration("api.request", 100*time.Millisecond)
	p.RecordDuration("api.request", 200*time.Millisecond)
	p.RecordDuration("api.request", 300*time.Millisecond)

	p.mu.RLock()
	timer := p.Timers["api.request"]
	p.mu.RUnlock()

	timer.mu.Lock()
	if len(timer.Samples) != 3 {
		t.Errorf("expected 3 samples, got %d", len(timer.Samples))
	}
	timer.mu.Unlock()
}

func TestGetPercentiles(t *testing.T) {
	p := NewProfiler()

	// Add 100 samples from 1ms to 100ms
	for i := 1; i <= 100; i++ {
		p.RecordDuration("latency", time.Duration(i)*time.Millisecond)
	}

	p50 := p.GetP50("latency")
	p95 := p.GetP95("latency")
	p99 := p.GetP99("latency")

	// P50 should be around 50ms
	if p50 < 49*time.Millisecond || p50 > 51*time.Millisecond {
		t.Errorf("expected P50 around 50ms, got %v", p50)
	}

	// P95 should be around 95ms
	if p95 < 94*time.Millisecond || p95 > 96*time.Millisecond {
		t.Errorf("expected P95 around 95ms, got %v", p95)
	}

	// P99 should be around 99ms
	if p99 < 98*time.Millisecond || p99 > 100*time.Millisecond {
		t.Errorf("expected P99 around 99ms, got %v", p99)
	}
}

func TestGetPercentilesNonExistent(t *testing.T) {
	p := NewProfiler()

	if p.GetP50("nonexistent") != 0 {
		t.Error("expected 0 for nonexistent timer")
	}
	if p.GetP95("nonexistent") != 0 {
		t.Error("expected 0 for nonexistent timer")
	}
	if p.GetP99("nonexistent") != 0 {
		t.Error("expected 0 for nonexistent timer")
	}
}

func TestGetPercentilesSingleSample(t *testing.T) {
	p := NewProfiler()
	p.RecordDuration("single", 42*time.Millisecond)

	if p.GetP50("single") != 42*time.Millisecond {
		t.Errorf("expected 42ms, got %v", p.GetP50("single"))
	}
	if p.GetP95("single") != 42*time.Millisecond {
		t.Errorf("expected 42ms, got %v", p.GetP95("single"))
	}
	if p.GetP99("single") != 42*time.Millisecond {
		t.Errorf("expected 42ms, got %v", p.GetP99("single"))
	}
}

func TestReport(t *testing.T) {
	p := NewProfiler()

	// Add timer data
	for i := 0; i < 50; i++ {
		p.RecordDuration("api.request", time.Duration(100+i*10)*time.Millisecond)
	}
	for i := 0; i < 100; i++ {
		p.RecordDuration("tool.execute", time.Duration(50+i*5)*time.Millisecond)
	}

	// Add counter data
	p.Increment("api.requests", 50)
	p.Increment("tool.calls", 100)
	p.Increment("tool.errors", 7)
	p.Increment("tokens.input", 234000)
	p.Increment("tokens.output", 45000)
	p.Increment("cache.hits", 12)
	p.Increment("cache.misses", 33)

	report := p.Report()

	// Check report structure
	if !strings.Contains(report, "Performance Profile:") {
		t.Error("report missing header")
	}
	if !strings.Contains(report, "═══════════════════════════════════════════") {
		t.Error("report missing separator")
	}
	if !strings.Contains(report, "Timing:") {
		t.Error("report missing Timing section")
	}
	if !strings.Contains(report, "Counters:") {
		t.Error("report missing Counters section")
	}
	if !strings.Contains(report, "api.request:") {
		t.Error("report missing api.request timer")
	}
	if !strings.Contains(report, "tool.execute:") {
		t.Error("report missing tool.execute timer")
	}
	if !strings.Contains(report, "P50=") {
		t.Error("report missing P50")
	}
	if !strings.Contains(report, "P95=") {
		t.Error("report missing P95")
	}
	if !strings.Contains(report, "P99=") {
		t.Error("report missing P99")
	}
	if !strings.Contains(report, "234,000") {
		t.Error("report missing formatted token count")
	}
}

func TestReportWithRecommendations(t *testing.T) {
	p := NewProfiler()

	// Create conditions for recommendations
	p.Increment("tool.calls", 100)
	p.Increment("tool.errors", 10) // 10% error rate

	// Add high P95 latency
	for i := 0; i < 20; i++ {
		p.RecordDuration("api.request", 4*time.Second)
	}

	report := p.Report()
	if !strings.Contains(report, "Recommendations:") {
		t.Error("report missing Recommendations section")
	}
	if !strings.Contains(report, "Tool error rate above 5%") {
		t.Error("report missing tool error recommendation")
	}
	if !strings.Contains(report, "P95 API latency") {
		t.Error("report missing P95 latency recommendation")
	}
}

func TestHotPaths(t *testing.T) {
	p := NewProfiler()

	// Create a sequence of spans
	now := time.Now()
	p.mu.Lock()
	p.Spans = append(p.Spans, ProfileSpan{
		Name:     "api.request",
		Start:    now,
		End:      now.Add(100 * time.Millisecond),
		Duration: 100 * time.Millisecond,
		Metadata: make(map[string]string),
	})
	p.Spans = append(p.Spans, ProfileSpan{
		Name:     "tool.execute",
		Start:    now.Add(100 * time.Millisecond),
		End:      now.Add(200 * time.Millisecond),
		Duration: 100 * time.Millisecond,
		Metadata: make(map[string]string),
	})
	p.Spans = append(p.Spans, ProfileSpan{
		Name:     "api.request",
		Start:    now.Add(200 * time.Millisecond),
		End:      now.Add(400 * time.Millisecond),
		Duration: 200 * time.Millisecond,
		Metadata: make(map[string]string),
	})
	p.mu.Unlock()

	paths := p.HotPaths()
	if len(paths) == 0 {
		t.Fatal("expected at least one hot path")
	}

	// The triple should be present
	found := false
	for _, path := range paths {
		if len(path) == 3 && path[0] == "api.request" && path[1] == "tool.execute" && path[2] == "api.request" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected hot path [api.request, tool.execute, api.request]")
	}
}

func TestHotPathsEmpty(t *testing.T) {
	p := NewProfiler()
	paths := p.HotPaths()
	if paths != nil {
		t.Errorf("expected nil hot paths for empty profiler, got %v", paths)
	}
}

func TestHotPathsSingleSpan(t *testing.T) {
	p := NewProfiler()
	now := time.Now()
	p.mu.Lock()
	p.Spans = append(p.Spans, ProfileSpan{
		Name:     "api.request",
		Start:    now,
		End:      now.Add(100 * time.Millisecond),
		Duration: 100 * time.Millisecond,
		Metadata: make(map[string]string),
	})
	p.mu.Unlock()

	paths := p.HotPaths()
	if paths != nil {
		t.Errorf("expected nil hot paths for single span, got %v", paths)
	}
}

func TestRecommendationsHighErrorRate(t *testing.T) {
	p := NewProfiler()
	p.Increment("tool.calls", 100)
	p.Increment("tool.errors", 10) // 10% error rate

	recs := p.Recommendations()
	found := false
	for _, rec := range recs {
		if strings.Contains(rec, "Tool error rate above 5%") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected recommendation about high tool error rate")
	}
}

func TestRecommendationsHighLatency(t *testing.T) {
	p := NewProfiler()
	for i := 0; i < 20; i++ {
		p.RecordDuration("api.request", 4*time.Second)
	}

	recs := p.Recommendations()
	found := false
	for _, rec := range recs {
		if strings.Contains(rec, "P95 API latency") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected recommendation about high API latency")
	}
}

func TestRecommendationsLowCacheHitRate(t *testing.T) {
	p := NewProfiler()
	p.Increment("cache.hits", 5)
	p.Increment("cache.misses", 95)

	recs := p.Recommendations()
	found := false
	for _, rec := range recs {
		if strings.Contains(rec, "Cache hit rate is low") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected recommendation about low cache hit rate")
	}
}

func TestRecommendationsHighTokenRatio(t *testing.T) {
	p := NewProfiler()
	p.Increment("tokens.input", 100000)
	p.Increment("tokens.output", 5000) // ratio of 20:1

	recs := p.Recommendations()
	found := false
	for _, rec := range recs {
		if strings.Contains(rec, "input/output token ratio") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected recommendation about high token ratio")
	}
}

func TestRecommendationsEmpty(t *testing.T) {
	p := NewProfiler()
	recs := p.Recommendations()
	if len(recs) != 0 {
		t.Errorf("expected no recommendations for empty profiler, got %v", recs)
	}
}

func TestProfilerReset(t *testing.T) {
	p := NewProfiler()

	p.Increment("counter", 10)
	p.RecordDuration("timer", 100*time.Millisecond)
	span := p.StartSpan("test")
	p.EndSpan(span)

	p.Reset()

	if len(p.Spans) != 0 {
		t.Errorf("expected 0 spans after reset, got %d", len(p.Spans))
	}
	if len(p.Counters) != 0 {
		t.Errorf("expected 0 counters after reset, got %d", len(p.Counters))
	}
	if len(p.Timers) != 0 {
		t.Errorf("expected 0 timers after reset, got %d", len(p.Timers))
	}
}

func TestExportJSON(t *testing.T) {
	p := NewProfiler()

	p.Increment("api.requests", 10)
	p.RecordDuration("api.request", 100*time.Millisecond)
	p.RecordDuration("api.request", 200*time.Millisecond)

	now := time.Now()
	p.mu.Lock()
	p.Spans = append(p.Spans, ProfileSpan{
		Name:     "api.request",
		Start:    now,
		End:      now.Add(100 * time.Millisecond),
		Duration: 100 * time.Millisecond,
		Metadata: map[string]string{"model": "claude"},
	})
	p.Spans = append(p.Spans, ProfileSpan{
		Name:     "tool.execute",
		Start:    now.Add(100 * time.Millisecond),
		End:      now.Add(200 * time.Millisecond),
		Duration: 100 * time.Millisecond,
		Metadata: map[string]string{},
	})
	p.mu.Unlock()

	jsonStr := p.ExportJSON()
	if jsonStr == "" || jsonStr == "{}" {
		t.Fatal("ExportJSON returned empty or error result")
	}

	// Validate JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("ExportJSON produced invalid JSON: %v", err)
	}

	// Check fields
	if _, ok := result["timers"]; !ok {
		t.Error("JSON missing 'timers' field")
	}
	if _, ok := result["counters"]; !ok {
		t.Error("JSON missing 'counters' field")
	}
	if _, ok := result["spans"]; !ok {
		t.Error("JSON missing 'spans' field")
	}
	if _, ok := result["hot_paths"]; !ok {
		t.Error("JSON missing 'hot_paths' field")
	}
	if _, ok := result["recommendations"]; !ok {
		t.Error("JSON missing 'recommendations' field")
	}
}

func TestExportJSONEmpty(t *testing.T) {
	p := NewProfiler()
	jsonStr := p.ExportJSON()

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("ExportJSON produced invalid JSON for empty profiler: %v", err)
	}
}

func TestSpanMetadata(t *testing.T) {
	p := NewProfiler()

	span := p.StartSpan("api.request")
	span.Metadata["model"] = "claude-3-opus"
	span.Metadata["tokens"] = "1500"
	time.Sleep(5 * time.Millisecond)
	p.EndSpan(span)

	if len(p.Spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(p.Spans))
	}
	if p.Spans[0].Metadata["model"] != "claude-3-opus" {
		t.Error("metadata not preserved in span")
	}
}

func TestSpanChildren(t *testing.T) {
	p := NewProfiler()

	parent := p.StartSpan("parent")
	child := p.StartSpan("child")
	time.Sleep(5 * time.Millisecond)
	p.EndSpan(child)
	parent.Children = append(parent.Children, *child)
	p.EndSpan(parent)

	if len(p.Spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(p.Spans))
	}
}

func TestConcurrentSpans(t *testing.T) {
	p := NewProfiler()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			span := p.StartSpan("concurrent.op")
			time.Sleep(time.Millisecond)
			p.EndSpan(span)
		}()
	}
	wg.Wait()

	p.mu.RLock()
	if len(p.Spans) != 50 {
		t.Errorf("expected 50 spans, got %d", len(p.Spans))
	}
	p.mu.RUnlock()
}

func TestConcurrentRecordDuration(t *testing.T) {
	p := NewProfiler()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p.RecordDuration("concurrent.timer", time.Duration(i)*time.Millisecond)
		}(i)
	}
	wg.Wait()

	p.mu.RLock()
	timer := p.Timers["concurrent.timer"]
	p.mu.RUnlock()

	timer.mu.Lock()
	if len(timer.Samples) != 100 {
		t.Errorf("expected 100 samples, got %d", len(timer.Samples))
	}
	timer.mu.Unlock()
}

func TestProfilerFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "0s"},
		{500 * time.Microsecond, "500.0µs"},
		{1500 * time.Microsecond, "1.5ms"},
		{100 * time.Millisecond, "100.0ms"},
		{1500 * time.Millisecond, "1.5s"},
		{3 * time.Second, "3.0s"},
	}

	for _, tc := range tests {
		result := formatDuration(tc.input)
		if result != tc.expected {
			t.Errorf("formatDuration(%v) = %s, expected %s", tc.input, result, tc.expected)
		}
	}
}

func TestFormatInt(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1000000, "1,000,000"},
		{234000, "234,000"},
		{45000, "45,000"},
		{-1000, "-1,000"},
	}

	for _, tc := range tests {
		result := formatInt(tc.input)
		if result != tc.expected {
			t.Errorf("formatInt(%d) = %s, expected %s", tc.input, result, tc.expected)
		}
	}
}

func TestCounterExtra(t *testing.T) {
	p := NewProfiler()
	p.Increment("tool.calls", 100)
	p.Increment("tool.errors", 7)
	p.Increment("cache.hits", 12)
	p.Increment("cache.misses", 33)

	extra := p.counterExtra("tool.errors", 7)
	if !strings.Contains(extra, "7.0%") {
		t.Errorf("expected tool.errors extra to contain '7.0%%', got '%s'", extra)
	}

	extra = p.counterExtra("cache.hits", 12)
	if !strings.Contains(extra, "26.7%") {
		t.Errorf("expected cache.hits extra to contain '26.7%%', got '%s'", extra)
	}

	extra = p.counterExtra("api.requests", 50)
	if extra != "" {
		t.Errorf("expected empty extra for api.requests, got '%s'", extra)
	}
}

func TestAveragePathDuration(t *testing.T) {
	p := NewProfiler()
	now := time.Now()

	p.mu.Lock()
	// First sequence: api.request -> tool.execute (100ms + 50ms)
	p.Spans = append(p.Spans, ProfileSpan{
		Name:     "api.request",
		Start:    now,
		End:      now.Add(100 * time.Millisecond),
		Duration: 100 * time.Millisecond,
		Metadata: make(map[string]string),
	})
	p.Spans = append(p.Spans, ProfileSpan{
		Name:     "tool.execute",
		Start:    now.Add(100 * time.Millisecond),
		End:      now.Add(150 * time.Millisecond),
		Duration: 50 * time.Millisecond,
		Metadata: make(map[string]string),
	})
	// Second sequence: api.request -> tool.execute (200ms + 100ms)
	p.Spans = append(p.Spans, ProfileSpan{
		Name:     "api.request",
		Start:    now.Add(200 * time.Millisecond),
		End:      now.Add(400 * time.Millisecond),
		Duration: 200 * time.Millisecond,
		Metadata: make(map[string]string),
	})
	p.Spans = append(p.Spans, ProfileSpan{
		Name:     "tool.execute",
		Start:    now.Add(400 * time.Millisecond),
		End:      now.Add(500 * time.Millisecond),
		Duration: 100 * time.Millisecond,
		Metadata: make(map[string]string),
	})
	p.mu.Unlock()

	avg := p.averagePathDuration([]string{"api.request", "tool.execute"})
	// Average should be ((100+50) + (200+100)) / 2 = 225ms
	expected := 225 * time.Millisecond
	if avg != expected {
		t.Errorf("expected average path duration %v, got %v", expected, avg)
	}
}

func TestReportHotPathsSection(t *testing.T) {
	p := NewProfiler()
	now := time.Now()

	p.mu.Lock()
	p.Spans = append(p.Spans, ProfileSpan{
		Name:     "api.request",
		Start:    now,
		End:      now.Add(100 * time.Millisecond),
		Duration: 100 * time.Millisecond,
		Metadata: make(map[string]string),
	})
	p.Spans = append(p.Spans, ProfileSpan{
		Name:     "tool.execute",
		Start:    now.Add(100 * time.Millisecond),
		End:      now.Add(200 * time.Millisecond),
		Duration: 100 * time.Millisecond,
		Metadata: make(map[string]string),
	})
	p.mu.Unlock()

	report := p.Report()
	if !strings.Contains(report, "Hot Paths:") {
		t.Error("report missing Hot Paths section")
	}
	if !strings.Contains(report, "→") {
		t.Error("report missing path arrow notation")
	}
}

func TestProfilerIntegration(t *testing.T) {
	p := NewProfiler()

	// Simulate a typical agent workflow
	for i := 0; i < 10; i++ {
		// API call
		apiSpan := p.StartSpan("api.request")
		apiSpan.Metadata["model"] = "claude-3-opus"
		time.Sleep(2 * time.Millisecond)
		p.EndSpan(apiSpan)
		p.Increment("api.requests", 1)
		p.Increment("tokens.input", 1500)
		p.Increment("tokens.output", 500)

		// Tool execution
		toolSpan := p.StartSpan("tool.execute")
		time.Sleep(time.Millisecond)
		p.EndSpan(toolSpan)
		p.Increment("tool.calls", 1)
		if i%7 == 0 {
			p.Increment("tool.errors", 1)
		}
	}

	// Verify report generation
	report := p.Report()
	if report == "" {
		t.Fatal("expected non-empty report")
	}

	// Verify JSON export
	jsonStr := p.ExportJSON()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("integration test produced invalid JSON: %v", err)
	}

	// Verify hot paths exist
	paths := p.HotPaths()
	if len(paths) == 0 {
		t.Error("expected hot paths from integration workflow")
	}

	// Verify reset works
	p.Reset()
	if len(p.Spans) != 0 || len(p.Counters) != 0 || len(p.Timers) != 0 {
		t.Error("reset did not clear all data")
	}
}
