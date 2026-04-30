package cmd

import (
	"fmt"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
	"github.com/GrayCodeAI/hawk/onboarding"
	"github.com/GrayCodeAI/hawk/plugin"
	"github.com/spf13/cobra"
)

var (
	version                    string
	buildDate                  string
	model                      string
	provider                   string
	promptFlag                 string
	printMode                  bool
	versionFlag                bool
	outputFormat               string
	inputFormat                string
	noSessionPersistence       bool
	resumeID                   string
	continueFlag               bool
	forkSessionFlag            bool
	sessionIDFlag              string
	settingsFlag               string
	addDirs                    []string
	mcpServers                 []string
	toolsFlag                  []string
	toolsFlagSet               bool
	allowedToolsFlag           []string
	disallowedToolsFlag        []string
	permissionMode             string
	dangerouslySkipPermissions bool
	maxTurns                   int
	maxBudgetUSD               float64
	systemPromptFlag           string
	systemPromptFile           string
	appendSystemPromptFlag     string
	appendSystemPromptFile     string
)

// SetVersion sets the version string from main.
func SetVersion(v string) {
	version = v
}

// SetBuildDate sets the build date from main.
func SetBuildDate(d string) {
	buildDate = d
}

var rootCmd = &cobra.Command{
	Use:   "hawk [prompt]",
	Short: "AI coding agent powered by eyrie",
	Long:  "hawk is an AI coding agent that reads, writes, and runs code in your terminal.",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load saved API key from settings
		hawkconfig.LoadAPIKeyFromSettings()

		if versionFlag {
			if buildDate != "" && buildDate != "unknown" {
				cmd.Println(fmt.Sprintf("%s (Hawk) built %s", version, buildDate))
			} else {
				cmd.Println(fmt.Sprintf("%s (Hawk)", version))
			}
			return nil
		}
		if promptFlag == "" && len(args) > 0 {
			promptFlag = strings.Join(args, " ")
		}
		toolsFlagSet = cmd.Flags().Changed("tools")
		if err := validateRootFlags(); err != nil {
			return err
		}
		if printMode || promptFlag != "" || inputFormat == "stream-json" {
			if promptFlag == "" {
				stdinPrompt, err := readPromptFromStdin(inputFormat)
				if err != nil {
					return err
				}
				promptFlag = stdinPrompt
			}
			if promptFlag == "" {
				return fmt.Errorf("prompt required in print mode")
			}
			return runPrint(promptFlag)
		}

		// Show welcome
		onboarding.Welcome(version)

		// First-run setup if needed
		if onboarding.NeedsSetup() {
			if err := onboarding.RunSetup(); err != nil {
				return err
			}
		}

		// Launch TUI
		return runChat()
	},
}

