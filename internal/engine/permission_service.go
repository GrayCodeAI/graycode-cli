package engine

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/permissions"
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
	// mode is the active permission mode (e.g. plan, normal, auto).
	mode PermissionMode
	// maxTurns / maxBudgetUSD are the per-session cost/scope caps.
	maxTurns     int
	maxBudgetUSD float64
	// allowedDirs is the list of directories the agent may write to.
	allowedDirs []string
	// permissionFn is the user-callback that prompts for approval.
	permissionFn func(PermissionRequest)
	// approval is the human-in-the-loop gate for high-risk tool actions.
	approval *ApprovalGate
	// autonomy is the agent's autonomy level (0-3).
	autonomy AutonomyLevel
	// log is the session logger.
	log *logger.Logger
}

// NewPermissionService constructs a PermissionService with a fresh
// PermissionEngine. Tests can inject a custom engine via WithEngine.
//
// Note: mode is intentionally left at the zero value (PermissionMode(""))
// so that IsZero() correctly reports true for a freshly constructed
// service. Callers that want the default mode should call SetMode
// (or set the field directly during tests). See M4 in the code review.
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
	granted, denyMsg := s.perm.CheckTool(ctx, info)
	if !granted {
		s.log.Warn("permission denied", map[string]interface{}{
			"tool": info.Name,
		})
	}
	return granted, denyMsg
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

// SetMode validates the mode string and applies it. Returns an error
// for unknown modes.
func (s *PermissionService) SetMode(mode string) error {
	switch mode {
	case "default", "plan", "accept-edits", "auto", "bypass-permissions":
		s.mode = PermissionMode(mode)
		return nil
	}
	return fmt.Errorf("permissions: unknown mode %q", mode)
}

// SetMaxTurns caps the agent loop's turn count.
func (s *PermissionService) SetMaxTurns(turns int) { s.maxTurns = turns }

// SetMaxBudgetUSD caps the agent loop's spend in USD.
func (s *PermissionService) SetMaxBudgetUSD(usd float64) { s.maxBudgetUSD = usd }

// SetAllowedDirs sets the directories the agent may write to.
func (s *PermissionService) SetAllowedDirs(dirs []string) { s.allowedDirs = dirs }

// SetAutonomy sets the agent's autonomy level.
func (s *PermissionService) SetAutonomy(level AutonomyLevel) { s.autonomy = level }

// SetApproval replaces the ApprovalGate.
func (s *PermissionService) SetApproval(a *ApprovalGate) { s.approval = a }

// SetPermissionFn replaces the user-callback.
func (s *PermissionService) SetPermissionFn(fn func(PermissionRequest)) {
	s.permissionFn = fn
	s.perm.PromptFn = fn
}

// Mode returns the active mode.
func (s *PermissionService) Mode() PermissionMode { return s.mode }

// MaxTurns returns the cap (0 = no cap).
func (s *PermissionService) MaxTurns() int { return s.maxTurns }

// MaxBudgetUSD returns the cap.
func (s *PermissionService) MaxBudgetUSD() float64 { return s.maxBudgetUSD }

// AllowedDirs returns the write-allowlist.
func (s *PermissionService) AllowedDirs() []string { return s.allowedDirs }

// Autonomy returns the autonomy level.
func (s *PermissionService) Autonomy() AutonomyLevel { return s.autonomy }

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
// A zero PermissionService has no approval gate, no custom permission
// fn, and an empty mode — that's the "freshly constructed" state
// used by NewSessionWithClient (the constructor no longer pre-sets
// mode = PermissionModeDefault, see M4 in the code review).
func (s *PermissionService) IsZero() bool {
	return s == nil || (s.approval == nil && s.permissionFn == nil && s.mode == "")
}
