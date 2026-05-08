package engine

import (
	"context"
	"strings"
	"sync"
	"time"
)

// BackgroundAgentPool manages async sub-agents that run in the background.
// When background agents finish, their results are collected for re-injection
// into the main agent loop (inspired by herm's completion cycles).
type BackgroundAgentPool struct {
	mu       sync.Mutex
	pending  []backgroundTask
	results  []BackgroundResult
	maxWait  time.Duration
	maxCycles int
}

type backgroundTask struct {
	id     string
	prompt string
	cancel context.CancelFunc
	done   chan BackgroundResult
}

// BackgroundResult holds the output of a completed background agent.
type BackgroundResult struct {
	ID      string
	Prompt  string
	Output  string
	Error   error
	Elapsed time.Duration
}

// NewBackgroundAgentPool creates a pool with configurable wait limits.
func NewBackgroundAgentPool() *BackgroundAgentPool {
	return &BackgroundAgentPool{
		maxWait:   2 * time.Minute,
		maxCycles: 3,
	}
}

// Submit launches a background sub-agent. The spawn function runs asynchronously.
func (p *BackgroundAgentPool) Submit(id, prompt string, spawn func(ctx context.Context, prompt string) (string, error)) {
	ctx, cancel := context.WithTimeout(context.Background(), p.maxWait)
	done := make(chan BackgroundResult, 1)

	task := backgroundTask{id: id, prompt: prompt, cancel: cancel, done: done}
	p.mu.Lock()
	p.pending = append(p.pending, task)
	p.mu.Unlock()

	go func() {
		start := time.Now()
		output, err := spawn(ctx, prompt)
		done <- BackgroundResult{
			ID:      id,
			Prompt:  prompt,
			Output:  output,
			Error:   err,
			Elapsed: time.Since(start),
		}
		cancel()
	}()
}

// Collect gathers all completed background results without blocking.
// Returns immediately with whatever results are available.
func (p *BackgroundAgentPool) Collect() []BackgroundResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	var completed []BackgroundResult
	var stillPending []backgroundTask

	for _, task := range p.pending {
		select {
		case result := <-task.done:
			completed = append(completed, result)
		default:
			stillPending = append(stillPending, task)
		}
	}

	p.pending = stillPending
	p.results = append(p.results, completed...)
	return completed
}

// WaitAll blocks until all pending tasks complete or timeout.
func (p *BackgroundAgentPool) WaitAll() []BackgroundResult {
	p.mu.Lock()
	pending := make([]backgroundTask, len(p.pending))
	copy(pending, p.pending)
	p.mu.Unlock()

	var all []BackgroundResult
	for _, task := range pending {
		select {
		case result := <-task.done:
			all = append(all, result)
		case <-time.After(p.maxWait):
			all = append(all, BackgroundResult{
				ID:     task.id,
				Prompt: task.prompt,
				Error:  context.DeadlineExceeded,
			})
			task.cancel()
		}
	}

	p.mu.Lock()
	p.pending = nil
	p.results = append(p.results, all...)
	p.mu.Unlock()
	return all
}

// HasPending returns true if background agents are still running.
func (p *BackgroundAgentPool) HasPending() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pending) > 0
}

// PendingCount returns the number of in-flight background agents.
func (p *BackgroundAgentPool) PendingCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pending)
}

// AllResults returns all results collected so far (completed background tasks).
func (p *BackgroundAgentPool) AllResults() []BackgroundResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]BackgroundResult, len(p.results))
	copy(out, p.results)
	return out
}

// FormatResults formats background results for injection into the agent context.
func FormatResults(results []BackgroundResult) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Background research completed:\n\n")
	for _, r := range results {
		b.WriteString("## Task: " + r.Prompt + "\n")
		if r.Error != nil {
			b.WriteString("Error: " + r.Error.Error() + "\n\n")
		} else {
			b.WriteString(r.Output + "\n\n")
		}
	}
	return b.String()
}
