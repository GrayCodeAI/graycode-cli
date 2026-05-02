package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

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
	sandboxFlag                string
	autoCommitFlag             bool
	watchFlag                  bool
	vibeMode                   bool
	powerLevel                 int
	timeout                    time.Duration
	councilMode                bool
	teachMode                  bool
	teachDepth                 int
	autoSkillFlag              bool
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
		// Load persisted env vars (API keys from ~/.hawk/env)
		hawkconfig.LoadEnvFile()

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

		// First-run setup if needed
		if onboarding.NeedsSetup() {
			onboarding.Welcome(version)
			if err := onboarding.RunSetup(); err != nil {
				return err
			}
		}

		// Auto-skill: analyze project and install matching skills.
		if autoSkillFlag {
			cwd, _ := os.Getwd()
			msg, _ := plugin.RunAutoSkill(cwd)
			if msg != "" {
				fmt.Println(msg)
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
	rootCmd.Flags().StringVar(&sandboxFlag, "sandbox", "", "sandbox mode for Bash commands: strict, workspace, or off")
	rootCmd.Flags().BoolVar(&autoCommitFlag, "auto-commit", false, "auto-commit file changes made by Write and Edit tools")
	rootCmd.Flags().BoolVar(&watchFlag, "watch", false, "watch the working directory for file changes")
	rootCmd.Flags().BoolVar(&vibeMode, "vibe", false, "vibe coding mode: auto-apply, auto-run, no confirmations")
	rootCmd.Flags().IntVar(&powerLevel, "power", 5, "power level 1-10 (auto-configures model, context, review depth)")
	rootCmd.Flags().DurationVar(&timeout, "timeout", 0, "time budget for the operation (e.g., 2m, 5m, 1h)")
	rootCmd.Flags().BoolVar(&councilMode, "council", false, "consult multiple models and synthesize best answer")
	rootCmd.Flags().BoolVar(&teachMode, "teach", false, "explain reasoning as the agent works")
	rootCmd.Flags().IntVar(&teachDepth, "teach-depth", 2, "explanation depth: 1=what, 2=why, 3=how")
	rootCmd.Flags().BoolVar(&autoSkillFlag, "auto-skill", false, "auto-detect project and install matching skills")
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "output the version number")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(toolsCmd)
	rootCmd.AddCommand(pluginCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(researchCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(sightCmd)
	rootCmd.AddCommand(fingerprintCmd)
	rootCmd.AddCommand(cmdHistoryCmd)
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(rulesCmd)
	rootCmd.AddCommand(sandboxCmd)
	rootCmd.AddCommand(costCmd)
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `To load completions:

Bash:
  source <(hawk completion bash)
  # To load completions for each session, execute once:
  # Linux:
  hawk completion bash > /etc/bash_completion.d/hawk
  # macOS:
  hawk completion bash > /usr/local/etc/bash_completion.d/hawk

Zsh:
  source <(hawk completion zsh)
  # To load completions for each session, execute once:
  hawk completion zsh > "${fpath[1]}/_hawk"

Fish:
  hawk completion fish | source
  # To load completions for each session, execute once:
  hawk completion fish > ~/.config/fish/completions/hawk.fish

PowerShell:
  hawk completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, run:
  hawk completion powershell > hawk.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			cmd.Root().GenBashCompletion(cmd.OutOrStdout())
		case "zsh":
			cmd.Root().GenZshCompletion(cmd.OutOrStdout())
		case "fish":
			cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
		case "powershell":
			cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		}
	},
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
	Use:   "config [provider <name>|model <name>|get <key>|set <key> <value>]",
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
			case "provider":
				if len(args) < 2 {
					return fmt.Errorf("usage: hawk config provider <name>")
				}
				if err := hawkconfig.SetGlobalSetting("provider", strings.Join(args[1:], " ")); err != nil {
					return err
				}
				cmd.Println("updated provider")
				return nil
			case "model":
				if len(args) < 2 {
					return fmt.Errorf("usage: hawk config model <name>")
				}
				if err := hawkconfig.SetGlobalSetting("model", strings.Join(args[1:], " ")); err != nil {
					return err
				}
				cmd.Println("updated model")
				return nil
			case "keys":
				cmd.Println(apiKeyConfigSummary())
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
	Use:   "sessions",
	Short: "List saved sessions",
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

var (
	researchGrep      string
	researchDirection string
	researchBudgetMin int
	researchBranch    string
	researchResults   string
)

var researchCmd = &cobra.Command{
	Use:   "research [flags] <metric-command>",
	Short: "Autonomous research loop (Karpathy autoresearch pattern)",
	Long:  "hawk research --grep '^val_bpb:' --direction lower 'uv run train.py'",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("metric command is required")
		}
		cfg := ResearchConfig{
			MetricCmd:    strings.Join(args, " "),
			MetricGrep:   researchGrep,
			Direction:    researchDirection,
			Budget:       researchBudgetMin,
			BranchPrefix: researchBranch,
			ResultsFile:  researchResults,
		}
		return runPrint(BuildResearchPrompt(cfg))
	},
}

func init() {
	researchCmd.Flags().StringVar(&researchGrep, "grep", "", "grep pattern to extract metric from run.log")
	researchCmd.Flags().StringVar(&researchDirection, "direction", "lower", "optimization direction: lower or higher")
	researchCmd.Flags().IntVar(&researchBudgetMin, "budget", 5, "time budget per experiment in minutes")
	researchCmd.Flags().StringVar(&researchBranch, "branch", "autoresearch", "git branch prefix")
	researchCmd.Flags().StringVar(&researchResults, "results", "results.tsv", "results TSV file path")
}

var (
	contextFocus  string
	contextOutput string
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Export project context as a single document for use in any LLM",
	RunE: func(cmd *cobra.Command, args []string) error {
		if contextOutput != "" {
			if err := ExportContextToFile("", contextFocus, contextOutput); err != nil {
				return err
			}
			cmd.Println("Context exported to", contextOutput)
			return nil
		}
		result, err := ExportContext("", contextFocus)
		if err != nil {
			return err
		}
		cmd.Print(result)
		return nil
	},
}

func init() {
	contextCmd.Flags().StringVar(&contextFocus, "focus", "", "focus on a specific area (e.g., 'engine', 'auth')")
	contextCmd.Flags().StringVarP(&contextOutput, "output", "o", "", "write context to a file instead of stdout")
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
