package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/onboarding"
	"github.com/GrayCodeAI/hawk/internal/plugin"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/update"
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
	outputFields               string
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
	dangerouslySkipPermissions bool
	dryRunFlag                 bool
	maxTurns                   int
	maxBudgetUSD               float64
	systemPromptFlag           string
	systemPromptFile           string
	appendSystemPromptFlag     string
	appendSystemPromptFile     string
	sandboxFlag                string
	autoCommitFlag             bool
	watchFlag                  bool
	repoMapFlag                bool
	mapTokensFlag              int
	replFlag                   bool
	vibeMode                   bool
	powerLevel                 int
	timeout                    time.Duration
	councilMode                bool
	teachMode                  bool
	teachDepth                 int
	autoSkillFlag              bool
	recoverFlag                bool
	startupProfileFlag         bool
	preflightLiveFlag          bool
	quietFlag                  bool
)

var (
	recoverEnsureCatalogBeforeAgent = ensureCatalogBeforeAgent
	recoverRunChat                  = runChat
)

// SetVersion sets the version string from main.
func SetVersion(v string) {
	version = v
}

// SetBuildDate sets the build date from main.
func SetBuildDate(d string) {
	buildDate = d
}

func registeredProviderCount() int {
	return hawkconfig.RegisteredProviderCount()
}

var rootCmd = &cobra.Command{
	Use:   "hawk [prompt]",
	Short: "AI coding agent powered by eyrie",
	Long: fmt.Sprintf(`hawk is an AI coding agent that reads, writes, and runs code in your terminal.

It connects to %d first-class LLM providers through eyrie, executes tools (file I/O, shell,
git, web search), and manages sessions — all from a keyboard-driven TUI or
headless mode for scripts and CI.

Quick orientation:
  hawk                     Start interactive TUI
  hawk -p "prompt"         One-shot: send prompt, print response, exit
  hawk exec "task"         Autonomous multi-turn execution
  hawk path                Check environment readiness
  hawk doctor              Run diagnostics
  hawk config              Manage settings and credentials

API keys are stored in the OS keychain (macOS Keychain / Linux keyring).
Run hawk and use /config to set up your first provider.`, registeredProviderCount()),
	Example: `  hawk
  hawk -p "explain this repo"
  hawk exec "fix failing tests"
  hawk preflight
  hawk path`,
	Args:          cobra.ArbitraryArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if versionFlag {
			cmd.Println(versionLine())
			return nil
		}
		if promptFlag == "" && len(args) > 0 {
			promptFlag = strings.Join(args, " ")
		}
		toolsFlagSet = cmd.Flags().Changed("tools")
		if err := validateRootFlags(); err != nil {
			return err
		}
		if dangerouslySkipPermissions {
			if err := confirmDangerousSkipPermissions(); err != nil {
				return err
			}
		}

		if settings, err := loadEffectiveSettings(); err == nil {
			if !replFlag && settings.ReplMode != nil && *settings.ReplMode {
				replFlag = true
			}
			// Apply saved theme — mutates all global color vars immediately.
			if settings.Theme != "" {
				ApplyTheme(settings.Theme)
			}
		}

		if printMode || promptFlag != "" || inputFormat == "stream-json" || replFlag || watchFlag {
			// Credential migration is deferred until a path that actually
			// uses credentials: `hawk path`, `hawk version`, auto-skill and
			// other cold commands no longer construct the eyrie engine
			// (M17 — was ~1.8s on every root command).
			logMigrateProviderSecretsError(logger.Default(), hawkconfig.MigrateProviderSecrets())
			if promptFlag == "" && !replFlag && !watchFlag {
				stdinPrompt, err := readPromptFromStdin(inputFormat)
				if err != nil {
					return err
				}
				promptFlag = stdinPrompt
			}
			if promptFlag == "" && !replFlag && !watchFlag {
				return fmt.Errorf("prompt required in print mode")
			}
			if err := ensureCatalogBeforeAgent(context.Background(), true); err != nil {
				return err
			}
			// Folder trust check — non-interactive paths (print/repl/watch)
			// load the same project-scoped hooks, MCP servers, and plugins as
			// the TUI, so gate them identically: untrusted folders block
			// project automation.
			if tr := engine.ProjectTrust(""); tr.Blocked {
				return fmt.Errorf("cannot start: folder not trusted (%s)\nProject-scoped hooks, MCP servers, and custom specialists are blocked.\nRun 'hawk trust add' to trust this folder before running hawk", tr.Path)
			}
			if replFlag {
				return runRepl()
			}
			if watchFlag {
				return runWatch(promptFlag)
			}
			return runPrint(promptFlag)
		}

		// Auto-skill: analyze project and install matching skills.
		if autoSkillFlag {
			cwd, _ := os.Getwd()
			msg, _ := plugin.RunAutoSkill(cwd)
			if msg != "" {
				fmt.Println(msg)
			}
		}

		// Recovery: scan for interrupted sessions before launching TUI.
		if recoverFlag {
			candidates := session.ScanForRecovery()
			if len(candidates) > 0 {
				// Auto-resume the most recent interrupted session
				c := candidates[0]
				fmt.Printf("Found interrupted session %s (%s, %d msgs)\n", c.SessionID, c.Interruption, c.MessageCount)
				resumeID = c.SessionID
			}
		}

		if err := ensureCatalogBeforeAgent(context.Background(), false); err != nil {
			return err
		}

		// TUI path uses credentials — run the one-time hygiene pass here.
		logMigrateProviderSecretsError(logger.Default(), hawkconfig.MigrateProviderSecrets())

		// Folder trust check — block starting CLI in an untrusted directory
		if tr := engine.ProjectTrust(""); tr.Blocked {
			return fmt.Errorf("cannot start CLI: folder not trusted (%s)\nProject-scoped hooks, MCP servers, and custom specialists are blocked.\nRun 'hawk trust add' to trust this folder before starting hawk", tr.Path)
		}

		// Launch TUI — use /config to set API keys; eyrie supplies providers and models
		return runChat()
	},
}

