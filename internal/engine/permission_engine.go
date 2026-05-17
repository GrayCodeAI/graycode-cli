package engine

import (
	"context"
	"time"

	"github.com/GrayCodeAI/hawk/permissions"
)

// PermissionEngine encapsulates all permission-checking logic.
// Extracted from Session to keep the god object lean.
type PermissionEngine struct {
	Memory     *PermissionMemory
	AutoMode   *permissions.AutoModeState
	Classifier *permissions.Classifier
	BypassKill *permissions.BypassKillswitch
	Mode       PermissionMode
	Autonomy   AutonomyLevel
	PromptFn   func(PermissionRequest) // callback to ask user
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

// SetMode applies a permission mode string.
func (pe *PermissionEngine) SetMode(mode string) error {
	return setPermissionMode(&pe.Mode, mode)
}

// CheckTool determines if a tool call is allowed, denied, or needs user prompt.
// Returns (granted bool, denyReason string).
// If the user must be asked, it blocks on PromptFn with a 5-minute timeout.
func (pe *PermissionEngine) CheckTool(ctx context.Context, tc toolCallInfo) (bool, string) {
	isSafe := !toolNeedsPermission(tc.Name, tc.Args)
	autoCfg := PresetConfig(pe.Autonomy)
	if !autoCfg.NeedsPermission(tc.Name, isSafe) || pe.PromptFn == nil {
		return true, ""
	}

	summary := toolSummary(tc.Name, tc.Args)

	if pe.BypassKill.IsEnabled() {
		return true, ""
	}
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
	if decision := pe.modeDecision(tc.Name); decision != nil {
		if !*decision {
			return false, "Permission denied by permission mode."
		}
		return true, ""
	}
	if decision := pe.Memory.Check(tc.Name, summary); decision != nil {
		if !*decision {
			return false, "Permission denied (rule)."
		}
		return true, ""
	}

	// Ask user
	resp := make(chan bool, 1)
	pe.PromptFn(PermissionRequest{
		ToolName: tc.Name,
		ToolID:   tc.ID,
		Summary:  summary,
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

func (pe *PermissionEngine) modeDecision(name string) *bool {
	toolName := canonicalToolName(name)
	switch pe.Mode {
	case PermissionModeBypassPermissions:
		return boolPtr(true)
	case PermissionModeDontAsk:
		return boolPtr(false)
	case PermissionModePlan:
		if toolName == "ExitPlanMode" {
			return nil
		}
		return boolPtr(false)
	case PermissionModeAcceptEdits:
		if toolName == "Write" || toolName == "Edit" || toolName == "NotebookEdit" {
			return boolPtr(true)
		}
	}
	return nil
}

// ApplyToolState updates permission mode based on plan mode tools.
func (pe *PermissionEngine) ApplyToolState(name string) {
	switch canonicalToolName(name) {
	case "EnterPlanMode":
		pe.Mode = PermissionModePlan
	case "ExitPlanMode":
		pe.Mode = PermissionModeDefault
	}
}

// toolCallInfo is a minimal struct for permission checking.
type toolCallInfo struct {
	Name string
	ID   string
	Args map[string]interface{}
}
