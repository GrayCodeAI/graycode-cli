package metrics

import "sync"

// ToolAggregate holds per-tool aggregate statistics collected during a session.
type ToolAggregate struct {
	CallCount  int
	TotalBytes int64
	TotalMs    int64
	Errors     int
}

// ToolAggregator collects per-tool call statistics in a thread-safe manner.
// Inspired by herm's TraceToolSummary pattern for per-tool aggregates.
type ToolAggregator struct {
	mu    sync.Mutex
	tools map[string]*ToolAggregate
}

// NewToolAggregator creates a new ToolAggregator ready for use.
func NewToolAggregator() *ToolAggregator {
	return &ToolAggregator{
		tools: make(map[string]*ToolAggregate),
	}
}

// Record records a tool invocation with its byte count, duration, and error status.
func (a *ToolAggregator) Record(toolName string, bytes int64, durationMs int64, isError bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	agg, ok := a.tools[toolName]
	if !ok {
		agg = &ToolAggregate{}
		a.tools[toolName] = agg
	}

	agg.CallCount++
	agg.TotalBytes += bytes
	agg.TotalMs += durationMs
	if isError {
		agg.Errors++
	}
}

// Get returns the aggregate for a specific tool, or nil if not recorded.
func (a *ToolAggregator) Get(toolName string) *ToolAggregate {
	a.mu.Lock()
	defer a.mu.Unlock()

	agg, ok := a.tools[toolName]
	if !ok {
		return nil
	}
	// Return a copy to avoid races after unlock.
	cp := *agg
	return &cp
}

// All returns a snapshot of all tool aggregates. The returned map is a copy.
func (a *ToolAggregator) All() map[string]*ToolAggregate {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := make(map[string]*ToolAggregate, len(a.tools))
	for name, agg := range a.tools {
		cp := *agg
		result[name] = &cp
	}
	return result
}

// Reset clears all recorded tool aggregates.
func (a *ToolAggregator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tools = make(map[string]*ToolAggregate)
}