func init() {
	rootCmd.Flags().StringVarP(&model, "model", "m", "", "model to use (from eyrie catalog; see /models)")
	rootCmd.Flags().BoolVarP(&printMode, "print", "p", false, "print response and exit")
	rootCmd.Flags().StringVar(&promptFlag, "prompt", "", "send a single prompt and exit (legacy alias for --print)")
	rootCmd.Flags().StringVar(&outputFormat, "output-format", "text", `output format for --print: "text", "json", or "stream-json"`)
	rootCmd.Flags().StringVar(&outputFields, "output-fields", "", `comma-separated field whitelist for --output-format json (e.g. "result,session_id")`)
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
	rootCmd.Flags().StringArrayVar(&allowedToolsFlag, "allowed-tools", nil, `comma or space-separated tool permission rules to allow (e.g. "Bash(git:*) Edit")`)
	rootCmd.Flags().StringArrayVar(&disallowedToolsFlag, "disallowed-tools", nil, `comma or space-separated tool permission rules to deny (e.g. "Bash(git:*) Edit")`)
	rootCmd.Flags().BoolVar(&dangerouslySkipPermissions, "dangerously-skip-permissions", false, "skip normal permission prompts (hooks, spec gates, sandbox, and dry-run still apply)")
	rootCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "deny every tool call unconditionally (preview only, nothing executes)")
	rootCmd.Flags().IntVar(&maxTurns, "max-turns", 0, "maximum number of agentic turns in non-interactive mode")
	rootCmd.Flags().Float64Var(&maxBudgetUSD, "max-budget-usd", 0, "maximum estimated API spend in USD")
	rootCmd.Flags().StringVar(&systemPromptFlag, "system-prompt", "", "system prompt to use for the session")
	rootCmd.Flags().StringVar(&systemPromptFile, "system-prompt-file", "", "read system prompt from a file")
	rootCmd.Flags().StringVar(&appendSystemPromptFlag, "append-system-prompt", "", "append text to the default or custom system prompt")
	rootCmd.Flags().StringVar(&appendSystemPromptFile, "append-system-prompt-file", "", "read text from a file and append it to the system prompt")
	rootCmd.Flags().StringVar(&sandboxFlag, "sandbox", "", "permission sandbox: strict, workspace, or off (same as /autonomy sandbox; not Docker container mode)")
	rootCmd.Flags().BoolVar(&autoCommitFlag, "auto-commit", false, "auto-commit file changes made by Write and Edit tools")
	rootCmd.Flags().BoolVar(&watchFlag, "watch", false, "watch the working directory for file changes and re-run on changes")
	rootCmd.Flags().BoolVar(&repoMapFlag, "repo-map", false, "inject an AST-ranked repository map (Aider-style) into the system prompt")
	rootCmd.Flags().IntVar(&mapTokensFlag, "map-tokens", 1024, "token budget for the --repo-map overview")
	rootCmd.Flags().BoolVar(&replFlag, "repl", false, "start interactive REPL mode (like aider) for multi-turn conversation without TUI")
	rootCmd.Flags().BoolVar(&vibeMode, "vibe", false, "vibe coding mode: auto-apply, auto-run, no confirmations")
	rootCmd.Flags().IntVar(&powerLevel, "power", 5, "power level 1-10 (auto-configures model, context, review depth)")
	rootCmd.Flags().DurationVar(&timeout, "timeout", 0, "time budget for the operation (e.g., 2m, 5m, 1h)")
	rootCmd.Flags().BoolVar(&councilMode, "council", false, "consult multiple models and synthesize best answer")
	rootCmd.Flags().BoolVar(&teachMode, "teach", false, "explain reasoning as the agent works")
	rootCmd.Flags().IntVar(&teachDepth, "teach-depth", 2, "explanation depth: 1=what, 2=why, 3=how")
	rootCmd.Flags().BoolVar(&autoSkillFlag, "auto-skill", false, "auto-detect project and install matching skills")
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "output the version number")
	rootCmd.Flags().BoolVar(&refreshCatalogFlag, "refresh-catalog", false, "refresh the eyrie model catalog before starting")
	rootCmd.Flags().BoolVar(&skipCatalogRefreshFlag, "no-auto-catalog-refresh", false, "disable automatic catalog refresh when cache is missing, empty, or stale")
	rootCmd.Flags().BoolVar(&recoverFlag, "recover", false, "scan for interrupted sessions and offer to resume")
	rootCmd.Flags().BoolVar(&startupProfileFlag, "startup-profile", false, "print startup performance profile")
	rootCmd.Flags().BoolVarP(&quietFlag, "quiet", "q", false, "suppress non-essential output (spinners, progress, decoration); machine-parseable output only")
	preflightCmd.Flags().BoolVar(&preflightLiveFlag, "live", false, "verify selected provider connectivity and authentication")
	preflightCmd.Flags().BoolVar(&preflightJSON, "json", false, "output preflight report as JSON")
	doctorCmd.Flags().BoolVar(&doctorJSONFlag, "json", false, "output diagnostics as JSON")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(preflightCmd)
	rootCmd.AddCommand(credentialsCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(toolsCmd)
	rootCmd.AddCommand(pluginCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(researchCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(fingerprintCmd)
	rootCmd.AddCommand(cmdHistoryCmd)
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(rulesCmd)
	rootCmd.AddCommand(sandboxCmd)
	rootCmd.AddCommand(costCmd)
	rootCmd.AddCommand(featuresCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(missionCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(snapshotCmd)
	rootCmd.AddCommand(evalCmd)
	rootCmd.AddCommand(harnessCmd)
	rootCmd.AddCommand(recoverCmd)
	rootCmd.AddCommand(manpageCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(bugReportCmd)
	completionCmd.AddCommand(completionInstallCmd)
}

// confirmDangerousSkipPermissions enforces a safety guard when
// --dangerously-skip-permissions is set. It skips normal permission prompts,
// but does not disable hooks, spec gates, sandbox enforcement, or dry-run.
// In a terminal, it requires typing the full confirmation token (not a single
// key) so a stray keystroke or terminal-escape trickery cannot confirm it. In
// non-interactive mode (CI, scripts), it requires the
// HAWK_DANGEROUSLY_SKIP_PERMISSIONS=1 environment variable.
func confirmDangerousSkipPermissions() error {
	if isStdinTerminal() {
		fmt.Fprintf(os.Stderr, "Type %s to confirm skipping permission prompts: ", dangerSkipConfirmToken)
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return fmt.Errorf("--dangerously-skip-permissions requires confirmation")
		}
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if !strings.EqualFold(answer, dangerSkipConfirmToken) {
			return fmt.Errorf("--dangerously-skip-permissions declined; aborting")
		}
		return nil
	}
	// Non-interactive: require explicit env var override.
	if os.Getenv("HAWK_DANGEROUSLY_SKIP_PERMISSIONS") != "1" {
		return fmt.Errorf("--dangerously-skip-permissions requires HAWK_DANGEROUSLY_SKIP_PERMISSIONS=1 in non-interactive mode")
	}
	return nil
}

// dangerSkipConfirmToken is the exact string a user must type to confirm
// --dangerously-skip-permissions. It is deliberately longer and distinct from
// the flag name so it cannot be triggered by a stray keystroke, shell
// autocomplete, or a single injected line — the user must understand and
// intentionally type the confirmation.
const dangerSkipConfirmToken = "i-understand-the-risks-skip-permissions"

// isStdinTerminal reports whether stdin is connected to a terminal.
// Delegates to the shared stdinIsTerminal so tests can override uniformly.
func isStdinTerminal() bool {
	return stdinIsTerminal()
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell|json]",
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

JSON:
  hawk completion json
  # Print a machine-readable command/flag spec for IDE integration.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell", "json"},
	Args:                  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			_ = cmd.Root().GenBashCompletion(cmd.OutOrStdout())
		case "zsh":
			_ = cmd.Root().GenZshCompletion(cmd.OutOrStdout())
		case "fish":
			_ = cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
		case "powershell":
			_ = cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		case "json":
			jsonStr, err := NewCompletionGenerator().GenerateJSON()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return
			}
			_, _ = cmd.OutOrStdout().Write([]byte(jsonStr))
			_, _ = cmd.OutOrStdout().Write([]byte("\n"))
		}
	},
}

