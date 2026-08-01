package safety

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	contracts "github.com/GrayCodeAI/hawk-core-contracts/policy"
	"github.com/GrayCodeAI/hawk/internal/hooks"
	"github.com/GrayCodeAI/hawk/internal/permissions"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// SpecStage tracks position in the independent spec-driven-development
// workflow. It is orthogonal to AutonomyLevel: a session can be at any
// trust tier while at any spec stage — trust governs *how* a tool call is
// approved, spec stage governs *which* tools are relevant to call right now.
type SpecStage int

const (
	SpecStageNone SpecStage = iota // no active spec workflow
	SpecStageSpecify
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
	// SandboxMode controls filesystem/process policy for tool execution. It is
	// deliberately separate from Autonomy: autonomy decides whether a user
	// prompt is needed, while the sandbox decides what the tool may actually do.
	SandboxMode sandbox.Mode
	Stage       SpecStage
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
	Phase    int
	Phases   int                     // total number of phases detected from tasks.md
	PromptFn func(PermissionRequest) // callback to ask user
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
)

// Decision is the structured result of a permission evaluation.
type Decision struct {
	Outcome DecisionOutcome
	Reason  DecisionReason
	Message string
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
}

// Snapshot returns a copy of the engine's request-relevant scalar policy.
func (pe *PermissionEngine) Snapshot() PolicySnapshot {
	return PolicySnapshot{Autonomy: pe.Autonomy, AutonomyExplicit: pe.AutonomyExplicit,
		SandboxMode: pe.SandboxMode, Stage: pe.Stage, DryRun: pe.DryRun,
		SpecSlug: pe.SpecSlug, Phase: pe.Phase, Phases: pe.Phases}
}

// NewPermissionEngine creates a PermissionEngine with sensible defaults.
func NewPermissionEngine() *PermissionEngine {
	return &PermissionEngine{
		Memory:     NewPermissionMemory(),
		AutoMode:   permissions.NewAutoModeState(),
		Classifier: permissions.NewClassifier(),
		BypassKill: permissions.NewBypassKillswitch(),
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
	return clone.CheckToolDecision(ctx, tc)
}

// CheckToolDecision returns structured policy metadata while preserving the
// existing permission behavior and human-readable messages.
func (pe *PermissionEngine) CheckToolDecision(ctx context.Context, tc ToolCallInfo) Decision {
	if pe.DryRun {
		return Decision{Outcome: DecisionDeny, Reason: ReasonDryRun, Message: "dry-run: tool execution disabled"}
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
		case "Specify", "Plan", "Tasks", "AskUserQuestion", "SpecStatus", "SpecEdit", "SpecList", "SpecReset", "SpecConfig", "Clarify", "Analyze", "Checklist", "Constitution", "Converge":
			if !pe.specToolAllowed(toolName) {
				return Decision{Outcome: DecisionDeny, Reason: ReasonSpecGate, Message: pe.specStageReason(toolName)}
			}
			return Decision{Outcome: DecisionAllow, Reason: ReasonSpecGate}
		case "ApproveImplementation":
			if pe.Stage != SpecStageTasks {
				return Decision{Outcome: DecisionDeny, Reason: ReasonSpecGate, Message: "Spec stage active: ApproveImplementation is available only after Tasks completes."}
			}
			// Always a real human decision — never auto-allowed by tier,
			// bypass-kill, or auto-mode, unlike everything below. Show the
			// actual spec/plan/tasks content in the prompt rather than a
			// bare tool name, so approval isn't a blind yes/no.
			return pe.promptDecisionWithSummary(ctx, tc, specApprovalSummary(pe.SpecSlug))
		default:
			if tool.IsReadOnly(tc.Name) {
				return Decision{Outcome: DecisionAllow, Reason: ReasonSpecGate}
			}
			return Decision{Outcome: DecisionDeny, Reason: ReasonSpecGate, Message: "Spec stage active: only Specify/Plan/Tasks (and reads) are allowed until ApproveImplementation."}
		}
	}

	summary := ToolSummary(tc.Name, tc.Args)
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
		return Decision{Outcome: DecisionAllow, Reason: ReasonBypass}
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
	case "Specify":
		return pe.Stage == SpecStageSpecify
	case "Plan":
		return pe.Stage == SpecStageSpecify && pe.SpecSlug != ""
	case "Tasks":
		return pe.Stage == SpecStagePlan
	default:
		return true
	}
}

func (pe *PermissionEngine) specStageReason(toolName string) string {
	switch toolName {
	case "Plan":
		return "Spec stage active: Plan is available only after Specify completes."
	case "Tasks":
		return "Spec stage active: Tasks is available only after Plan completes."
	case "Specify":
		return "Spec stage active: Specify is not available at the current stage."
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

// promptUser blocks on PromptFn, asking the user to approve tc, using the
// generic tool summary.
func (pe *PermissionEngine) promptUser(ctx context.Context, tc ToolCallInfo) (bool, string) {
	d := pe.promptDecision(ctx, tc)
	return d.Outcome == DecisionAllow, d.Message
}

// promptUserWithSummary is promptUser with a caller-supplied summary,
// letting ApproveImplementation show spec/plan/tasks content instead of
// the generic (and, since it takes no args, empty) tool summary.
func (pe *PermissionEngine) promptUserWithSummary(ctx context.Context, tc ToolCallInfo, summary string) (bool, string) {
	d := pe.promptDecisionWithSummary(ctx, tc, summary)
	return d.Outcome == DecisionAllow, d.Message
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
	for _, f := range []string{"spec.md", "plan.md", "tasks.md"} {
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
	switch canonicalToolName(name) {
	case "Specify":
		pe.Stage = SpecStageSpecify
	case "Plan":
		pe.Stage = SpecStagePlan
	case "Tasks":
		pe.Stage = SpecStageTasks
	case "ApproveImplementation":
		pe.Stage = SpecStageImplementing
		pe.Phase = 1
		pe.Phases = detectPhases(pe.SpecSlug)
	}
}

// ToolCallInfo is a minimal struct for permission checking.
type ToolCallInfo struct {
	Name string
	ID   string
	Args map[string]interface{}
}
