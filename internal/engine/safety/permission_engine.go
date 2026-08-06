package safety

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	contracts "github.com/GrayCodeAI/hawk-core-contracts/policy"
	"github.com/GrayCodeAI/hawk/internal/governance"
	"github.com/GrayCodeAI/hawk/internal/hooks"
	"github.com/GrayCodeAI/hawk/internal/permissions"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

var reNeedsClarify = regexp.MustCompile(`\[NEEDS CLARIFICATION.*?\]`)

// SpecStage tracks position in the independent spec-driven-development
// workflow. It is orthogonal to AutonomyLevel: a session can be at any
// trust tier while at any spec stage — trust governs *how* a tool call is
// approved, spec stage governs *which* tools are relevant to call right now.
type SpecStage int

const (
	SpecStageNone SpecStage = iota // no active spec workflow
	SpecStageProposal
	SpecStageSpecify
	SpecStageDesign
	SpecStagePlan
	SpecStageTasks
	SpecStageImplementing
)

// PermissionEngine encapsulates all permission-checking logic.
// Extracted from Session to keep the god object lean.
type PermissionEngine struct {
	Memory           *PermissionMemory
	AutoMode         *permissions.AutoModeState
	Classifier       *permissions.Classifier
	BypassKill       *permissions.BypassKillswitch
	Autonomy         AutonomyLevel
	AutonomyExplicit bool
	// Revision increments when policy configuration is replaced. It is
	// attached to decisions so audit consumers can correlate evaluations.
	Revision uint64
	// SandboxMode controls filesystem/process policy for tool execution. It is
	// deliberately separate from Autonomy: autonomy decides whether a user
	// prompt is needed, while the sandbox decides what the tool may actually do.
	SandboxMode sandbox.Mode
	Stage       SpecStage
	specDone    specDone
	// DryRun is a global kill switch: when true, every tool call is denied
	// unconditionally, regardless of tier or spec stage. Replaces the old
	// PermissionModeDontAsk's hard-lockout role — that mode was otherwise
	// redundant with tier-based asking, but this orthogonal, always-deny
	// behavior (mainly for CI/headless "preview only, never execute" runs)
	// had no equivalent once Mode was removed.
	DryRun bool
	// SpecSlug is the directory name (under .hawk/specs/) for the active
	// spec workflow, set by the Specify tool. Lives here (session-scoped,
	// via PermissionEngine) rather than as a package-level variable in
	// internal/tool, so concurrent sessions/sub-agents in the same process
	// never share or clobber each other's spec directory.
	SpecSlug string

	// Phase gates sequential task completion within the Implementing stage.
	// 0 means no phase gating (default); 1+ means the model should complete
	// Phase N before progressing to N+1.
	Phase           int
	Phases          int                     // total number of phases detected from tasks.md
	convergeChecked bool                    // whether convergence has been checked this session
	PromptFn        func(PermissionRequest) // callback to ask user
	// Governance is the POLICY ∩ PROFILE ceiling. It is evaluated before
	// every other gate (hooks, spec stage, rules, autonomy, bypass) so no
	// agent state or user-granted bypass can loosen an administrator-set
	// ceiling. Nil means fail-open (no governance policy installed).
	Governance *governance.Engine
}

// DecisionOutcome is the result of evaluating a tool request.
type DecisionOutcome string

const (
	DecisionAllow DecisionOutcome = "allow"
	DecisionAsk   DecisionOutcome = "ask"
	DecisionDeny  DecisionOutcome = "deny"
)

// DecisionReason is stable metadata for callers, telemetry, and tests. The
// human-readable message remains available through Decision.Message.
type DecisionReason string

const (
	ReasonNone              DecisionReason = ""
	ReasonDryRun            DecisionReason = "dry_run"
	ReasonHookDenied        DecisionReason = "hook_denied"
	ReasonSandbox           DecisionReason = "sandbox"
	ReasonSpecGate          DecisionReason = "spec_gate"
	ReasonRuleDenied        DecisionReason = "rule_denied"
	ReasonAutoModeDenied    DecisionReason = "auto_mode_denied"
	ReasonRuleAllowed       DecisionReason = "rule_allowed"
	ReasonAutoModeAllowed   DecisionReason = "auto_mode_allowed"
	ReasonAutonomy          DecisionReason = "autonomy"
	ReasonBypass            DecisionReason = "bypass"
	ReasonClassifiedSafe    DecisionReason = "classified_safe"
	ReasonUserPrompt        DecisionReason = "user_prompt"
	ReasonPromptUnavailable DecisionReason = "prompt_unavailable"
	ReasonGovernance        DecisionReason = "governance"
)