var completionInstallCmd = &cobra.Command{
	Use:   "install [bash|zsh|fish]",
	Short: "Install shell completion script to the default location",
	Long: `Install the shell completion script to the standard location for your OS.

Bash:
  hawk completion install bash
  # Installs to ~/.local/share/bash-completion/completions/hawk (Linux)
  # or /opt/homebrew/etc/bash_completion.d/hawk (macOS Homebrew)

Zsh:
  hawk completion install zsh
  # Installs to the first directory in $fpath (e.g. /usr/local/share/zsh/site-functions/_hawk)

Fish:
  hawk completion install fish
  # Installs to ~/.config/fish/completions/hawk.fish`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish"},
	Args:                  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := args[0]
		path, err := InstallCompletion(shell)
		if err != nil {
			return err
		}

		// Generate the completion script.
		var script strings.Builder
		switch shell {
		case "bash":
			_ = cmd.Root().GenBashCompletion(&script)
		case "zsh":
			_ = cmd.Root().GenZshCompletion(&script)
		case "fish":
			_ = cmd.Root().GenFishCompletion(&script, true)
		}

		// Ensure parent directory exists.
		dir := path[:strings.LastIndex(path, "/")]
		if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- shell completion directory must be traversable
			return fmt.Errorf("cannot create directory %s: %w", dir, err)
		}

		if err := os.WriteFile(path, []byte(script.String()), 0o644); err != nil { // #nosec G306 -- completion scripts are intentionally user-readable
			return fmt.Errorf("cannot write completion script: %w", err)
		}

		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Installed %s completion to %s\n", shell, path); err != nil {
			return fmt.Errorf("cannot write completion message: %w", err)
		}
		return nil
	},
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for hawk updates",
	Long:  "Check GitHub for a newer hawk release and print upgrade instructions.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ver := version
		if ver == "" {
			ver = "dev"
		}
		cmd.Println(update.Summary(ver))
		return nil
	},
}