func init() {
	rootCmd.Flags().StringVarP(&model, "model", "m", "", "model to use (e.g. claude-sonnet-4-20250514)")
	rootCmd.Flags().BoolVarP(&printMode, "print", "p", false, "print response and exit")
	rootCmd.Flags().StringVar(&promptFlag, "prompt", "", "send a single prompt and exit (legacy alias for --print)")
	rootCmd.Flags().StringVar(&outputFormat, "output-format", "text", `output format for --print: "text", "json", or "stream-json"`)
	rootCmd.Flags().StringVar(&inputFormat, "input-format", "text", `input format for --print: "text" or "stream-json"`)
	rootCmd.Flags().BoolVar(&noSessionPersistence, "no-session-persistence", false, "disable session persistence in print mode")
	rootCmd.Flags().StringVar(&provider, "provider", "", "LLM provider (anthropic, openai, gemini, etc.)")
	rootCmd.Flags().StringVarP(&resumeID, "resume", "r", "", "resume a saved session by ID")
	rootCmd.Flags().BoolVarP(&continueFlag, "continue", "c", false, "continue the most recent conversation in the current directory")
	rootCmd.Flags().BoolVar(&forkSessionFlag, "fork-session", false, "when resuming, create a new session ID instead of reusing the original")
	rootCmd.Flags().StringVar(&sessionIDFlag, "session-id", "", "use a specific session ID for the conversation")
	rootCmd.Flags().StringVar(&settingsFlag, "settings", "", "path to a settings JSON file or a JSON string to load for this session")
	rootCmd.Flags().StringArrayVar(&addDirs, "add-dir", nil, "additional directories to include in session context")
	rootCmd.Flags().StringArrayVar(&mcpServers, "mcp", nil, "MCP server command")
	rootCmd.Flags().StringArrayVar(&toolsFlag, "tools", nil, `available tools: "" disables all tools, "default" enables all, or names like "Bash,Edit,Read"`)
	rootCmd.Flags().StringArrayVar(&allowedToolsFlag, "allowedTools", nil, `comma or space-separated tool permission rules to allow (e.g. "Bash(git:*) Edit")`)
	rootCmd.Flags().StringArrayVar(&allowedToolsFlag, "allowed-tools", nil, `comma or space-separated tool permission rules to allow (e.g. "Bash(git:*) Edit")`)
	rootCmd.Flags().StringArrayVar(&disallowedToolsFlag, "disallowedTools", nil, `comma or space-separated tool permission rules to deny (e.g. "Bash(git:*) Edit")`)
	rootCmd.Flags().StringArrayVar(&disallowedToolsFlag, "disallowed-tools", nil, `comma or space-separated tool permission rules to deny (e.g. "Bash(git:*) Edit")`)
	rootCmd.Flags().StringVar(&permissionMode, "permission-mode", "", "permission mode: default, acceptEdits, bypassPermissions, dontAsk, or plan")
	rootCmd.Flags().BoolVar(&dangerouslySkipPermissions, "dangerously-skip-permissions", false, "bypass all permission checks")
	rootCmd.Flags().IntVar(&maxTurns, "max-turns", 0, "maximum number of agentic turns in non-interactive mode")
	rootCmd.Flags().Float64Var(&maxBudgetUSD, "max-budget-usd", 0, "maximum estimated API spend in USD")
	rootCmd.Flags().StringVar(&systemPromptFlag, "system-prompt", "", "system prompt to use for the session")
	rootCmd.Flags().StringVar(&systemPromptFile, "system-prompt-file", "", "read system prompt from a file")
	rootCmd.Flags().StringVar(&appendSystemPromptFlag, "append-system-prompt", "", "append text to the default or custom system prompt")
	rootCmd.Flags().StringVar(&appendSystemPromptFile, "append-system-prompt-file", "", "read text from a file and append it to the system prompt")
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "output the version number")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(toolsCmd)
	rootCmd.AddCommand(pluginCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print hawk version",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println("hawk", version)
	},
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run first-time setup again",
	RunE: func(cmd *cobra.Command, args []string) error {
		onboarding.Welcome(version)
		return onboarding.RunSetup()
	},
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run local diagnostics",
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := loadEffectiveSettings()
		if err != nil {
			return err
		}
		cmd.Println(doctorReport(settings))
		return nil
	},
}

var configCmd = &cobra.Command{
	Use:   "config [get <key>|set <key> <value>]",
	Short: "Show or update settings",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			switch args[0] {
			case "get":
				if len(args) != 2 {
					return fmt.Errorf("usage: hawk config get <key>")
				}
				settings, err := loadEffectiveSettings()
				if err != nil {
					return err
				}
				value, ok := hawkconfig.SettingValue(settings, args[1])
				if !ok {
					return fmt.Errorf("unsupported setting key %q", args[1])
				}
				cmd.Println(value)
				return nil
			case "set":
				if len(args) < 3 {
					return fmt.Errorf("usage: hawk config set <key> <value>")
				}
				if err := hawkconfig.SetGlobalSetting(args[1], strings.Join(args[2:], " ")); err != nil {
					return err
				}
				cmd.Println("updated", args[1])
				return nil
			default:
				return fmt.Errorf("unknown config action %q", args[0])
			}
		}
		settings, err := loadEffectiveSettings()
		if err != nil {
			return err
		}
		cmd.Println(settingsSummary(settings))
		return nil
	},
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Show MCP server configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := loadEffectiveSettings()
		if err != nil {
			return err
		}
		cmd.Println(mcpConfigSummary(settings))
		return nil
	},
}

var sessionsCmd = &cobra.Command{
	Use:     "sessions",
	Aliases: []string{"history"},
	Short:   "List saved sessions",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println(sessionsSummary())
	},
}

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List built-in tools",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println(builtInToolsSummary())
	},
}

var pluginCmd = &cobra.Command{
	Use:   "plugin [list|install <dir>|uninstall <name>]",
	Short: "Manage plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			cmd.Println(plugin.Summary())
			return nil
		}
		switch args[0] {
		case "list":
			cmd.Println(plugin.Summary())
			return nil
		case "install":
			if len(args) < 2 {
				return fmt.Errorf("usage: hawk plugin install <directory>")
			}
			if err := plugin.Install(args[1]); err != nil {
				return err
			}
			cmd.Println("installed", args[1])
			return nil
		case "uninstall":
			if len(args) < 2 {
				return fmt.Errorf("usage: hawk plugin uninstall <name>")
			}
			if err := plugin.Uninstall(args[1]); err != nil {
				return err
			}
			cmd.Println("uninstalled", args[1])
			return nil
		default:
			return fmt.Errorf("unknown plugin action %q", args[0])
		}
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
