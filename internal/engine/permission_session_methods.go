package engine

import (
	"fmt"
	"strings"
)

func (s *Session) SetPermissionMode(mode string) error {
	err := s.Perm.SetMode(mode)
	if err == nil {
		s.Mode = s.Perm.Mode
	}
	return err
}

func (s *Session) SetMaxTurns(turns int) error {
	if turns < 0 {
		return fmt.Errorf("max turns must be non-negative")
	}
	s.MaxTurns = turns
	return nil
}

func (s *Session) SetMaxBudgetUSD(amount float64) error {
	if amount < 0 {
		return fmt.Errorf("max budget must be non-negative")
	}
	s.MaxBudgetUSD = amount
	return nil
}

func (s *Session) modeDecision(name string) *bool {
	toolName := canonicalToolName(name)
	switch s.Mode {
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

func (s *Session) exceededBudget() bool {
	return s.MaxBudgetUSD > 0 && s.Cost.Total() > s.MaxBudgetUSD
}

func boolPtr(v bool) *bool {
	return &v
}

func pathArgument(args map[string]interface{}) (string, bool) {
	if p, ok := args["path"].(string); ok && p != "" {
		return p, true
	}
	if p, ok := args["file_path"].(string); ok && p != "" {
		return p, true
	}
	return "", false
}

func canonicalToolName(name string) string {
	switch strings.ToLower(name) {
	case "bash":
		return "Bash"
	case "file_read", "read":
		return "Read"
	case "file_write", "write":
		return "Write"
	case "file_edit", "edit":
		return "Edit"
	case "ls":
		return "LS"
	case "glob":
		return "Glob"
	case "grep":
		return "Grep"
	case "web_fetch", "webfetch":
		return "WebFetch"
	case "web_search", "websearch":
		return "WebSearch"
	case "agent", "task":
		return "Agent"
	case "ask_user", "askuserquestion":
		return "AskUserQuestion"
	case "todo", "todowrite":
		return "TodoWrite"
	case "lsp":
		return "LSP"
	case "enter_plan_mode", "enterplanmode":
		return "EnterPlanMode"
	case "exit_plan_mode", "exitplanmode":
		return "ExitPlanMode"
	case "notebook_edit", "notebookedit":
		return "NotebookEdit"
	case "config":
		return "Config"
	case "brief", "sendusermessage":
		return "SendUserMessage"
	default:
		return name
	}
}