var bugReportCmd = &cobra.Command{
	Use:   "bug-report",
	Short: "Print a redacted diagnostic report for bug reports",
	Long: `Print a redacted environment report suitable for pasting into a GitHub issue.

Includes: version, platform, Go version, provider status, and doctor output.
API keys and secrets are never included.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var b strings.Builder
		b.WriteString("## hawk bug report\n\n")
		b.WriteString(fmt.Sprintf("- **Version:** %s\n", versionLine()))
		b.WriteString(fmt.Sprintf("- **Platform:** %s\n", update.Platform()))
		b.WriteString(fmt.Sprintf("- **Go:** %s\n", runtime.Version()))
		b.WriteString(fmt.Sprintf("- **OS/Arch:** %s/%s\n", runtime.GOOS, runtime.GOARCH))
		b.WriteString("\n## Doctor output\n\n```\n")
		settings := hawkconfig.LoadSettings()
		b.WriteString(doctorReport(settings))
		b.WriteString("\n```\n")
		cmd.Print(b.String())
		return nil
	},
}

// versionInfo is the machine-readable version output.
type versionInfo struct {
	Version   string `json:"version"`
	BuildDate string `json:"build_date,omitempty"`
}

var versionJSON bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print hawk version",
	Run: func(cmd *cobra.Command, args []string) {
		if versionJSON {
			info := versionInfo{Version: DisplayVersion()}
			if d := strings.TrimSpace(buildDate); d != "" && d != "unknown" {
				info.BuildDate = d
			}
			out, err := json.MarshalIndent(info, "", "  ")
			if err != nil {
				if _, ferr := fmt.Fprintf(cmd.ErrOrStderr(), "marshaling version: %v\n", err); ferr != nil {
					// Best effort, ignore error
				}
				return
			}
			cmd.Println(string(out))
			return
		}
		cmd.Println(versionLine())
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "output version as JSON")
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run first-time setup again",
	RunE: func(cmd *cobra.Command, args []string) error {
		onboarding.Welcome(version)
		return onboarding.RunSetup()
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive onboarding wizard for first-time setup",
	Long:  "Launch the interactive setup wizard to configure credentials, select providers/models, and initialize hawk.",
	RunE: func(cmd *cobra.Command, args []string) error {
		onboarding.Welcome(version)
		return onboarding.RunSetup()
	},
}

