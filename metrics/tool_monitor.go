package metrics

import (
	"sync"
	"time"
)

// ToolExecution records a single tool invocation.
type ToolExecution struct {
	Name      string
	StartTime time.Time
	Duration  time.Duration
	Success   bool
	Error     string
}

// ToolMonitor tracks real-time tool execution metrics.
type ToolMonitor struct {
	mu          sync.Mutex
	executions  []ToolExecution
	activeCalls map[string]time.Time
}

// NewToolMonitor creates a new monitor.
func NewToolMonitor() *ToolMonitor {
	return &ToolMonitor{activeCalls: make(map[string]time.Time)}
}

// Start records the beginning of a tool call.
func (tm *ToolMonitor) Start(name, callID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.activeCalls[callID] = time.Now()
}

// End records the completion of a tool call.
func (tm *ToolMonitor) End(name, callID string, success bool, errMsg string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	start, ok := tm.activeCalls[callID]
	if !ok {
		start = time.Now()
	}
	delete(tm.activeCalls, callID)
	tm.executions = append(tm.executions, ToolExecution{
		Name:      name,
		StartTime: start,
		Duration:  time.Since(start),
		Success:   success,
		Error:     errMsg,
	})
}

// Stats returns aggregated metrics.
func (tm *ToolMonitor) Stats() ToolStats {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	stats := ToolStats{
		PerTool: make(map[string]*ToolMetrics),
	}
	for _, ex := range tm.executions {
		stats.TotalCalls++
		if ex.Success {
			stats.SuccessCalls++
		} else {
			stats.FailedCalls++
		}
		stats.TotalDuration += ex.Duration

		m, ok := stats.PerTool[ex.Name]
		if !ok {
			m = &ToolMetrics{Name: ex.Name}
			stats.PerTool[ex.Name] = m
		}
		m.Calls++
		m.TotalDuration += ex.Duration
		if !ex.Success {
			m.Failures++
		}
	}
	stats.ActiveCalls = len(tm.activeCalls)
	return stats
}

// ToolStats holds aggregated tool metrics.
type ToolStats struct {
	TotalCalls    int
	SuccessCalls  int
	FailedCalls   int
	ActiveCalls   int
	TotalDuration time.Duration
	PerTool       map[string]*ToolMetrics
}

// ToolMetrics holds per-tool metrics.
type ToolMetrics struct {
	Name          string
	Calls         int
	Failures      int
	TotalDuration time.Duration
}

// AvgDuration returns average call duration for a tool.
func (m *ToolMetrics) AvgDuration() time.Duration {
	if m.Calls == 0 {
		return 0
	}
	return m.TotalDuration / time.Duration(m.Calls)
}

// SuccessRate returns the success rate for a tool.
func (m *ToolMetrics) SuccessRate() float64 {
	if m.Calls == 0 {
		return 0
	}
	return float64(m.Calls-m.Failures) / float64(m.Calls)
}
