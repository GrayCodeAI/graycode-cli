package engine

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/hawk/internal/engine/safety"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/permissions"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
)

// PermissionService is the Session's view of the safety/approval layer.
// It owns the PermissionEngine, the legacy PermissionMemory / AutoMode /
// Classifier / BypassKill shims (which are now thin wrappers around
// Perm), the AutonomyLevel, the MaxTurns / MaxBudgetUSD caps, the
// ApprovalGate, and the AllowedDirs/permission function callbacks.
//
// Extracted from Session in Phase 2 of the god-object decomposition
// (see docs/session-decomposition.md). The legacy fields on Session
// (Perm, Permissions, AutoMode, Classifier, BypassKill, Mode, MaxTurns,
// MaxBudgetUSD, AllowedDirs, PermissionFn, Autonomy, Approval) stay on
// Session for backward compat with code that reads them directly; they
// are all thin forwarders to the new service. They will be removed in
// Phase 7.
type PermissionService struct {
	// perm is the underlying PermissionEngine. Always non-nil after
	// construction.
	perm *PermissionEngine
	// memory/autoMode/classifier/bypassKill are the legacy
	// PermissionEngine sub-fields, re-exported as top-level fields for
	// backward compat.
	memory     *PermissionMemory
	autoMode   *permissions.AutoModeState
	classifier *permissions.Classifier
	bypassKill *permissions.BypassKillswitch
	// maxTurns / maxBudgetUSD are the per-session cost/scope caps.
	maxTurns     int
	maxBudgetUSD float64
	// allowedDirs is the list of directories the agent may write to.
	allowedDirs []string
	// permissionFn is the user-callback that prompts for approval.
	permissionFn func(PermissionRequest)
	// approval is the human-in-the-loop gate for high-risk tool actions.
	approval *ApprovalGate
	// log is the session logger.
	log *logger.Logger
}

// NewPermissionService constructs a PermissionService with a fresh
// PermissionEngine. Tests can inject a custom engine via WithEngine.
func NewPermissionService(log *logger.Logger) *PermissionService {
	if log == nil {
		log = logger.Default()
	}
	pe := NewPermissionEngine()
	return &PermissionService{
		perm:       pe,
		memory:     pe.Memory,
		autoMode:   pe.AutoMode,
		classifier: pe.Classifier,
		bypassKill: pe.BypassKill,
		log:        log,
	}
}

// WithEngine replaces the underlying PermissionEngine. Used by tests
// and by callers that want a pre-configured engine.
func (s *PermissionService) WithEngine(pe *PermissionEngine) *PermissionService {
	s.perm = pe
	s.memory = pe.Memory
	s.autoMode = pe.AutoMode
	s.classifier = pe.Classifier
	s.bypassKill = pe.BypassKill
	return s
}

// Engine returns the underlying PermissionEngine. Used by the legacy
// Session fields that read s.Perm directly.
func (s *PermissionService) Engine() *PermissionEngine { return s.perm }

// CheckTool is the central permission check. Returns (granted, denyMsg).
// The caller (engine/stream_tool_exec.go) handles the tool_result
// event emission and the post-call side effects.
func (s *PermissionService) CheckTool(ctx context.Context, info ToolCallInfo) (bool, string) {
	d := s.CheckToolDecision(ctx, info)
	if d.Outcome != safety.DecisionAllow {
		s.log.Warn("permission denied", map[string]interface{}{
			"tool":   info.Name,
			"reason": string(d.Reason),
		})
	}
	return d.Outcome == safety.DecisionAllow, d.Message
}

// CheckToolDecision evaluates a request and exposes stable decision metadata.
func (s *PermissionService) CheckToolDecision(ctx context.Context, info ToolCallInfo) safety.Decision {
	return s.perm.CheckToolDecision(ctx, info)
}

// PolicySnapshot returns the scalar policy used for a single request.
func (s *PermissionService) PolicySnapshot() safety.PolicySnapshot {
	return s.perm.Snapshot()
}

// CheckToolSnapshot evaluates a request against a previously captured policy.
func (s *PermissionService) CheckToolSnapshot(ctx context.Context, info ToolCallInfo, snapshot safety.PolicySnapshot) safety.Decision {
	return s.perm.CheckToolSnapshot(ctx, info, snapshot)
}