var doctorJSONFlag bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run local diagnostics",
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := loadEffectiveSettings()
		if err != nil {
			return err
		}
		if doctorJSONFlag {
			cmd.Println(doctorOutput(settings))
		} else {
			cmd.Println(doctorReport(settings))
		}
		return nil
	},
}

var preflightJSON bool

var preflightCmd = &cobra.Command{
	Use:   "preflight",
	Short: "Check local readiness; use --live to verify the selected provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := loadEffectiveSettings()
		if err != nil {
			return err
		}
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		if preflightLiveFlag {
			limit := timeout
			if limit <= 0 {
				limit = 15 * time.Second
			}
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, limit)
			defer cancel()
		}
		r := hawkconfig.EnginePreflightReportWithSettings(ctx, settings, hawkconfig.EnginePreflightOptions{VerifyLive: preflightLiveFlag})
		if preflightJSON {
			out, err := json.MarshalIndent(r, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling preflight: %w", err)
			}
			cmd.Println(string(out))
		} else {
			cmd.Println(hawkconfig.FormatEnginePreflight(r))
		}
		if !r.Ready {
			if preflightLiveFlag {
				return fmt.Errorf("live preflight failed — check the selected provider credential and network access")
			}
			return fmt.Errorf("preflight failed — run hawk and complete /config")
		}
		return nil
	},
}

