package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	agentcontracts "github.com/GrayCodeAI/eagle/agent"

	engagent "github.com/GrayCodeAI/hawk/internal/engine/agent"
	"github.com/GrayCodeAI/hawk/internal/eventlog"
	"github.com/GrayCodeAI/hawk/internal/gitworktree"
	"github.com/GrayCodeAI/hawk/internal/hooks"
	"github.com/GrayCodeAI/hawk/internal/prompts"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// WireAgentTool sets up typed sub-agent spawning via SpawnController.
// Modes: explore (read-only research), plan (read-only planning), general-purpose (full tools).
// Faces and tools should prefer Session.SpawnController() for new call sites.
func (s *Session) WireAgentTool() {
	_ = s.ensureBackgroundManager()
	// SpawnController.Spawn uses spawnSubAgentRequest; the tool AgentSpawnFn
	// remains the thin adapter so AgentTool/MultiAgent keep working.
	s.Tools().SetAgentSpawnFn(func(ctx context.Context, req agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
		return s.spawnSubAgentRequest(ctx, req, 0)
	})
}

// ensureBackgroundManager lazily attaches a BackgroundAgentManager on ToolService.
func (s *Session) ensureBackgroundManager() *tool.BackgroundAgentManager {
	if s.Tools() == nil {
		return nil
	}
	return s.Tools().EnsureBackgroundManager()
}

// spawnSubAgentRequest is the typed entrypoint used by the Agent tool.
func (s *Session) spawnSubAgentRequest(ctx context.Context, req agentcontracts.SpawnRequest, depth int) (agentcontracts.SpawnResult, error) {
	norm, err := req.Normalize()
	if err != nil {
		return agentcontracts.SpawnResult{Status: agentcontracts.StatusFailed, Error: err.Error()}, err
	}
	mode := mapContractsType(norm.SubagentType)
	start := time.Now()

	// Emit subagent.descriptor and tool-workflow.agent-start to the journal.
	if j := s.Persistence().Journal(); j != nil {
		j.AppendSubagentDescriptorFull(eventlog.SubagentDescriptorFact{
			Name:          string(norm.SubagentType),
			Agent:         string(mode),
			Depth:         depth,
			Mode:          "one-shot",
			Provider:      "hawk",
			Label:         truncateSummary(norm.Prompt, 200),
			AgentProvider: s.ChatLLM().Provider(),
			AgentModel:    s.ChatLLM().Model(),
		})
		j.AppendToolWorkflowAgentStart(string(norm.SubagentType))
	}

	subStartErr := hooks.Execute(ctx, hooks.EventSubagentStart, map[string]interface{}{
		"subagent_type": string(norm.SubagentType),
		"prompt":        norm.Prompt,
		"isolation":     string(norm.Isolation),
		"depth":         depth,
	})
	if j := s.Persistence().Journal(); j != nil {
		hookErr := ""
		if subStartErr != nil {
			hookErr = subStartErr.Error()
		}
		j.AppendHookResult(string(hooks.EventSubagentStart), hookErr)
	}

	out, wtPath, err := s.spawnSubAgent(ctx, norm, mode, depth)
	res := agentcontracts.SpawnResult{
		SubagentType: string(norm.SubagentType),
		DurationMs:   time.Since(start).Milliseconds(),
		Output:       out,
		Summary:      truncateSummary(out, 200),
		WorktreePath: wtPath,
	}
	if err != nil {
		res.Status = agentcontracts.StatusFailed
		res.Error = err.Error()
		if j := s.Persistence().Journal(); j != nil {
			j.AppendToolWorkflowAgentEnd(string(norm.SubagentType))
			j.AppendHookInvoked(string(hooks.EventSubagentStop))
			j.AppendHookInvoked(string(hooks.EventFailure))
		}
		subStopErr := hooks.Execute(ctx, hooks.EventSubagentStop, map[string]interface{}{
			"subagent_type": string(norm.SubagentType),
			"status":        res.Status,
			"error":         res.Error,
		})
		if j := s.Persistence().Journal(); j != nil {
			hookErr := ""
			if subStopErr != nil {
				hookErr = subStopErr.Error()
			}
			j.AppendHookResult(string(hooks.EventSubagentStop), hookErr)
		}
		failErr := hooks.Execute(ctx, hooks.EventFailure, map[string]interface{}{
			"source": "subagent",
			"error":  res.Error,
		})
		if j := s.Persistence().Journal(); j != nil {
			hookErr := ""
			if failErr != nil {
				hookErr = failErr.Error()
			}
			j.AppendHookResult(string(hooks.EventFailure), hookErr)
		}
		return res, err
	}
	res.Status = agentcontracts.StatusCompleted
	if j := s.Persistence().Journal(); j != nil {
		j.AppendToolWorkflowAgentEnd(string(norm.SubagentType))
		j.AppendHookInvoked(string(hooks.EventSubagentStop))
	}
	subStopErr := hooks.Execute(ctx, hooks.EventSubagentStop, map[string]interface{}{
		"subagent_type": string(norm.SubagentType),
		"status":        res.Status,
		"duration_ms":   res.DurationMs,
	})
	if j := s.Persistence().Journal(); j != nil {
		hookErr := ""
		if subStopErr != nil {
			hookErr = subStopErr.Error()
		}
		j.AppendHookResult(string(hooks.EventSubagentStop), hookErr)
	}
	return res, nil
}

