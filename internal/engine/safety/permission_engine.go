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
	"github.com/GrayCodeAI/hawk/internal/permissions"
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
	Memory     *PermissionMemory
	AutoMode   *permissions.AutoModeState
	Classifier *permissions.Classifier
	BypassKill *permissions.BypassKillswitch
	Autonomy   AutonomyLevel
	Stage      SpecStage
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
func (pe *PermissionEngine) CheckTool(ctx context.Context, tc ToolCallInfo) (bool, string) {
	if pe.DryRun {
		return false, "dry-run: tool execution disabled"
	}

	toolName := canonicalToolName(tc.Name)

	// Spec-stage gate — checked first, independent of trust tier, so no
	// autonomy level can ever bypass it (the bug the old Mode/Autonomy
	// split had: a high tier could short-circuit before Plan Mode's check
	// ever ran). While a spec workflow is active and not yet approved for
	// implementation, only the workflow's own tools and reads may proceed.
	if pe.Stage != SpecStageNone && pe.Stage != SpecStageImplementing {
		switch toolName {
		case "Specify", "Plan", "Tasks":
			return true, ""
		case "ApproveImplementation":
			// Always a real human decision — never auto-allowed by tier,
			// bypass-kill, or auto-mode, unlike everything below. Show the
			// actual spec/plan/tasks content in the prompt rather than a
			// bare tool name, so approval isn't a blind yes/no.
			return pe.promptUserWithSummary(ctx, tc, specApprovalSummary(pe.SpecSlug))
		default:
			if tool.IsReadOnly(tc.Name) {
				return true, ""
			}
			return false, "Spec stage active: only Specify/Plan/Tasks (and reads) are allowed until ApproveImplementation."
		}
	}

	isSafe := !ToolNeedsPermission(tc.Name, tc.Args)
	autoCfg := PresetConfig(pe.Autonomy)
	if !autoCfg.NeedsPermission(tc.Name, isSafe) {
		return true, ""
	}
	if pe.BypassKill.IsEnabled() {
		return true, ""
	}
	summary := ToolSummary(tc.Name, tc.Args)
	if pe.Classifier != nil && tc.Name == "Bash" {
		if pe.Classifier.Classify(summary) == "safe" {
			return true, ""
		}
	}
	if pe.AutoMode != nil {
		if allowed, ok := pe.AutoMode.ShouldAutoAllow(tc.Name, summary); ok {
			if allowed {
				return true, ""
			}
			return false, "Permission denied (auto-mode)."
		}
	}
	if decision := pe.Memory.Check(tc.Name, summary); decision != nil {
		if !*decision {
			return false, "Permission denied (rule)."
		}
		return true, ""
	}
	return pe.promptUser(ctx, tc)
}

// promptUser blocks on PromptFn, asking the user to approve tc, using the
// generic tool summary.
func (pe *PermissionEngine) promptUser(ctx context.Context, tc ToolCallInfo) (bool, string) {
	return pe.promptUserWithSummary(ctx, tc, ToolSummary(tc.Name, tc.Args))
}

// promptUserWithSummary is promptUser with a caller-supplied summary,
// letting ApproveImplementation show spec/plan/tasks content instead of
// the generic (and, since it takes no args, empty) tool summary.
func (pe *PermissionEngine) promptUserWithSummary(ctx context.Context, tc ToolCallInfo, summary string) (bool, string) {
	if pe.PromptFn == nil {
		return false, "Permission prompt unavailable."
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
			return false, "Permission denied by user."
		}
		return true, ""
	case <-ctx.Done():
		return false, "Permission prompt cancelled."
	case <-time.After(5 * time.Minute):
		return false, "Permission prompt timed out."
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
