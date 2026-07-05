package engine

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/spec"
)

// specConfigForPrompt loads the user's spec configuration and returns it
// formatted for injection into the system prompt. Returns "" if no config
// exists or all fields are empty.
func specConfigForPrompt() string {
	cfg := spec.LoadSpecConfig()
	return cfg.FormatForPrompt()
}

// specStageSystemPrompt is appended to the system prompt (ephemerally) while
// a spec workflow is active and not yet approved for implementation. It
// steers the model through discovery → Specify → Plan → Tasks → approval,
// mirroring the old Plan Mode's research-then-approve shape but with real,
// persisted documents at each stage.
const specStageSystemPrompt = "\n\n## Spec Stage (workflow gate)\n" +
	"You are working through a spec-driven workflow. Research is unrestricted, but write/execute tools are blocked until you complete the workflow. " +
	"\n\n### Workflow\n" +
	"1. **Discovery** (before writing anything): " +
	"If the user's request is vague, unclear, or has multiple valid interpretations, ask clarifying questions first. " +
	"Ask about language, framework, architecture, methodology, or anything else you need to know. " +
	"Present options with tradeoffs when choices are meaningful. " +
	"You can also surface assumptions explicitly and ask the user to confirm or correct them. " +
	"Use the `AskUser` tool for questions — you can ask one at a time or batch them. " +
	"There is no limit on questions — ask what you need. " +
	"If the user says 'you decide' or gives you freedom, make reasonable choices based on the codebase context.\n" +
	"2. **Specify**: Call `Specify` with your full understanding to write spec.md. " +
	"Use `[NEEDS CLARIFICATION: ...]` markers in the spec for any remaining unknowns (max 3 unresolved at a time).\n" +
	"3. **Plan**: Call `Plan` with your technical approach to write plan.md.\n" +
	"4. **Tasks**: Call `Tasks` with a breakdown to write tasks.md.\n" +
	"5. **Approve**: Call `ApproveImplementation` to ask the user to approve moving to implementation. " +
	"Only after they approve will Write/Edit/Bash be permitted.\n" +
	"\n### Quality checks\n" +
	"- Spec should focus on WHAT and WHY, not HOW (no implementation details).\n" +
	"- Requirements should be testable, unambiguous, with measurable success criteria.\n" +
	"- Edge cases, scope boundaries, and assumptions should be documented.\n" +
	"- Tasks must use `- [ ]` checkbox format.\n" +
	"\nUse `SpecConfig` tool to check user's language/framework/methodology/architecture preferences. " +
	"Use `SpecList` to see existing specs. Use `SpecEdit` to refine artifacts mid-workflow."

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
