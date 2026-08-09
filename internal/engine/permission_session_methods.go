package engine

import (
	"fmt"
	"os"
	"path/filepath"
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
// steers the model through the full spec-driven workflow.
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
	"2. **Proposal**: Call `Proposal` to write proposal.md — establish WHY this change is needed (problem, goals, scope, success criteria).\n" +
	"3. **Constitution**: Call `Constitution` with action='init' to create the project constitution if none exists. " +
	"The constitution defines non-negotiable rules that guide all subsequent decisions.\n" +
	"4. **Specify** + **Design** (parallel): After Proposal and Constitution, call both `Specify` (requirements — WHAT the system does) and `Design` (technical approach — HOW). These can be done in either order or concurrently.\n" +
	"5. **Plan**: Call `Plan` with your implementation plan to write plan.md. Requires both Specify and Design to be complete. " +
	"Plan must document phase gate compliance (Simplicity, Anti-Abstraction, Integration-First).\n" +
	"6. **Tasks**: Call `Tasks` with a breakdown to write tasks.md.\n" +
	"7. **Approve**: Call `ApproveImplementation` to ask the user to approve moving to implementation. " +
	"Only after they approve will Write/Edit/Bash be permitted.\n" +
	"\n### Quality checks\n" +
	"- Proposal should be concise (1-2 pages) focusing on WHY.\n" +
	"- Spec should focus on WHAT (requirements, scenarios), not HOW.\n" +
	"- Design should focus on HOW (architecture, decisions, trade-offs).\n" +
	"- Requirements should use EARS notation (The system shall... / WHEN...THEN...).\n" +
	"- Each requirement gets a REQ-XXX.Y.Z identifier for traceability.\n" +
	"- Requirements should be testable, unambiguous, with measurable success criteria.\n" +
	"- Edge cases, scope boundaries, and assumptions should be documented.\n" +
	"- Tasks must use `- [ ]` checkbox format and reference REQ IDs.\n" +
	"- Use [NEEDS CLARIFICATION: question] markers instead of guessing (max 3 at a time).\n" +
	"\nUse `SpecConfig` tool to check user's language/framework/methodology/architecture preferences. " +
	"Use `SpecList` to see existing specs. Use `SpecEdit` to refine artifacts mid-workflow. " +
	"Use `Constitution` tool to create/update project governing principles."

func constitutionForPrompt(slug string) string {
	if slug == "" {
		return ""
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	path := filepath.Join(cwd, ".hawk", "specs", slug, "constitution.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return "\n\n## Project Constitution (active)\n" +
		"The following constitution governs all decisions in this spec workflow. " +
		"Every artifact you create must comply with these principles.\n\n" +
		string(data)
}

func (s *Session) SetMaxTurns(turns int) error {
	if turns < 0 {
		return fmt.Errorf("max turns must be non-negative")
	}
	if s.LifecycleSvc() != nil {
		s.LifecycleSvc().Limits().SetMaxTurns(turns)
	}
	return nil
}

func (s *Session) SetMaxBudgetUSD(amount float64) error {
	if amount < 0 {
		return fmt.Errorf("max budget must be non-negative")
	}
	if s.LifecycleSvc() != nil {
		s.LifecycleSvc().Limits().SetMaxBudgetUSD(amount)
	}
	if tracker := s.currentTokUsageTracker(); tracker != nil {
		limits := tracker.GetLimits()
		limits.CostUSD = amount
		tracker.SetLimits(limits)
	}
	return nil
}

// ApplyTokUsageSettings optionally enables tok.UsageTracker token ceilings.
// Values: 0 or -1 leave/disable the ceiling (provider owns rate limits);
// >0 opts into a local cap.
func (s *Session) ApplyTokUsageSettings(hourly, daily, session int) {
	if s == nil {
		return
	}
	if hourly == 0 && daily == 0 && session == 0 {
		return
	}
	tracker := s.ensureTokUsageTracker()
	limits := tracker.GetLimits()
	if hourly == -1 {
		limits.HourlyTokens = 0
	} else if hourly > 0 {
		limits.HourlyTokens = hourly
	}
	if daily == -1 {
		limits.DailyTokens = 0
	} else if daily > 0 {
		limits.DailyTokens = daily
	}
	if session == -1 {
		limits.SessionTokens = 0
	} else if session > 0 {
		limits.SessionTokens = session
	}
	tracker.SetLimits(limits)
}

func (s *Session) exceededBudget() bool {
	return s.LifecycleSvc().Limits().MaxBudgetUSD() > 0 && s.Cost.Total() > s.LifecycleSvc().Limits().MaxBudgetUSD()
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
	case "sql", "sql_query":
		return "SQL"
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