func mapContractsType(t agentcontracts.SubagentType) SubAgentMode {
	switch t {
	case agentcontracts.TypePlan:
		return SubAgentPlan
	case agentcontracts.TypeGeneralPurpose:
		return SubAgentGeneral
	default:
		return SubAgentExplore
	}
}

// spawnSubAgent creates a sub-agent with the given mode and depth tracking.
// Returns (output, worktreePath, error).
func (s *Session) spawnSubAgent(ctx context.Context, norm agentcontracts.Normalized, mode SubAgentMode, depth int) (string, string, error) {
	if depth >= MaxAgentDepth {
		return "", "", fmt.Errorf("max agent depth %d exceeded", MaxAgentDepth)
	}

	maxTurns := DefaultTurnsForMode(mode)
	if mode == SubAgentExplore && norm.Thoroughness != "" {
		maxTurns = engagent.ThoroughnessTurns(engagent.ExploreThoroughness(norm.Thoroughness))
	}

	model := s.resolveSubAgentModel(mode)
	if norm.Model != "" {
		model = norm.Model
	}
	registry := s.resolveSubAgentTools(mode)

	// Emit agent.preset.selected when the model is resolved based on
	// the sub-agent mode preset (DSH agent.preset.selected seam).
	if j := s.Persistence().Journal(); j != nil && norm.Model == "" {
		j.AppendAgentPresetSelected(string(mode))
	}

	// Capability mode can further restrict tools beyond profile defaults.
	if norm.CapabilityMode == agentcontracts.CapReadOnly {
		registry = registry.Filter(ExploreTools)
	}

	subPromptCtx := prompts.PromptContext{
		MaxTurns: maxTurns,
		Task:     norm.Prompt,
	}
	subSystemPrompt, err := prompts.BuildSubAgentPrompt(subPromptCtx)
	if err != nil {
		subSystemPrompt = s.Persistence().System()
	}
	if mode == SubAgentPlan {
		subSystemPrompt = planSystemPrefix + "\n\n" + subSystemPrompt
	}

	sub := s.SubSession(model, subSystemPrompt, registry)
	sub.PermSvc().SetPermissionFn(s.PermSvc().PermissionFn())
	// Explore/plan: hard read-only bash allowlist (in addition to tool filter).
	if IsReadOnlyMode(mode) || norm.CapabilityMode == agentcontracts.CapReadOnly {
		sub.Tools().SetReadOnlyBash(true)
	}
	// A child receives an independent snapshot of the parent's policy. This
	// prevents parent mutations from changing an in-flight child and prevents
	// child defaults from silently widening the parent's permissions.
	sub.PermSvc().ApplyPolicySnapshot(s.PermSvc().PolicySnapshot())
	// DSH 2.4: Inherit delegated sandbox policy from parent at delegation boundary.
	sandbox.InheritDelegatedPolicy(s, sub)
	// DSH 2.4: Pin child approval policy to never (deny interactive asks deterministically).
	sub.PermSvc().SetAskUserFn(func(question string) (string, error) {
		return "", fmt.Errorf("subagent cannot prompt user for interactive input")
	})
	sub.SetPermissionFn(func(req PermissionRequest) {
		if req.Response != nil {
			req.Response <- false
		}
	})
	// DSH 2.4: Append model-facing delegation statement.
	sub.AppendSystemContext(sandbox.FormatDelegationStatement())
	if s.LifecycleSvc() != nil {
		s.LifecycleSvc().Limits().SetMaxTurns(maxTurns)
	}

	var (
		wtPath  string
		cleanup func()
	)
	workDir := norm.CWD
	if norm.Isolation == agentcontracts.IsoWorktree {
		repoDir, wdErr := os.Getwd()
		if wdErr != nil {
			return "", "", fmt.Errorf("worktree isolation: resolve cwd: %w", wdErr)
		}
		path, cu, createErr := gitworktree.Create(ctx, repoDir, "")
		if createErr != nil {
			return "", "", fmt.Errorf("worktree isolation: %w", createErr)
		}
		wtPath = path
		cleanup = cu
		workDir = path
	}
	if cleanup != nil {
		defer cleanup()
	}
	if workDir != "" {
		sub.Tools().SetWorkingDir(workDir)
		sub.SetAllowedDirs([]string{workDir})
	}

	prompt := norm.Prompt
	if workDir != "" {
		prompt = fmt.Sprintf("Working directory: %s\n\n%s", workDir, prompt)
	}
	if norm.ResumeFrom != "" {
		if priorSession, loadErr := session.Load(norm.ResumeFrom); loadErr == nil && priorSession != nil {
			for _, m := range priorSession.Messages {
				sub.Persistence().AddMessage(m.Role, m.Content)
			}
		} else {
			prompt = fmt.Sprintf("Resume prior subagent %s.\n\n%s", norm.ResumeFrom, prompt)
		}
	}
	sub.AddUser(prompt)

	// Propagate parent agent spawn so nested agents work, and share bg manager.
	sub.Tools().SetAgentSpawnFn(func(ctx context.Context, req agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
		return s.spawnSubAgentRequest(ctx, req, depth+1)
	})
	if bm := s.ensureBackgroundManager(); bm != nil && sub.Tools() != nil {
		sub.Tools().WithBackgroundManager(bm)
	}

	ch, err := sub.Stream(ctx)
	if err != nil {
		return "", wtPath, err
	}

	var b strings.Builder
	for ev := range ch {
		switch ev.Type {
		case "content":
			b.WriteString(ev.Content)
		case "error":
			return b.String(), wtPath, nil
		}
	}
	return b.String(), wtPath, nil
}

const planSystemPrefix = "You are a planning sub-agent. Produce an ordered, actionable plan. " +
	"Do not modify files. Prefer research tools (Read, Grep, Glob, LS) and only use Bash for read-only inspection."

func (s *Session) resolveSubAgentModel(mode SubAgentMode) string {
	current := s.ChatLLM().Model()
	if s.LifecycleSvc().Cascade() == nil {
		return current
	}
	switch mode {
	case SubAgentExplore:
		return s.LifecycleSvc().Cascade().SelectModel("summarize", current, s.LifecycleSvc().Cascade().Roles.Explorer)
	case SubAgentPlan:
		return s.LifecycleSvc().Cascade().SelectModel("planning", current, s.LifecycleSvc().Cascade().Roles.Planner)
	case SubAgentGeneral:
		return s.LifecycleSvc().Cascade().SelectModel("implement", current, "")
	default:
		return current
	}
}

func (s *Session) resolveSubAgentTools(mode SubAgentMode) *tool.Registry {
	registry := s.Tools().Registry()
	switch mode {
	case SubAgentExplore:
		return registry.Filter(ExploreTools)
	case SubAgentPlan:
		return registry.Filter(PlanTools)
	default:
		return registry
	}
}

func truncateSummary(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