var configCmd = &cobra.Command{
	Use:   "config [get|set|provider|model|keys|routing-preview|migrate-deployments]",
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
			case "routing-preview":
				if len(args) < 2 {
					return fmt.Errorf("usage: hawk config routing-preview <model>")
				}
				settings, err := loadEffectiveSettings()
				if err != nil {
					return err
				}
				out, err := hawkconfig.RoutingPreviewJSONWithSettings(cmd.Context(), settings, strings.Join(args[1:], " "))
				if err != nil {
					return err
				}
				cmd.Println(out)
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
	Short: "Show MCP configuration; run or register hawk as an MCP server",
	Long: "With no subcommand, summarizes the MCP servers hawk connects to (consumes).\n" +
		"  hawk mcp serve   — run hawk itself as an MCP server over stdio\n" +
		"  hawk mcp config  — print the JSON block to register hawk in Claude Desktop/Cursor/Windsurf",
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

var toolsJSON bool

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List built-in tools",
	Run: func(cmd *cobra.Command, args []string) {
		if toolsJSON {
			tools := allTools()
			type toolEntry struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			entries := make([]toolEntry, len(tools))
			for i, t := range tools {
				entries[i] = toolEntry{Name: t.Name(), Description: t.Description()}
			}
			data, _ := json.MarshalIndent(entries, "", "  ")
			cmd.Println(string(data))
			return
		}
		cmd.Println(builtInToolsSummary())
	},
}

func init() {
	toolsCmd.Flags().BoolVar(&toolsJSON, "json", false, "output tools as JSON")
}

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println(plugin.Summary())
		return nil
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

var recoverCmd = &cobra.Command{
	Use:   "recover [session-id]",
	Short: "Scan for interrupted sessions and resume",
	Long: `Scan for sessions that were interrupted (crash, terminal close, etc.)
and offer to resume them. If a session-id is provided, resume that specific session.

Examples:
  hawk recover              # List interrupted sessions
  hawk recover abc123       # Resume specific session
  hawk --recover            # Auto-resume most recent interrupted session`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			s, note, err := session.ResumeSession(args[0])
			if err != nil {
				return err
			}
			cmd.Println(note)
			cmd.Printf("Resuming session %s (%d messages, %s/%s)\n",
				s.ID, len(s.Messages), s.Provider, s.Model)
			return resumeRecoveredSession(context.Background(), s.ID)
		}

		// Scan and list
		candidates := session.ScanForRecovery()
		cmd.Println(session.FormatRecoveryCandidates(candidates))

		if len(candidates) > 0 {
			cmd.Println("Resume with: hawk recover <id>")
			cmd.Println("Or launch TUI with: hawk --recover")
		}
		return nil
	},
}

func resumeRecoveredSession(ctx context.Context, sessionID string) error {
	resumeID = sessionID
	continueFlag = false
	if err := recoverEnsureCatalogBeforeAgent(ctx, false); err != nil {
		return err
	}
	return recoverRunChat()
}

// logMigrateProviderSecretsError surfaces a non-nil error from
// hawkconfig.MigrateProviderSecrets via the structured logger.
//
// MigrateProviderSecrets is a one-time hygiene pass that strips API keys
// from the on-disk provider.json (a known-bad location — see AGENTS.md).
// If it fails, the keys remain in the file and the user must be told so
// they can run hawk /config to move them to the OS keychain. Previously
// the error was silently discarded (cmd/root.go:114), so a failure left
// the user with secrets in plaintext and no indication that anything was
// wrong.
//
// We log and continue rather than failing startup: the migration is
// best-effort, and a missing or unreadable provider.json is not
// fatal — the rest of the app can still function.
func logMigrateProviderSecretsError(l *logger.Logger, err error) {
	if err == nil {
		return
	}
	l.Warn(
		"provider secret migration failed; API keys may remain in provider.json. Run `hawk /config` to move them to the OS keychain.",
		map[string]interface{}{"err": err.Error()},
	)
}
