package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/engine/safety"
	"github.com/GrayCodeAI/hawk/internal/governance"
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
	mu sync.RWMutex
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
	// askUserFn is the fallback interactive approval callback.
	askUserFn func(question string) (string, error)
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
	s := &PermissionService{
		perm:       pe,
		memory:     pe.Memory,
		autoMode:   pe.AutoMode,
		classifier: pe.Classifier,
		bypassKill: pe.BypassKill,
		log:        log,
	}
	s.loadManagedGovernance()
	return s
}

// loadManagedGovernance attempts to install the administrator-set POLICY
// ceiling from the platform trust-root (see governance.ManagedPolicyPath).
// A missing file is the normal unmanaged case and leaves the engine
// fail-open; a malformed file is logged loudly so misconfiguration is
// visible without hard-failing every session.
func (s *PermissionService) loadManagedGovernance() {
	if s == nil || s.perm == nil || s.perm.Governance == nil {
		return
	}
	path := governance.ManagedPolicyPath()
	if path == "" {
		return
	}
	if err := s.perm.Governance.LoadPolicy(path); err != nil {
		if !os.IsNotExist(err) {
			s.log.Error("governance: failed to load managed policy", map[string]interface{}{
				"path":  path,
				"error": err.Error(),
			})
		}
	}
}

// WithEngine replaces the underlying PermissionEngine. Used by tests
// and by callers that want a pre-configured engine.
func (s *PermissionService) WithEngine(pe *PermissionEngine) *PermissionService {
	if s == nil {
		return s
	}
	if pe == nil {
		pe = NewPermissionEngine()
	}
	s.perm = pe
	s.memory = pe.Memory
	s.autoMode = pe.AutoMode
	s.classifier = pe.Classifier
	s.bypassKill = pe.BypassKill
	return s
}

// Logger returns the logger used by permission decisions.
func (s *PermissionService) Logger() *logger.Logger {
	if s == nil {
		return nil
	}
	return s.log
}

// SetLogger replaces the logger used by permission decisions.
func (s *PermissionService) SetLogger(l *logger.Logger) {
	if s == nil {
		return
	}
	if l == nil {
		l = logger.Default()
	}
	s.log = l
}

// Engine returns the underlying PermissionEngine. Used by the legacy
// Session fields that read s.Perm directly.
func (s *PermissionService) Engine() *PermissionEngine { return s.perm }

// CheckTool is the central permission check. Returns (granted, denyMsg).
// The caller (engine/stream_tool_exec.go) handles the tool_result
// event emission and the post-call side effects.
func (s *PermissionService) CheckTool(ctx context.Context, info ToolCallInfo) (bool, string) {
	if s == nil || s.perm == nil {
		return false, "permission service is unavailable"
	}
	granted, denyMsg := s.perm.CheckTool(ctx, info)
	if !granted {
		s.log.Warn("permission denied", map[string]interface{}{
			"tool":   info.Name,
			"reason": denyMsg,
		})
	}
	return granted, denyMsg
}

// CheckToolDecision evaluates a request and exposes stable decision metadata.
func (s *PermissionService) CheckToolDecision(ctx context.Context, info ToolCallInfo) safety.Decision {
	perm := s.engineCopy()
	return perm.CheckToolDecision(ctx, info)
}

// EvaluateTool returns allow, ask, or deny without blocking on the UI.
func (s *PermissionService) EvaluateTool(ctx context.Context, info ToolCallInfo) safety.Decision {
	perm := s.engineCopy()
	return perm.EvaluateTool(ctx, info)
}

// PolicySnapshot returns the scalar policy used for a single request.
func (s *PermissionService) PolicySnapshot() safety.PolicySnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := s.perm.Snapshot()
	snapshot.AllowedDirs = append([]string(nil), s.allowedDirs...)
	return snapshot
}

// CheckToolSnapshot evaluates a request against a previously captured policy.
func (s *PermissionService) CheckToolSnapshot(ctx context.Context, info ToolCallInfo, snapshot safety.PolicySnapshot) safety.Decision {
	perm := s.engineCopy()
	return perm.CheckToolSnapshot(ctx, info, snapshot)
}

// engineCopy returns a copy of the engine for cross-goroutine evaluation. The
// service lock is held only for the duration of the copy, so evaluation does
// not block policy updates or user prompts.
func (s *PermissionService) engineCopy() *PermissionEngine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.perm.Copy()
}

