package engine

import (
	"context"
	"fmt"
	"time"

	agentcontracts "github.com/GrayCodeAI/eagle/agent"

	"github.com/GrayCodeAI/hawk/internal/taskruntime"
)

// SpawnController is the single entrypoint for subagent spawn, background
// agents, and task lookup. Faces and tools should use this instead of
// reaching into BackgroundAgentManager / agent loops separately.
//
// It wraps Session.spawnSubAgentRequest and shares the session's
// taskruntime.Registry via ToolService's BackgroundAgentManager.
type SpawnController struct {
	session *Session
}

// SpawnController returns the session-scoped spawn facade (always non-nil
// for a live session; methods no-op safely if session is nil).
func (s *Session) SpawnController() *SpawnController {
	if s == nil {
		return &SpawnController{}
	}
	return &SpawnController{session: s}
}

// Tasks returns the unified background task registry, creating a background
// manager on the tool service if needed.
func (c *SpawnController) Tasks() *taskruntime.Registry {
	if c == nil || c.session == nil || c.session.Tools() == nil {
		return nil
	}
	bm := c.session.Tools().EnsureBackgroundManager()
	if bm == nil {
		return nil
	}
	return bm.Registry()
}

// Spawn runs a typed subagent to completion (or failure).
func (c *SpawnController) Spawn(ctx context.Context, req agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
	if c == nil || c.session == nil {
		return agentcontracts.SpawnResult{Status: agentcontracts.StatusFailed, Error: "spawn: no session"}, fmt.Errorf("spawn: no session")
	}
	// Ensure WireAgentTool path exists.
	if c.session.Tools() != nil && c.session.Tools().AgentSpawnFn() == nil {
		c.session.WireAgentTool()
	}
	return c.session.spawnSubAgentRequest(ctx, req, 0)
}

// SpawnBackground starts a subagent on the unified task registry and returns its id.
func (c *SpawnController) SpawnBackground(ctx context.Context, id string, req agentcontracts.SpawnRequest) (string, error) {
	if c == nil || c.session == nil {
		return "", fmt.Errorf("spawn: no session")
	}
	req.Background = true
	if _, err := req.Normalize(); err != nil {
		return "", err
	}
	if c.session.Tools() != nil && c.session.Tools().AgentSpawnFn() == nil {
		c.session.WireAgentTool()
	}
	bm := c.session.Tools().EnsureBackgroundManager()
	if bm == nil {
		return "", fmt.Errorf("spawn: background manager unavailable")
	}
	if id == "" {
		id = fmt.Sprintf("bg-%d", time.Now().UnixNano())
	}
	fn := c.session.Tools().AgentSpawnFn()
	bm.Spawn(ctx, id, req, fn)
	return id, nil
}

// Wait waits for background tasks up to timeout and returns them.
func (c *SpawnController) Wait(timeout time.Duration) []*taskruntime.Task {
	reg := c.Tasks()
	if reg == nil {
		return nil
	}
	return reg.Wait(timeout)
}

// Status returns a compact snapshot for HUD / status commands.
func (c *SpawnController) Status() string {
	reg := c.Tasks()
	if reg == nil {
		return "tasks: none"
	}
	return fmt.Sprintf("tasks: pending=%d", reg.PendingCount())
}
