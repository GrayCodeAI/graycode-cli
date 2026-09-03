package agent

import (
	"context"
	"strings"
	"sync"
	"time"

	agentcontracts "github.com/GrayCodeAI/eagle/agent"

	"github.com/GrayCodeAI/graycode-cli/internal/taskruntime"
)

// BackgroundAgentPool manages async sub-agents that run in the background.
// PACK-02: backed by taskruntime.Registry (shared with tool.BackgroundAgentManager).
//
// Deprecated: new code should use Session.SpawnController() (Spawn /
// SpawnBackground / Tasks) which shares the same taskruntime.Registry via
// ToolService.EnsureBackgroundManager. This pool is retained for older
// callers and tests.
type BackgroundAgentPool struct {
	mu      sync.Mutex
	reg     *taskruntime.Registry
	results []BackgroundResult
	maxWait time.Duration
	// parent is the context every background agent derives its cancellable
	// context from. When the owning session ends, call Stop() to cancel all
	// in-flight agents. Previously Submit used context.Background(), so
	// agents could never be cancelled and leaked past session teardown (C8).
	parent context.Context
	// cancels tracks the per-agent cancel functions so Stop()/completion can
	// release them. Keyed by task ID.
	cancels map[string]context.CancelFunc
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
// Agents derive their contexts from context.Background() unless
// NewBackgroundAgentPoolWithContext is used. Call Stop() when the owning
// session ends to cancel any in-flight agents.
func NewBackgroundAgentPool() *BackgroundAgentPool {
	return NewBackgroundAgentPoolWithContext(context.Background())
}

// NewBackgroundAgentPoolWithContext creates a pool whose background agents
// derive their cancellable contexts from parent. Cancelling the parent (e.g.
// via session teardown) or calling Stop() cancels every in-flight agent.
func NewBackgroundAgentPoolWithContext(parent context.Context) *BackgroundAgentPool {
	return &BackgroundAgentPool{
		reg:     taskruntime.New(),
		maxWait: 2 * time.Minute,
		parent:  parent,
		cancels: make(map[string]context.CancelFunc),
	}
}

// Submit launches a background sub-agent. The spawn function runs asynchronously.
func (p *BackgroundAgentPool) Submit(id, prompt string, spawn func(ctx context.Context, prompt string) (string, error)) {
	if p.parent == nil {
		p.parent = context.Background()
	}
	ctx, cancel := context.WithCancel(p.parent)
	p.mu.Lock()
	if p.cancels == nil {
		p.cancels = make(map[string]context.CancelFunc)
	}
	p.cancels[id] = cancel
	p.mu.Unlock()
	req := agentcontracts.SpawnRequest{Prompt: prompt, Background: true}
	fn := func(ctx context.Context, r agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
		defer p.releaseCancel(id)
		out, err := spawn(ctx, r.Prompt)
		if err != nil {
			return agentcontracts.SpawnResult{Status: agentcontracts.StatusFailed, Error: err.Error()}, err
		}
		return agentcontracts.SpawnResult{Status: agentcontracts.StatusCompleted, Output: out}, nil
	}
	p.reg.SpawnAgent(ctx, id, req, fn)
}

// releaseCancel cancels and forgets the cancel func for a finished task so
// the pool does not accumulate entries for every completed background agent.
func (p *BackgroundAgentPool) releaseCancel(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancels != nil {
		if c, ok := p.cancels[id]; ok {
			c()
			delete(p.cancels, id)
		}
	}
}

// Stop cancels every in-flight background agent and releases their context
// resources. Safe to call multiple times. Call this during session teardown
// so background agents do not outlive the session that spawned them (C8).
func (p *BackgroundAgentPool) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, c := range p.cancels {
		c()
		delete(p.cancels, id)
	}
}

// Collect gathers all completed background results without blocking.
func (p *BackgroundAgentPool) Collect() []BackgroundResult {
	completed := p.reg.CollectCompleted()
	var out []BackgroundResult
	for _, t := range completed {
		br := toPoolResult(t)
		out = append(out, br)
		p.mu.Lock()
		p.results = append(p.results, br)
		p.mu.Unlock()
	}
	return out
}

// WaitAll blocks until all pending tasks complete or timeout.
func (p *BackgroundAgentPool) WaitAll() []BackgroundResult {
	tasks := p.reg.Wait(p.maxWait)
	var all []BackgroundResult
	for _, t := range tasks {
		// Wait returns done map snapshot; also drain collect
		all = append(all, toPoolResult(t))
	}
	// Clear done via CollectCompleted so WaitAll is not sticky forever
	_ = p.reg.CollectCompleted()
	p.mu.Lock()
	p.results = append(p.results, all...)
	p.mu.Unlock()
	return all
}

// HasPending returns true if background agents are still running.
func (p *BackgroundAgentPool) HasPending() bool {
	return p.reg.HasPending()
}

// PendingCount returns the number of in-flight background agents.
func (p *BackgroundAgentPool) PendingCount() int {
	return p.reg.PendingCount()
}

// AllResults returns all results collected so far (completed background tasks).
func (p *BackgroundAgentPool) AllResults() []BackgroundResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]BackgroundResult, len(p.results))
	copy(out, p.results)
	return out
}

// ClearResults clears all collected results to free memory.
func (p *BackgroundAgentPool) ClearResults() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.results = nil
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

func toPoolResult(t *taskruntime.Task) BackgroundResult {
	br := BackgroundResult{
		ID:      t.ID,
		Prompt:  t.Prompt,
		Output:  t.Output,
		Elapsed: t.DoneAt.Sub(t.StartedAt),
	}
	if t.Error != "" {
		br.Error = context.DeadlineExceeded
		if t.Status != taskruntime.StatusKilled {
			br.Error = errString(t.Error)
		}
	}
	return br
}

type stringError string

func (e stringError) Error() string { return string(e) }

func errString(s string) error { return stringError(s) }