// ApplyPolicySnapshot installs a bounded parent policy into a child service.
// The rule store is deep-copied so later parent changes cannot widen or alter
// an in-flight child policy.
func (s *PermissionService) ApplyPolicySnapshot(snapshot safety.PolicySnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.perm.Autonomy = snapshot.Autonomy
	s.perm.AutonomyExplicit = snapshot.AutonomyExplicit
	s.perm.SandboxMode = snapshot.SandboxMode
	s.perm.Stage = snapshot.Stage
	s.perm.DryRun = snapshot.DryRun
	s.perm.SpecSlug = snapshot.SpecSlug
	s.perm.Phase = snapshot.Phase
	s.perm.Phases = snapshot.Phases
	s.perm.Revision = snapshot.Revision
	s.perm.Memory = safety.NewPermissionMemoryFromSnapshot(snapshot.Rules)
	s.memory = s.perm.Memory
	// Rebuild UnifiedGrants so it wraps the snapshot's Memory (not the
	// service's original Memory, which is now stale).
	if s.perm.UnifiedGrants != nil {
		s.perm.UnifiedGrants = permissions.NewUnifiedGrants(s.perm.Memory, s.perm.AutoMode)
	}
	s.allowedDirs = append([]string(nil), snapshot.AllowedDirs...)
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
	g := s.approval
	if g == nil || !g.Enabled {
		return true, ""
	}
	cat, risky := g.classifyAction(toolName, args)
	if !risky || !g.categoryEnabled(cat) {
		return true, ""
	}
	if s.perm.Autonomy <= g.MaxAutoApprove {
		return true, ""
	}
	// Atomic check-and-consume: session-wide approval or remaining N-count.
	// tryConsumeApproval holds the lock across both checks so concurrent
	// high-risk tool calls cannot double-spend a session or N-count approval.
	if g.tryConsumeApproval(cat) {
		return true, ""
	}
	req := ApprovalRequest{
		ToolName: canonicalToolName(toolName),
		Category: cat,
		Summary:  approvalSummary(toolName, args),
		Args:     args,
	}
	if g.ConfirmFn != nil {
		switch g.ConfirmFn(req) {
		case ApprovalApproveForSession:
			g.sessionApprove(cat)
			return true, ""
		case ApprovalApproveForN:
			// Default N=5 when the typed response carries no count.
			n := req.N
			if n <= 0 {
				n = 5
			}
			g.nApprove(cat, n)
			return true, ""
		case ApprovalApprove:
			return true, ""
		default:
			return false, "Action denied by human approval gate (" + string(cat) + ")."
		}
	}
	if s.askUserFn != nil {
		ans, err := s.askUserFn("Approve high-risk action [" + string(cat) + "]: " + req.Summary + "? (yes/no/session/N)")
		if err != nil {
			return false, "Action denied by human approval gate (" + string(cat) + ")."
		}
		lower := strings.ToLower(strings.TrimSpace(ans))
		switch lower {
		case "session", "s", "approve-session", "yes-session":
			g.sessionApprove(cat)
			return true, ""
		default:
			// "10" or "5x" style: approve for N.
			if n, ok := parseApprovalCount(lower); ok {
				g.nApprove(cat, n)
				return true, ""
			}
			if isAffirmative(ans) {
				return true, ""
			}
			return false, "Action denied by human approval gate (" + string(cat) + ")."
		}
	}
	return false, fmt.Sprintf("High-risk action requires approval but no confirmation handler is configured (%q).", cat)
}

// SetMaxTurns caps the agent loop's turn count.
func (s *PermissionService) SetMaxTurns(turns int) {
	if s != nil {
		s.maxTurns = turns
	}
}

// SetMaxBudgetUSD caps the agent loop's spend in USD.
func (s *PermissionService) SetMaxBudgetUSD(usd float64) {
	if s != nil {
		s.maxBudgetUSD = usd
	}
}

// SetAllowedDirs sets the directories the agent may write to.
func (s *PermissionService) SetAllowedDirs(dirs []string) {
	if s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.allowedDirs = append([]string(nil), dirs...)
	}
}

// SetAutonomy sets the agent's autonomy level and rebuilds the per-flag
// profile from that level, preserving any user overrides. Writes directly to
// the underlying PermissionEngine — the same field CheckTool reads — rather
// than a separate shadow field, so the change actually takes effect.
func (s *PermissionService) SetAutonomy(level AutonomyLevel) {
	if s == nil || s.perm == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.perm.Autonomy = level
	s.perm.AutonomyExplicit = true
	s.perm.Revision++
	// Rebuild profile from the new level, then re-apply overrides so the
	// user's per-flag tweaks survive a tier change.
	if s.perm.Profile != nil {
		overrides := s.perm.Profile.Overrides()
		s.perm.Profile = safety.ProfileFromLevel(level)
		s.perm.Profile.ApplyOverrides(overrides)
	}
}