// Decision is the structured result of a permission evaluation.
type Decision struct {
	Outcome      DecisionOutcome
	Reason       DecisionReason
	Message      string
	MatchedRule  string
	Capabilities []Capability
	Risk         RiskLevel
	Revision     uint64
}

// PolicySnapshot captures the scalar policy state for one tool evaluation.
// Callers can use it to ensure a request is evaluated consistently even when
// the live session settings change while the tool is running.
type PolicySnapshot struct {
	Autonomy         AutonomyLevel
	AutonomyExplicit bool
	SandboxMode      sandbox.Mode
	Stage            SpecStage
	DryRun           bool
	SpecSlug         string
	Phase            int
	Phases           int
	Revision         uint64
	Rules            RuleSnapshot
	AllowedDirs      []string
}

// Snapshot returns a copy of the engine's request-relevant scalar policy.
func (pe *PermissionEngine) Snapshot() PolicySnapshot {
	var rules RuleSnapshot
	if pe.Memory != nil {
		rules = pe.Memory.Snapshot()
	}
	return PolicySnapshot{
		Autonomy: pe.Autonomy, AutonomyExplicit: pe.AutonomyExplicit,
		SandboxMode: pe.SandboxMode, Stage: pe.Stage, DryRun: pe.DryRun,
		SpecSlug: pe.SpecSlug, Phase: pe.Phase, Phases: pe.Phases, Revision: pe.Revision,
		Rules: rules,
	}
}

// NewPermissionEngine creates a PermissionEngine with sensible defaults.
func NewPermissionEngine() *PermissionEngine {
	return &PermissionEngine{
		Memory:     NewPermissionMemory(),
		AutoMode:   permissions.NewAutoModeState(),
		Classifier: permissions.NewClassifier(),
		BypassKill: permissions.NewBypassKillswitch(),
		Governance: governance.New(),
	}
}

// CheckTool determines if a tool call is allowed, denied, or needs user prompt.
// Returns (granted bool, denyReason string).
// If the user must be asked, it blocks on PromptFn with a 5-minute timeout.
//
// Order (Year 0 PACK-04):
//  1. DryRun
//  2. PreToolUse decision hooks (can deny before autonomy short-circuits)
//  3. Spec-stage gate
//  4. Autonomy / bypass / classifier / auto-mode / memory / user prompt
func (pe *PermissionEngine) CheckTool(ctx context.Context, tc ToolCallInfo) (bool, string) {
	d := pe.CheckToolDecision(ctx, tc)
	return d.Outcome == DecisionAllow, d.Message
}

// CheckToolSnapshot evaluates a request using the supplied immutable scalar
// policy snapshot. Mutable rule stores and the prompt callback remain owned by
// the engine so remembered decisions and user approval keep their semantics.
func (pe *PermissionEngine) CheckToolSnapshot(ctx context.Context, tc ToolCallInfo, snapshot PolicySnapshot) Decision {
	clone := *pe
	clone.Autonomy = snapshot.Autonomy
	clone.AutonomyExplicit = snapshot.AutonomyExplicit
	clone.SandboxMode = snapshot.SandboxMode
	clone.Stage = snapshot.Stage
	clone.DryRun = snapshot.DryRun
	clone.SpecSlug = snapshot.SpecSlug
	clone.Phase = snapshot.Phase
	clone.Phases = snapshot.Phases
	clone.Revision = snapshot.Revision
	clone.Memory = NewPermissionMemoryFromSnapshot(snapshot.Rules)
	return clone.CheckToolDecision(ctx, tc)
}

// CheckToolDecision returns structured policy metadata while preserving the
// existing permission behavior and human-readable messages.
func (pe *PermissionEngine) CheckToolDecision(ctx context.Context, tc ToolCallInfo) Decision {
	d := pe.evaluateToolDecision(ctx, tc)
	policy := ToolPolicyFor(tc.Name)
	d.Capabilities = policy.Capabilities
	d.Risk = policy.DefaultRisk
	d.Revision = pe.Revision
	return d
}

