package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// UserLevel represents the user's experience level.
type UserLevel int

const (
	LevelBeginner     UserLevel = iota // simplified help, fewer commands shown
	LevelIntermediate                  // standard help
	LevelAdvanced                      // all commands and options shown
)

func (l UserLevel) String() string {
	switch l {
	case LevelBeginner:
		return "beginner"
	case LevelIntermediate:
		return "intermediate"
	case LevelAdvanced:
		return "advanced"
	default:
		return "intermediate"
	}
}

// ParseUserLevel parses a level string.
func ParseUserLevel(s string) (UserLevel, bool) {
	switch strings.ToLower(s) {
	case "beginner", "new", "novice":
		return LevelBeginner, true
	case "intermediate", "standard", "normal":
		return LevelIntermediate, true
	case "advanced", "expert", "pro":
		return LevelAdvanced, true
	default:
		return LevelIntermediate, false
	}
}

// DisclosureConfig tracks the user's preferred disclosure level.
type DisclosureConfig struct {
	Level UserLevel
}

// DefaultDisclosureConfig returns intermediate as default.
func DefaultDisclosureConfig() DisclosureConfig {
	return DisclosureConfig{Level: LevelIntermediate}
}

// BeginnerHelp returns a simplified help message for new users.
func BeginnerHelp() string {
	return `Getting Started with hawk

  Type your question or task and hawk will help.
  hawk reads your project and understands your code.

Essential Commands:
  /help     Show all commands
  /test     Run your project's tests
  /diff     See what you've changed
  /commit   Save your work with a smart message
  /review   Have hawk review your code
  /clear    Start a fresh conversation

Tips:
  - Just describe what you want in plain English
  - hawk will read files, run commands, and make changes
  - Use /help to discover more features as you get comfortable

Type anything to get started!`
}

// IntermediateHelp returns the standard help message.
func IntermediateHelp() string {
	return `hawk Commands

Workflow:
  /test        Run tests and fix failures
  /review      Code review for bugs and issues
  /commit      Auto-commit with AI message
  /diff        Show working diff
  /lint        Run linter
  /check       Full pre-ship check

Context:
  /context     Show current context
  /add <file>  Add file to context
  /drop <file> Remove file from context
  /focus <dir> Narrow agent attention

Session:
  /status      Show session info
  /compact     Compress conversation
  /clear       Clear conversation
  /history     List saved sessions
  /resume <id> Resume a session
  /export      Export session

Tools:
  /tools       List enabled tools
  /mcp         Show MCP servers
  /skills      Community skills
  /snapshot    Manage file snapshots

Use /help all for the complete list.`
}

// AdvancedHelp returns the full command reference.
func AdvancedHelp() string {
	return `hawk Full Command Reference

Workflow:
  /test [cmd]        Run tests (default: go test ./...)
  /review            Code review for bugs and issues
  /commit            Auto-commit changes
  /diff              Show git diff
  /lint [cmd]        Run linter
  /check             Full pre-ship check (review + fix + verify)
  /plan              Enter read-only planning mode
  /research <cmd>    Autonomous research loop
  /vibe              Enter vibe coding mode
  /think <topic>     Turn idea into approved plan
  /hunt <symptom>    Diagnose root cause

Context:
  /context           Show current context
  /context init      Initialize project context
  /context show      Show context files
  /add <file>        Add file to context
  /add-dir <path>    Add directory to context
  /drop <file>       Remove file from context
  /focus <dir>       Narrow agent attention
  /pin [n]           Pin last N messages
  /render            Export as CXML to clipboard

Session:
  /status            Show session info
  /session           Show session info
  /compact           Compress conversation (LLM summary)
  /clear             Clear conversation
  /new               Start fresh session
  /history           List saved sessions
  /resume <id>       Resume a session
  /recover           Scan for interrupted sessions
  /rename <name>     Rename session
  /tag <label>       Tag session
  /export            Export session to JSON
  /share             Share session as markdown
  /search <query>    Search across sessions
  /fork              Fork conversation
  /branches          List conversation branches
  /rewind            Undo last exchange
  /retry             Redo last message
  /undo              Undo last file change

Tools & Plugins:
  /tools             List enabled tools
  /mcp               Show MCP servers
  /skills            Community skills
  /plugins           List installed plugins
  /snapshot          Manage file snapshots
  /recipe [name]     Run a recipe
  /hooks             Show configured hooks

Model & Config:
  /model [name]      Switch or view model
  /config            Open settings panel
  /provider-status   Show provider info
  /fast              Toggle fast mode
  /effort <level>    Set reasoning effort
  /power <1-10>      Set power level
  /output-style      Set verbosity

Agents:
  /agents            List active agents
  /agents-init       Generate AGENTS.md
  /council <q>       Multi-model consensus
  /exec              Execute non-interactively

Memory & Intelligence:
  /memory            Show AGENTS.md instructions
  /yaad              Show yaad memory graph
  /remember          Store in memory
  /recall            Search memory
  /taste             Show learned preferences
  /dream             Memory consolidation
  /away              Generate session recap

Diagnostics:
  /doctor            Run health diagnostics
  /path              Developer path readiness
  /cost              Token usage and cost
  /tokens            Token estimate
  /metrics           Session metrics
  /stats             Analytics stats
  /audit             Tool audit summary
  /integrity         Session integrity check
  /stale             Show stale rules

System:
  /version           Show version
  /env               Show environment
  /sandbox           Toggle approval mode
  /yolo              Toggle auto-approve
  /vim               Toggle vim mode
  /theme <t>         Set theme
  /voice             Toggle voice input
  /upgrade           Check for updates
  /feedback <msg>    Submit feedback
  /quit              Save and exit`
}

// GetHelpForLevel returns the appropriate help text for the user's level.
func GetHelpForLevel(level UserLevel) string {
	switch level {
	case LevelBeginner:
		return BeginnerHelp()
	case LevelAdvanced:
		return AdvancedHelp()
	default:
		return IntermediateHelp()
	}
}

// disclosureCmd is the cobra command for managing disclosure level.
var disclosureCmd = &cobra.Command{
	Use:   "level [beginner|intermediate|advanced]",
	Short: "Set feature disclosure level",
	Long:  "Control how many features are shown. Beginner shows essentials, advanced shows everything.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDisclosure,
}

var currentDisclosure = DefaultDisclosureConfig()

func init() {
	// This is registered as a chat command, not a cobra subcommand
}

func runDisclosure(_ *cobra.Command, args []string) error {
	if len(args) == 0 {
		fmt.Printf("Current level: %s\n", currentDisclosure.Level)
		fmt.Println("Available: beginner, intermediate, advanced")
		return nil
	}
	level, ok := ParseUserLevel(args[0])
	if !ok {
		return fmt.Errorf("unknown level %q; use beginner, intermediate, or advanced", args[0])
	}
	currentDisclosure.Level = level
	fmt.Printf("Disclosure level set to: %s\n", level)
	return nil
}