// ApplyAutonomyOverrides merges per-flag overrides onto the active profile.
// Unknown flag names are ignored. The profile is rebuilt from the current
// level first so overrides are applied consistently.
func (s *PermissionService) ApplyAutonomyOverrides(overrides map[string]bool) {
	if s == nil || s.perm == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.perm.Profile == nil {
		s.perm.Profile = safety.ProfileFromLevel(s.perm.Autonomy)
	}
	s.perm.Profile.ApplyOverrides(overrides)
	s.perm.Revision++
}

// AutonomyProfile returns a copy of the active profile's override set (for
// display/persistence). Returns nil if no profile is active.
func (s *PermissionService) AutonomyProfile() *safety.AutonomyProfile {
	if s == nil || s.perm == nil || s.perm.Profile == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.perm.Profile
}

// SetSpecStage sets the independent spec-workflow stage. Also writes
// directly to the engine, same reasoning as SetAutonomy.
func (s *PermissionService) SetSpecStage(stage SpecStage) {
	if s != nil && s.perm != nil {
		s.perm.Stage = stage
	}
}

// SetDryRun toggles the global kill switch: when true, every tool call is
// denied unconditionally, regardless of tier or spec stage.
func (s *PermissionService) SetDryRun(dryRun bool) {
	if s != nil && s.perm != nil {
		s.perm.DryRun = dryRun
	}
}

// SetSandboxMode updates the sandbox policy used for subsequent tool calls.
func (s *PermissionService) SetSandboxMode(mode sandbox.Mode) {
	if s == nil || s.perm == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.perm.SandboxMode = mode
	s.perm.Revision++
}

// SandboxMode returns the active sandbox policy.
func (s *PermissionService) SandboxMode() sandbox.Mode {
	if s == nil || s.perm == nil {
		return sandbox.Mode("")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.perm.SandboxMode
}

// DryRun reports whether the kill switch is active.
func (s *PermissionService) DryRun() bool { return s != nil && s.perm != nil && s.perm.DryRun }

// SetApproval replaces the ApprovalGate.
func (s *PermissionService) SetApproval(a *ApprovalGate) {
	if s != nil {
		s.approval = a
	}
}

// SetAskUserFn sets the fallback interactive approval callback.
func (s *PermissionService) SetAskUserFn(fn func(question string) (string, error)) {
	if s != nil {
		s.askUserFn = fn
	}
}

func (s *PermissionService) AskUserFn() func(string) (string, error) {
	if s == nil {
		return nil
	}
	return s.askUserFn
}

// Approval returns the configured human-in-the-loop gate.
func (s *PermissionService) Approval() *ApprovalGate {
	if s == nil {
		return nil
	}
	return s.approval
}

// SpecSlug returns the active specification identifier.
func (s *PermissionService) SpecSlug() string {
	if s == nil || s.perm == nil {
		return ""
	}
	return s.perm.SpecSlug
}

// SetSpecSlug updates the active specification identifier.
func (s *PermissionService) SetSpecSlug(slug string) {
	if s != nil && s.perm != nil {
		s.perm.SpecSlug = slug
	}
}

// SetPermissionFn replaces the user-callback.
func (s *PermissionService) SetPermissionFn(fn func(PermissionRequest)) {
	if s == nil || s.perm == nil {
		return
	}
	s.permissionFn = fn
	s.perm.PromptFn = fn
}

// PermissionFn returns the configured approval callback for sub-agent
// construction and legacy integrations.
func (s *PermissionService) PermissionFn() func(PermissionRequest) {
	if s == nil {
		return nil
	}
	return s.permissionFn
}

// MaxTurns returns the cap (0 = no cap).
func (s *PermissionService) MaxTurns() int {
	if s == nil {
		return 0
	}
	return s.maxTurns
}

// MaxBudgetUSD returns the cap.
func (s *PermissionService) MaxBudgetUSD() float64 {
	if s == nil {
		return 0
	}
	return s.maxBudgetUSD
}

// AllowedDirs returns the write-allowlist.
func (s *PermissionService) AllowedDirs() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.allowedDirs...)
}

// Autonomy returns the autonomy level.
func (s *PermissionService) Autonomy() AutonomyLevel {
	if s == nil || s.perm == nil {
		return 0
	}
	return s.perm.Autonomy
}

