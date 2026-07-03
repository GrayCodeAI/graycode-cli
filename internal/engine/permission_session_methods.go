package engine

import (
	"fmt"
	"strings"
)

// specStageSystemPrompt is appended to the system prompt (ephemerally) while
// a spec workflow is active and not yet approved for implementation. It
// steers the model through Specify -> Plan -> Tasks and then an explicit
// approval handoff, mirroring the old Plan Mode's research-then-approve
// shape but with a real, persisted document at each stage.
const specStageSystemPrompt = "\n\n## Spec Stage (workflow gate)\n" +
	"You are working through a spec-driven workflow. Research is unrestricted, but write/execute tools are blocked until you complete the workflow. " +
	"Call `Specify` with your understanding of the problem to write spec.md. Then call `Plan` with your technical approach to write plan.md. " +
	"Then call `Tasks` with a breakdown to write tasks.md. " +
	"When all three are written, call `ApproveImplementation` to ask the user to approve moving to implementation — only after they approve will Write/Edit/Bash be permitted."

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

func (s *Session) exceededBudget() bool {
	return s.MaxBudgetUSD > 0 && s.Cost.Total() > s.MaxBudgetUSD
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
	case "specify":
		return "Specify"
	case "plan":
		return "Plan"
	case "tasks":
		return "Tasks"
	case "approve_implementation", "approveimplementation":
		return "ApproveImplementation"
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