// EvaluateTool performs policy evaluation without waiting for UI approval.
// It returns DecisionAsk when the only remaining step is user approval.
// CheckToolDecision remains the compatibility API that performs the prompt.
func (pe *PermissionEngine) EvaluateTool(ctx context.Context, tc ToolCallInfo) Decision {
	clone := *pe
	clone.PromptFn = nil
	d := clone.CheckToolDecision(ctx, tc)
	if d.Reason == ReasonPromptUnavailable {
		d.Outcome = DecisionAsk
		d.Reason = ReasonUserPrompt
		d.Message = "Permission approval required."
	}
	return d
}

func (pe *PermissionEngine) evaluateToolDecision(ctx context.Context, tc ToolCallInfo) Decision {
	if pe.DryRun {
		return Decision{Outcome: DecisionDeny, Reason: ReasonDryRun, Message: "dry-run: tool execution disabled"}
	}

	// Governance ceiling — evaluated first so no later gate (hooks, spec
	// stage, remembered rules, autonomy, bypass kill-switch) can override an
	// administrator-set POLICY ∩ PROFILE decision. This is the un-disableable
	// org ceiling: deny in either layer denies regardless of what the agent
	// or user grants at lower layers.
	if pe.Governance != nil {
		if d := pe.Governance.Evaluate(tc.Name, ToolSummary(tc.Name, tc.Args)); !d.Allowed {
			return Decision{Outcome: DecisionDeny, Reason: ReasonGovernance, Message: d.Reason}
		}
	}

	toolName := canonicalToolName(tc.Name)

	// PreToolUse decision hooks — deny gate before autonomy. Hooks that
	// return allow/nil do not grant permission by themselves; they only
	// short-circuit when ActionDeny (or equivalent).
	if denied, reason := pe.checkPreToolHooks(tc); denied {
		return Decision{Outcome: DecisionDeny, Reason: ReasonHookDenied, Message: reason}
	}
	// Strict sandbox mode is read-only. This check is independent of autonomy
	// and the spec workflow so neither can turn a read-only sandbox into a
	// write or process-execution path.
	if pe.SandboxMode == sandbox.ModeStrict && !pe.strictToolAllowed(tc) {
		return Decision{Outcome: DecisionDeny, Reason: ReasonSandbox, Message: "Sandbox strict mode: tool execution is read-only."}
	}

	// Spec-stage gate — independent of trust tier, so no autonomy level can
	// bypass it. While a spec workflow is active and not yet approved for
	// implementation, only the workflow's own tools and reads may proceed.
	if pe.Stage != SpecStageNone && pe.Stage != SpecStageImplementing {
		switch toolName {
		case "Proposal", "Specify", "Design", "Plan", "Tasks", "AskUserQuestion", "SpecStatus", "SpecEdit", "SpecList", "SpecReset", "SpecConfig", "Clarify", "Analyze", "Checklist", "Constitution", "Converge":
			if !pe.specToolAllowed(toolName) {
				return Decision{Outcome: DecisionDeny, Reason: ReasonSpecGate, Message: pe.specStageReason(toolName)}
			}
			return Decision{Outcome: DecisionAllow, Reason: ReasonSpecGate}
		case "ApproveImplementation":
			if pe.Stage != SpecStageTasks {
				return Decision{Outcome: DecisionDeny, Reason: ReasonSpecGate, Message: "Spec stage active: ApproveImplementation is available only after Tasks completes."}
			}
			return pe.promptDecisionWithSummary(ctx, tc, specApprovalSummary(pe.SpecSlug))
		default:
			if tool.IsReadOnly(tc.Name) {
				return Decision{Outcome: DecisionAllow, Reason: ReasonSpecGate}
			}
			return Decision{Outcome: DecisionDeny, Reason: ReasonSpecGate, Message: "Spec stage active: only spec workflow tools (and reads) are allowed until ApproveImplementation."}
		}
	}

	summary := ToolSummary(tc.Name, tc.Args)
	// Destructive commands are hard-blocked regardless of autonomy, rule
	// memory, or the bypass kill-switch (H6). The tool layer independently
	// rejects them (IsDestructiveCommand in BashTool.Execute), but failing
	// closed here too keeps the policy engine authoritative and prevents the
	// bypass from even appearing to grant destructive commands. Placed before
	// the rule/auto/autonomy allow paths so nothing can override it.
	if toolName == "Bash" {
		if cmd, ok := tc.Args["command"].(string); ok && tool.IsDestructiveCommand(cmd) {
			return Decision{Outcome: DecisionDeny, Reason: ReasonRuleDenied, Message: "denied: destructive command is blocked even with bypass/autonomy enabled"}
		}
	}
	// Explicit remembered decisions are policy rules. They must be consulted
	// before autonomy can short-circuit the request, especially for deny rules.
	var memoryDecision *bool
	if pe.Memory != nil {
		memoryDecision = pe.Memory.Check(tc.Name, summary)
	}
	var autoDecision *bool
	if pe.AutoMode != nil {
		if allowed, ok := pe.AutoMode.ShouldAutoAllow(tc.Name, summary); ok {
			autoDecision = &allowed
		}
	}
	if memoryDecision != nil && !*memoryDecision {
		return Decision{Outcome: DecisionDeny, Reason: ReasonRuleDenied, Message: "Permission denied (rule)."}
	}
	if autoDecision != nil && !*autoDecision {
		return Decision{Outcome: DecisionDeny, Reason: ReasonAutoModeDenied, Message: "Permission denied (auto-mode)."}
	}
	if memoryDecision != nil && *memoryDecision {
		return Decision{Outcome: DecisionAllow, Reason: ReasonRuleAllowed}
	}
	if autoDecision != nil && *autoDecision {
		return Decision{Outcome: DecisionAllow, Reason: ReasonAutoModeAllowed}
	}

	isSafe := !ToolNeedsPermission(tc.Name, tc.Args)
	autoCfg := PresetConfig(pe.Autonomy)
	if !autoCfg.NeedsPermission(tc.Name, isSafe) {
		return Decision{Outcome: DecisionAllow, Reason: ReasonAutonomy}
	}
	if pe.BypassKill.IsEnabled() {
		// Audit bypass usage so there is a record of every tool call the
		// kill-switch approved (H6). Note the destructive-command hard-deny
		// above still applies: bypass cannot grant destructive commands.
		slog.Warn("permission bypass used", "tool", tc.Name, "summary", summary)
		return Decision{Outcome: DecisionAllow, Reason: ReasonBypass, Message: "bypass: permission checks bypassed"}
	}
	if pe.Classifier != nil && tc.Name == "Bash" {
		if pe.Classifier.Classify(summary) == "safe" {
			return Decision{Outcome: DecisionAllow, Reason: ReasonClassifiedSafe}
		}
	}
	return pe.promptDecision(ctx, tc)
}