// SpecStage returns the active spec-workflow stage.
func (s *PermissionService) SpecStage() SpecStage {
	if s == nil || s.perm == nil {
		return SpecStageNone
	}
	return s.perm.Stage
}

// SpecPhaseProgress returns the current and total implementation phases.
func (s *PermissionService) SpecPhaseProgress() (current, total int) {
	if s == nil || s.perm == nil {
		return 0, 0
	}
	return s.perm.Phase, s.perm.Phases
}

// AdvanceSpecStage records the next spec workflow transition through the
// permission service instead of exposing the underlying engine to callers.
func (s *PermissionService) AdvanceSpecStage(toolName string) {
	if s == nil || s.perm == nil {
		return
	}
	s.perm.AdvanceSpecStage(toolName)
}

// ResetSpec clears the active spec workflow.
func (s *PermissionService) ResetSpec() {
	if s == nil || s.perm == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w := safety.SpecWorkflow{Stage: s.perm.Stage, Slug: s.perm.SpecSlug}
	w.Reset()
	s.perm.Stage, s.perm.SpecSlug = w.Stage, w.Slug
	s.perm.Revision++
}

// AutonomyExplicit reports whether the autonomy tier was explicitly chosen.
func (s *PermissionService) AutonomyExplicit() bool {
	if s == nil || s.perm == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.perm.AutonomyExplicit
}

// SpecProgress returns the workflow stage and phase counters atomically.
func (s *PermissionService) SpecProgress() (SpecStage, int, int) {
	if s == nil || s.perm == nil {
		return SpecStageNone, 0, 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.perm.Stage, s.perm.Phase, s.perm.Phases
}

// Memory returns the legacy PermissionMemory shim. The shim is
// kept in sync with the engine's classification state; callers
// that historically used `sess.Permissions.AllowSpec(...)` should
// migrate to `sess.PermSvc().Memory().AllowSpec(...)`.
func (s *PermissionService) Memory() *PermissionMemory {
	if s == nil {
		return nil
	}
	return s.memory
}

// SetMemory replaces the session's permission-memory policy store.
func (s *PermissionService) SetMemory(m *PermissionMemory) {
	if s == nil || s.perm == nil {
		return
	}
	s.memory = m
	s.perm.Memory = m
}

// AutoMode returns the legacy AutoModeState shim.
func (s *PermissionService) AutoMode() *permissions.AutoModeState {
	if s == nil {
		return nil
	}
	return s.autoMode
}

// Classifier returns the legacy Classifier shim.
func (s *PermissionService) Classifier() *permissions.Classifier {
	if s == nil {
		return nil
	}
	return s.classifier
}

// BypassKill returns the legacy BypassKillswitch shim.
func (s *PermissionService) BypassKill() *permissions.BypassKillswitch {
	if s == nil {
		return nil
	}
	return s.bypassKill
}

// SetNeverAllow replaces the personal hard-ceiling rule set on the engine.
func (s *PermissionService) SetNeverAllow(specs []string) {
	if s == nil || s.perm == nil {
		return
	}
	s.perm.SetNeverAllow(specs)
}

// NeverAllow returns a copy of the current never-allow rules.
func (s *PermissionService) NeverAllow() []string {
	if s == nil || s.perm == nil {
		return nil
	}
	return s.perm.NeverAllow()
}

// SetSpecAllowTests enables or disables safe test commands during spec stage.
func (s *PermissionService) SetSpecAllowTests(allow bool) {
	if s == nil || s.perm == nil {
		return
	}
	s.perm.SetSpecAllowTests(allow)
}

// AuditLog returns a formatted audit trail of recent permission decisions,
// or a message if the audit log is disabled.
func (s *PermissionService) AuditLog() string {
	if s == nil || s.perm == nil {
		return "Audit log unavailable."
	}
	if s.perm.AuditLog() == nil {
		return "Audit log disabled."
	}
	return s.perm.AuditLog().Format(50)
}

// PermissionMetrics returns a formatted metrics summary, or a message if
// metrics are disabled.
func (s *PermissionService) PermissionMetrics() string {
	if s == nil || s.perm == nil {
		return "Metrics unavailable."
	}
	if s.perm.PermissionMetrics() == nil {
		return "Metrics disabled."
	}
	return s.perm.PermissionMetrics().Format()
}

// IsZero reports whether this service has been fully configured.
// A zero PermissionService has no approval gate and no custom permission
// fn — that's the "freshly constructed" state used by NewSessionWithClient.
func (s *PermissionService) IsZero() bool {
	if s == nil {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.approval == nil && s.permissionFn == nil
}
