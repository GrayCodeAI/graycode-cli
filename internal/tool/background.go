package tool

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BackgroundAgentManager tracks background sub-agent goroutines so the
// engine can wait for them and collect their results after the main LLM
// turn completes.
type BackgroundAgentManager struct {
	mu      sync.Mutex
	cond    *sync.Cond
	agents  map[string]*BackgroundAgent
	results map[string]*BackgroundResult
}

// BackgroundAgent represents a running background sub-agent.
type BackgroundAgent struct {
	ID      string
	Prompt  string
	Started time.Time
}

// BackgroundResult holds the outcome of a completed background sub-agent.
type BackgroundResult struct {
	ID     string
	Prompt string
	Output string
	Err    error
	Done   time.Time
}

// NewBackgroundAgentManager creates a new manager.
func NewBackgroundAgentManager() *BackgroundAgentManager {
	m := &BackgroundAgentManager{
		agents:  make(map[string]*BackgroundAgent),
		results: make(map[string]*BackgroundResult),
	}
	m.cond = sync.NewCond(&m.mu)
	return m
}

// Spawn starts a background sub-agent goroutine. The agent runs the
// spawnFn asynchronously and stores the result when complete.
func (m *BackgroundAgentManager) Spawn(ctx context.Context, id, prompt string, spawnFn func(ctx context.Context, prompt string) (string, error)) {
	m.mu.Lock()
	m.agents[id] = &BackgroundAgent{
		ID:      id,
		Prompt:  prompt,
		Started: time.Now(),
	}
	m.mu.Unlock()

	go func() {
		output, err := spawnFn(ctx, prompt)

		m.mu.Lock()
		delete(m.agents, id)
		m.results[id] = &BackgroundResult{
			ID:     id,
			Prompt: prompt,
			Output: output,
			Err:    err,
			Done:   time.Now(),
		}
		m.cond.Broadcast()
		m.mu.Unlock()
	}()
}

// HasPending returns true if any background agents are still running.
func (m *BackgroundAgentManager) HasPending() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.agents) > 0
}

// WaitForResults blocks until all pending agents complete or the timeout
// is reached. Returns all collected results (including any that completed
// before this call).
func (m *BackgroundAgentManager) WaitForResults(timeout time.Duration) []*BackgroundResult {
	deadline := time.Now().Add(timeout)
	m.mu.Lock()
	defer m.mu.Unlock()
	for len(m.agents) > 0 {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		// Use a timer to wake up on timeout even if no agent completes.
		timer := time.AfterFunc(remaining, func() {
			m.mu.Lock()
			m.cond.Broadcast()
			m.mu.Unlock()
		})
		m.cond.Wait()
		timer.Stop()
	}

	results := make([]*BackgroundResult, 0, len(m.results))
	for _, r := range m.results {
		results = append(results, r)
	}
	return results
}

// CollectResults returns and clears all completed results.
func (m *BackgroundAgentManager) CollectResults() []*BackgroundResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	results := make([]*BackgroundResult, 0, len(m.results))
	for _, r := range m.results {
		results = append(results, r)
	}
	m.results = make(map[string]*BackgroundResult)
	return results
}

// GetResult returns the result for a specific agent ID, if completed.
func (m *BackgroundAgentManager) GetResult(id string) (*BackgroundResult, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.results[id]
	return r, ok
}

// IsRunning returns true if the agent with the given ID is still running.
func (m *BackgroundAgentManager) IsRunning(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.agents[id]
	return ok
}

// Elapsed returns the elapsed time for a running agent.
func (m *BackgroundAgentManager) Elapsed(id string) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.agents[id]; ok {
		return time.Since(a.Started)
	}
	return 0
}

// FormatResults returns a human-readable summary of background results
// suitable for injection into the conversation.
func FormatResults(results []*BackgroundResult) string {
	if len(results) == 0 {
		return ""
	}
	out := fmt.Sprintf("[Background sub-agent results: %d completed]\n\n", len(results))
	for _, r := range results {
		if r.Err != nil {
			out += fmt.Sprintf("--- Agent %s (error) ---\n%s\n\n", r.ID, r.Err.Error())
		} else {
			summary := r.Output
			if len(summary) > 500 {
				summary = summary[:500] + "...(truncated)"
			}
			out += fmt.Sprintf("--- Agent %s ---\n%s\n\n", r.ID, summary)
		}
	}
	return out
}