func (pe *PermissionEngine) specToolAllowed(toolName string) bool {
	switch toolName {
	case "Proposal":
		return pe.Stage == SpecStageNone || pe.Stage == SpecStageProposal
	case "Specify", "Design":
		if pe.SpecSlug != "" && !pe.constitutionExists() {
			return false
		}
		return pe.Stage >= SpecStageProposal && pe.Stage < SpecStagePlan
	case "Plan":
		return pe.specDone&(doneSpecify|doneDesign) == doneSpecify|doneDesign
	case "Tasks":
		return pe.Stage == SpecStagePlan
	default:
		return true
	}
}

func (pe *PermissionEngine) constitutionExists() bool {
	if pe.SpecSlug == "" {
		return false
	}
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	path := filepath.Join(cwd, ".hawk", "specs", pe.SpecSlug, "constitution.md")
	_, err = os.Stat(path)
	return err == nil
}

func (pe *PermissionEngine) phaseGatesPass() bool {
	if pe.SpecSlug == "" {
		return false
	}
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	planPath := filepath.Join(cwd, ".hawk", "specs", pe.SpecSlug, "plan.md")
	data, err := os.ReadFile(planPath)
	if err != nil {
		return false
	}
	content := strings.ToLower(string(data))
	hasSimplicity := strings.Contains(content, "simplicity") || strings.Contains(content, "≤3") || strings.Contains(content, "<=3")
	hasAntiAbstraction := strings.Contains(content, "anti-abstraction") || strings.Contains(content, "framework directly")
	hasIntegrationFirst := strings.Contains(content, "integration-first") || strings.Contains(content, "contract")
	hasComplexityTracking := strings.Contains(content, "complexity tracking") || strings.Contains(content, "justification")
	return hasSimplicity && hasAntiAbstraction && hasIntegrationFirst && hasComplexityTracking
}