// CheckApproval runs the human-in-the-loop gate on high-risk actions.
// Returns (approved, denyMsg). The caller handles tool_result emission.
// This is a thin wrapper around the engine's per-tool session.CheckApproval
// helper logic; the full implementation lives in
// internal/engine/safety/approval_gate.go (ApprovalGate) and is invoked
// via the Session.CheckApproval method (which has the full state). The
// service's own CheckApproval is a no-op when s.approval is nil so
// callers can use it as the canonical entry point.
func (s *PermissionService) CheckApproval(_ context.Context, toolName string, args map[string]interface{}) (bool, string) {
	if s.approval == nil || !s.approval.Enabled {
		return true, ""
	}
	// Delegate to the ApprovalGate classifier. The full session-aware
	// CheckApproval (which honors sessionApprovals cache) lives on Session
	// because it needs Session-scoped state. The classifier-only check
	// here is sufficient for the safety/dry-run code paths.
	cat, triggered := s.approval.classifyAction(toolName, args)
	if !triggered {
		return true, ""
	}
	if s.approval.MaxAutoApprove > 0 && s.perm.Autonomy <= s.approval.MaxAutoApprove {
		return true, ""
	}
	return false, fmt.Sprintf("approval required for category %q", cat)
}

// SetMaxTurns caps the agent loop's turn count.
func (s *PermissionService) SetMaxTurns(turns int) { s.maxTurns = turns }

// SetMaxBudgetUSD caps the agent loop's spend in USD.
func (s *PermissionService) SetMaxBudgetUSD(usd float64) { s.maxBudgetUSD = usd }

// SetAllowedDirs sets the directories the agent may write to.
func (s *PermissionService) SetAllowedDirs(dirs []string) { s.allowedDirs = dirs }

// SetAutonomy sets the agent's autonomy level. Writes directly to the
// underlying PermissionEngine — the same field CheckTool reads — rather
// than a separate shadow field, so the change actually takes effect.
func (s *PermissionService) SetAutonomy(level AutonomyLevel) {
	s.perm.Autonomy = level
	s.perm.AutonomyExplicit = true
	s.perm.Revision++
}

// SetSpecStage sets the independent spec-workflow stage. Also writes
// directly to the engine, same reasoning as SetAutonomy.
func (s *PermissionService) SetSpecStage(stage SpecStage) { s.perm.Stage = stage; s.perm.Revision++ }

// SetDryRun toggles the global kill switch: when true, every tool call is
// denied unconditionally, regardless of tier or spec stage.
func (s *PermissionService) SetDryRun(dryRun bool) { s.perm.DryRun = dryRun; s.perm.Revision++ }

// SetSandboxMode sets the OS/tool sandbox policy used for subsequent calls.
func (s *PermissionService) SetSandboxMode(mode sandbox.Mode) {
	s.perm.SandboxMode = mode
	s.perm.Revision++
}

// SandboxMode reports the active OS/tool sandbox policy.
func (s *PermissionService) SandboxMode() sandbox.Mode { return s.perm.SandboxMode }

// DryRun reports whether the kill switch is active.
func (s *PermissionService) DryRun() bool { return s.perm.DryRun }

// SetApproval replaces the ApprovalGate.
func (s *PermissionService) SetApproval(a *ApprovalGate) { s.approval = a }

// SetPermissionFn replaces the user-callback.
func (s *PermissionService) SetPermissionFn(fn func(PermissionRequest)) {
	s.permissionFn = fn
	s.perm.PromptFn = fn
}

// MaxTurns returns the cap (0 = no cap).
func (s *PermissionService) MaxTurns() int { return s.maxTurns }

// MaxBudgetUSD returns the cap.
func (s *PermissionService) MaxBudgetUSD() float64 { return s.maxBudgetUSD }

// AllowedDirs returns the write-allowlist.
func (s *PermissionService) AllowedDirs() []string { return s.allowedDirs }

// Autonomy returns the autonomy level.
func (s *PermissionService) Autonomy() AutonomyLevel { return s.perm.Autonomy }

// AutonomyExplicit reports whether the session selected or loaded a tier,
// including Supervised (which numerically shares the zero value).
func (s *PermissionService) AutonomyExplicit() bool { return s.perm.AutonomyExplicit }

// SpecStage returns the active spec-workflow stage.
func (s *PermissionService) SpecStage() SpecStage { return s.perm.Stage }

// Memory returns the legacy PermissionMemory shim. The shim is
// kept in sync with the engine's classification state; callers
// that historically used `sess.Permissions.AllowSpec(...)` should
// migrate to `sess.PermSvc().Memory().AllowSpec(...)`.
func (s *PermissionService) Memory() *PermissionMemory { return s.memory }

// AutoMode returns the legacy AutoModeState shim.
func (s *PermissionService) AutoMode() *permissions.AutoModeState { return s.autoMode }

// Classifier returns the legacy Classifier shim.
func (s *PermissionService) Classifier() *permissions.Classifier { return s.classifier }

// BypassKill returns the legacy BypassKillswitch shim.
func (s *PermissionService) BypassKill() *permissions.BypassKillswitch { return s.bypassKill }

// IsZero reports whether this service has been fully configured.
// A zero PermissionService has no approval gate and no custom permission
// fn — that's the "freshly constructed" state used by NewSessionWithClient.
func (s *PermissionService) IsZero() bool {
	return s == nil || (s.approval == nil && s.permissionFn == nil)
}