func (pe *PermissionEngine) unresolvedClarifications() int {
	if pe.SpecSlug == "" {
		return 0
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0
	}
	specPath := filepath.Join(cwd, ".hawk", "specs", pe.SpecSlug, "spec.md")
	data, err := os.ReadFile(specPath)
	if err != nil {
		return 0
	}
	matches := reNeedsClarify.FindAllString(string(data), -1)
	return len(matches)
}

func (pe *PermissionEngine) specStageReason(toolName string) string {
	switch toolName {
	case "Proposal":
		return "Proposal is only available when no spec workflow is active."
	case "Specify", "Design":
		if pe.SpecSlug != "" && !pe.constitutionExists() {
			return "Constitution required: call Constitution tool with action='init' before Specify/Design."
		}
		return "Spec stage active: Specify/Design require Proposal and must complete before Plan."
	case "Plan":
		if pe.specDone&(doneSpecify|doneDesign) != doneSpecify|doneDesign {
			return "Spec stage active: Plan requires both Specify and Design to be complete."
		}
		if pe.unresolvedClarifications() > 0 {
			return fmt.Sprintf("Spec stage active: resolve %d [NEEDS CLARIFICATION] marker(s) before advancing to Plan.", pe.unresolvedClarifications())
		}
		return "Spec stage active: Plan phase gates not documented."
	case "Tasks":
		return "Spec stage active: Tasks is available only after Plan completes."
	default:
		return "Spec stage active: tool is not available at the current stage."
	}
}

func (pe *PermissionEngine) strictToolAllowed(tc ToolCallInfo) bool {
	name := canonicalToolName(tc.Name)
	if tool.IsReadOnly(tc.Name) || name == "ApproveImplementation" {
		return true
	}
	switch name {
	case "AskUserQuestion", "SpecStatus", "SpecList", "Clarify", "Analyze", "Checklist", "Constitution", "Converge":
		return true
	case "SpecConfig":
		action, _ := tc.Args["action"].(string)
		return strings.ToLower(strings.TrimSpace(action)) != "set"
	default:
		return false
	}
}

// checkPreToolHooks runs decision hooks for PreToolUse / pre_tool.
// Returns (denied, reason).
func (pe *PermissionEngine) checkPreToolHooks(tc ToolCallInfo) (bool, string) {
	data := map[string]interface{}{
		"tool":    tc.Name,
		"tool_id": tc.ID,
		"args":    tc.Args,
	}
	// Matchers accepting PreToolUse or pre_tool both match via CanonicalEvent.
	d := hooks.ExecuteDecisionHooks(string(hooks.EventPreTool), data)
	if d == nil {
		return false, ""
	}
	switch d.Action {
	case hooks.ActionDeny:
		msg := d.Message
		if msg == "" {
			msg = d.Reason
		}
		if msg == "" {
			msg = "Permission denied (PreToolUse hook)."
		}
		return true, msg
	default:
		// allow / modify / instruct: do not grant; continue pipeline.
		// Modify of args is applied later by stream layer if needed.
		return false, ""
	}
}

func (pe *PermissionEngine) promptDecision(ctx context.Context, tc ToolCallInfo) Decision {
	return pe.promptDecisionWithSummary(ctx, tc, ToolSummary(tc.Name, tc.Args))
}

func (pe *PermissionEngine) promptDecisionWithSummary(ctx context.Context, tc ToolCallInfo, summary string) Decision {
	if pe.PromptFn == nil {
		return Decision{Outcome: DecisionDeny, Reason: ReasonPromptUnavailable, Message: "Permission prompt unavailable."}
	}
	resp := make(chan bool, 1)
	pe.PromptFn(PermissionRequest{
		PermissionRequest: contracts.PermissionRequest{
			ToolName: tc.Name,
			ToolID:   tc.ID,
			Summary:  summary,
		},
		Response: resp,
	})
	select {
	case allowed := <-resp:
		if !allowed {
			return Decision{Outcome: DecisionDeny, Reason: ReasonUserPrompt, Message: "Permission denied by user."}
		}
		return Decision{Outcome: DecisionAllow, Reason: ReasonUserPrompt}
	case <-ctx.Done():
		return Decision{Outcome: DecisionDeny, Reason: ReasonUserPrompt, Message: "Permission prompt cancelled."}
	case <-time.After(5 * time.Minute):
		return Decision{Outcome: DecisionDeny, Reason: ReasonUserPrompt, Message: "Permission prompt timed out."}
	}
}

// detectPhases counts numbered phase sections in tasks.md.
func detectPhases(slug string) int {
	if slug == "" {
		return 0
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0
	}
	tasksPath := filepath.Join(cwd, ".hawk", "specs", slug, "tasks.md")
	data, err := os.ReadFile(tasksPath) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return 0
	}
	return len(rePhaseSection.FindAllString(string(data), -1))
}

var rePhaseSection = regexp.MustCompile(`(?m)^## \d+\.`)

// specApprovalSummary reads spec.md/plan.md/tasks.md from the active spec's
// directory and builds a short preview for the ApproveImplementation
// prompt, so approving isn't a blind yes/no — the user sees what they're
// actually signing off on. Falls back to a plain name if the slug is empty
// or the files can't be read (e.g. deleted after being written).
func specApprovalSummary(slug string) string {
	phases := detectPhases(slug)
	if slug == "" {
		return "ApproveImplementation"
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "ApproveImplementation"
	}
	dir := filepath.Join(cwd, ".hawk", "specs", slug)

	var b strings.Builder
	for _, f := range []string{"proposal.md", "spec.md", "design.md", "plan.md", "tasks.md"} {
		content, err := os.ReadFile(filepath.Join(dir, f)) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
		if err != nil {
			continue
		}
		preview := strings.TrimSpace(string(content))
		const maxPreview = 300
		if len(preview) > maxPreview {
			preview = preview[:maxPreview] + "..."
		}
		fmt.Fprintf(&b, "## %s\n%s\n\n", f, preview)
	}
	if phases > 0 {
		fmt.Fprintf(&b, "Phases: %d\n", phases)
	}
	if b.Len() == 0 {
		return "ApproveImplementation"
	}
	return strings.TrimSpace(b.String())
}

// AdvancePhase increments the phase gate when the model completes the
// current phase's tasks and calls AdvancePhase.
func (pe *PermissionEngine) AdvancePhase() {
	if pe.Phase < pe.Phases {
		pe.Phase++
	}
}

// PhaseProgress returns a summary of phase completion for display.
func (pe *PermissionEngine) PhaseProgress() string {
	if pe.Phases <= 0 {
		return ""
	}
	return fmt.Sprintf("Phase %d/%d", pe.Phase, pe.Phases)
}

// AdvanceSpecStage updates Stage based on which spec-workflow tool the
// model just executed successfully. Called by stream_tool_exec.go — plays
// the same role ApplyToolState played for the old Plan Mode.
func (pe *PermissionEngine) AdvanceSpecStage(name string) {
	if canonicalToolName(name) == "SpecReset" {
		pe.Stage = SpecStageNone
		pe.SpecSlug = ""
		pe.specDone = 0
		pe.Phase = 0
		pe.Phases = 0
		pe.convergeChecked = false
		pe.Revision++
		return
	}
	w := SpecWorkflow{Stage: pe.Stage, Slug: pe.SpecSlug, Done: pe.specDone}
	if err := w.Transition(name, pe.SpecSlug); err != nil {
		return
	}
	pe.Stage, pe.SpecSlug, pe.specDone = w.Stage, w.Slug, w.Done
	pe.Revision++
	if canonicalToolName(name) == "ApproveImplementation" {
		pe.Phase = 1
		pe.Phases = detectPhases(pe.SpecSlug)
		pe.convergeChecked = false
	}
	if canonicalToolName(name) == "Plan" {
		if pe.unresolvedClarifications() > 0 {
			pe.Stage = SpecStageDesign
		} else if !pe.phaseGatesPass() {
			pe.Stage = SpecStageDesign
		}
	}
}

// ToolCallInfo is a minimal struct for permission checking.
type ToolCallInfo struct {
	Name string
	ID   string
	Args map[string]interface{}
}
