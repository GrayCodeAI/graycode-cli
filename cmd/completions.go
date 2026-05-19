package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/provider/routing"
)

// FlagInfo describes a CLI flag for completion generation.
type FlagInfo struct {
	Name        string   `json:"name"`
	Short       string   `json:"short,omitempty"`
	Description string   `json:"description"`
	Type        string   `json:"type"` // "string", "bool", "int"
	Choices     []string `json:"choices,omitempty"`
}

// CommandInfo describes a CLI command for completion generation.
type CommandInfo struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Subcommands []CommandInfo `json:"subcommands,omitempty"`
	Flags       []FlagInfo    `json:"flags,omitempty"`
}

// CompletionGenerator generates shell completion scripts for hawk.
type CompletionGenerator struct {
	Commands      []CommandInfo `json:"commands"`
	Flags         []FlagInfo    `json:"flags"`
	SlashCommands []string      `json:"slash_commands"`
	Models        []string      `json:"models"`
	Providers     []string      `json:"providers"`
}

// NewCompletionGenerator creates a CompletionGenerator pre-populated with hawk's
// command structure, flags, providers, models, and slash commands.
func NewCompletionGenerator() *CompletionGenerator {
	g := &CompletionGenerator{}
	g.populateCommands()
	g.populateFlags()
	g.populateProviders()
	g.populateModels()
	g.populateSlashCommands()
	return g
}

func (g *CompletionGenerator) populateCommands() {
	g.Commands = []CommandInfo{
		{
			Name:        "exec",
			Description: "Execute a single command non-interactively",
			Flags: []FlagInfo{
				{Name: "output-format", Short: "o", Description: "Output format: text or json", Type: "string", Choices: []string{"text", "json"}},
				{Name: "auto", Description: "Autonomy level", Type: "string", Choices: []string{"supervised", "basic", "semi", "full", "yolo"}},
				{Name: "model", Short: "m", Description: "Model ID to use", Type: "string"},
				{Name: "max-turns", Description: "Maximum agentic turns", Type: "int"},
				{Name: "cwd", Description: "Working directory", Type: "string"},
				{Name: "agent", Description: "Agent persona to use", Type: "string"},
				{Name: "session-id", Short: "s", Description: "Continue an existing session", Type: "string"},
			},
		},
		{
			Name:        "daemon",
			Description: "Start the hawk background daemon",
		},
		{
			Name:        "mission",
			Description: "Multi-agent orchestration on parallel git branches",
		},
		{
			Name:        "search",
			Description: "Search code using semantic and structural queries",
		},
		{
			Name:        "agent",
			Description: "Manage agent personas",
		},
		{
			Name:        "doctor",
			Description: "Run local diagnostics",
		},
		{
			Name:        "config",
			Description: "Show or update settings",
			Subcommands: []CommandInfo{
				{Name: "get", Description: "Get a setting value"},
				{Name: "set", Description: "Set a setting value"},
				{Name: "provider", Description: "Set the LLM provider"},
				{Name: "model", Description: "Set the model"},
				{Name: "keys", Description: "Show API key configuration"},
			},
		},
		{
			Name:        "sessions",
			Description: "List saved sessions",
		},
		{
			Name:        "tools",
			Description: "List built-in tools",
		},
		{
			Name:        "skills",
			Description: "Manage skills",
			Subcommands: []CommandInfo{
				{Name: "list", Description: "List installed skills"},
				{Name: "search", Description: "Search the community skill registry"},
				{Name: "install", Description: "Install a skill"},
				{Name: "remove", Description: "Remove a skill"},
				{Name: "info", Description: "Show skill details"},
				{Name: "trending", Description: "Show trending skills"},
				{Name: "audit", Description: "Audit installed skills"},
			},
		},
		{
			Name:        "completion",
			Description: "Generate shell completion script",
			Subcommands: []CommandInfo{
				{Name: "bash", Description: "Generate bash completion"},
				{Name: "zsh", Description: "Generate zsh completion"},
				{Name: "fish", Description: "Generate fish completion"},
				{Name: "powershell", Description: "Generate PowerShell completion"},
			},
		},
		{
			Name:        "research",
			Description: "Autonomous research loop",
			Flags: []FlagInfo{
				{Name: "grep", Description: "Grep pattern to extract metric", Type: "string"},
				{Name: "direction", Description: "Optimization direction", Type: "string", Choices: []string{"lower", "higher"}},
				{Name: "budget", Description: "Time budget per experiment in minutes", Type: "int"},
				{Name: "branch", Description: "Git branch prefix", Type: "string"},
				{Name: "results", Description: "Results TSV file path", Type: "string"},
			},
		},
		{
			Name:        "context",
			Description: "Export project context as a single document",
			Flags: []FlagInfo{
				{Name: "focus", Description: "Focus on a specific area", Type: "string"},
				{Name: "output", Short: "o", Description: "Write context to a file", Type: "string"},
			},
		},
		{
			Name:        "version",
			Description: "Print hawk version",
		},
		{
			Name:        "setup",
			Description: "Run first-time setup again",
		},
		{
			Name:        "plugin",
			Description: "Manage plugins",
			Subcommands: []CommandInfo{
				{Name: "list", Description: "List installed plugins"},
				{Name: "install", Description: "Install a plugin"},
				{Name: "uninstall", Description: "Uninstall a plugin"},
			},
		},
		{
			Name:        "mcp",
			Description: "Show MCP server configuration",
		},
		{
			Name:        "inspect",
			Description: "Inspect session or context state",
		},
		{
			Name:        "plan",
			Description: "Enter plan mode (read-only analysis)",
		},
		{
			Name:        "rules",
			Description: "Manage project rules",
		},
		{
			Name:        "sandbox",
			Description: "Sandbox configuration",
		},
		{
			Name:        "cost",
			Description: "Show token usage and cost summary",
		},
		{
			Name:        "snapshot",
			Description: "Manage session snapshots",
		},
		{
			Name:        "sight",
			Description: "Visual analysis tools",
		},
		{
			Name:        "fingerprint",
			Description: "Show project fingerprint",
		},
	}
}

func (g *CompletionGenerator) populateFlags() {
	g.Flags = []FlagInfo{
		{Name: "provider", Description: "LLM provider", Type: "string", Choices: nil}, // choices filled from Providers
		{Name: "model", Short: "m", Description: "Model to use", Type: "string"},
		{Name: "print", Short: "p", Description: "Print response and exit", Type: "bool"},
		{Name: "resume", Short: "r", Description: "Resume a saved session by ID", Type: "string"},
		{Name: "continue", Short: "c", Description: "Continue the most recent conversation", Type: "bool"},
		{Name: "mcp", Description: "MCP server command", Type: "string"},
		{Name: "allowed-tools", Description: "Tool permission rules to allow", Type: "string"},
		{Name: "disallowed-tools", Description: "Tool permission rules to deny", Type: "string"},
		{Name: "permission-mode", Description: "Permission mode", Type: "string", Choices: []string{"default", "acceptEdits", "bypassPermissions", "dontAsk", "plan"}},
		{Name: "dangerously-skip-permissions", Description: "Bypass all permission checks", Type: "bool"},
		{Name: "max-turns", Description: "Maximum number of agentic turns", Type: "int"},
		{Name: "max-budget-usd", Description: "Maximum estimated API spend in USD", Type: "string"},
		{Name: "system-prompt", Description: "System prompt to use", Type: "string"},
		{Name: "system-prompt-file", Description: "Read system prompt from a file", Type: "string"},
		{Name: "append-system-prompt", Description: "Append text to system prompt", Type: "string"},
		{Name: "append-system-prompt-file", Description: "Read text from a file and append it to the system prompt", Type: "string"},
		{Name: "output-format", Description: "Output format for --print", Type: "string", Choices: []string{"text", "json", "stream-json"}},
		{Name: "input-format", Description: "Input format for --print", Type: "string", Choices: []string{"text", "stream-json"}},
		{Name: "no-session-persistence", Description: "Disable session persistence in print mode", Type: "bool"},
		{Name: "session-id", Description: "Use a specific session ID", Type: "string"},
		{Name: "settings", Description: "Path to a settings JSON file", Type: "string"},
		{Name: "add-dir", Description: "Additional directories to include", Type: "string"},
		{Name: "tools", Description: "Available tools configuration", Type: "string"},
		{Name: "sandbox", Description: "Sandbox mode for Bash commands", Type: "string", Choices: []string{"strict", "workspace", "off"}},
		{Name: "auto-commit", Description: "Auto-commit file changes", Type: "bool"},
		{Name: "watch", Description: "Watch working directory for file changes", Type: "bool"},
		{Name: "vibe", Description: "Vibe coding mode", Type: "bool"},
		{Name: "power", Description: "Power level 1-10", Type: "int"},
		{Name: "timeout", Description: "Time budget for the operation", Type: "string"},
		{Name: "council", Description: "Consult multiple models", Type: "bool"},
		{Name: "teach", Description: "Explain reasoning as the agent works", Type: "bool"},
		{Name: "teach-depth", Description: "Explanation depth: 1=what, 2=why, 3=how", Type: "int"},
		{Name: "auto-skill", Description: "Auto-detect project and install matching skills", Type: "bool"},
		{Name: "container", Description: "Force container mode", Type: "bool"},
		{Name: "no-container", Description: "Disable container mode", Type: "bool"},
		{Name: "version", Short: "v", Description: "Output the version number", Type: "bool"},
		{Name: "fork-session", Description: "Create a new session ID when resuming", Type: "bool"},
	}
}

func (g *CompletionGenerator) populateProviders() {
	g.Providers = []string{
		"anthropic",
		"openai",
		"gemini",
		"openrouter",
		"grok",
		"groq",
		"deepseek",
		"mistral",
		"bedrock",
		"vertex",
		"ollama",
	}
}

func (g *CompletionGenerator) populateModels() {
	g.Models = routing.AllCatalogModelNames()
}

func (g *CompletionGenerator) populateSlashCommands() {
	g.SlashCommands = []string{
		"/add", "/add-dir", "/agents", "/agents-init", "/audit", "/branch", "/branches",
		"/bughunter", "/btw", "/check", "/clean", "/clear", "/color", "/commit", "/compact",
		"/compress", "/config", "/context", "/copy", "/cost", "/council", "/cron",
		"/design", "/diff", "/doctor", "/drop", "/effort", "/env", "/exit", "/explain",
		"/export", "/fast", "/files", "/focus", "/fork", "/help", "/history", "/hooks",
		"/hunt", "/init", "/integrity", "/keybindings", "/learn", "/lint", "/loop",
		"/mcp", "/memory", "/metrics", "/model", "/new", "/output-style",
		"/permissions", "/pin", "/plan", "/plugin", "/plugins", "/power",
		"/pr-comments", "/provider-status", "/quit", "/refresh-model-catalog",
		"/release-notes", "/reload-plugins", "/remote-env", "/rename", "/render",
		"/research", "/resume", "/retry", "/review", "/rewind", "/run",
		"/sandbox", "/search", "/security-review", "/session", "/share", "/skills",
		"/snapshot", "/stats", "/status", "/statusline", "/summary", "/tag", "/tasks",
		"/test", "/theme", "/think", "/think-back", "/thinkback", "/thinkback-play",
		"/tokens", "/tools", "/undo", "/upgrade", "/usage", "/version", "/vibe",
		"/vim", "/voice", "/welcome", "/yolo",
	}
}

// GenerateBash returns a complete bash completion script for hawk.
func (g *CompletionGenerator) GenerateBash() string {
	var b strings.Builder

	b.WriteString("# bash completion for hawk\n")
	b.WriteString("# Auto-generated by hawk completions generator\n\n")

	// Build subcommand list
	subcommands := make([]string, 0, len(g.Commands))
	for _, cmd := range g.Commands {
		subcommands = append(subcommands, cmd.Name)
	}

	// Build flag list
	flags := make([]string, 0, len(g.Flags))
	for _, f := range g.Flags {
		flags = append(flags, "--"+f.Name)
		if f.Short != "" {
			flags = append(flags, "-"+f.Short)
		}
	}

	// Build provider list
	providers := strings.Join(g.Providers, " ")

	// Build permission modes
	var permModes string
	for _, f := range g.Flags {
		if f.Name == "permission-mode" && len(f.Choices) > 0 {
			permModes = strings.Join(f.Choices, " ")
			break
		}
	}

	// Build slash commands list
	slashCmds := strings.Join(g.SlashCommands, " ")

	b.WriteString("_hawk_completions() {\n")
	b.WriteString("    local cur prev words cword\n")
	b.WriteString("    _init_completion || return\n")
	b.WriteString("\n")
	b.WriteString("    cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	b.WriteString("    prev=\"${COMP_WORDS[COMP_CWORD-1]}\"\n")
	b.WriteString("\n")
	b.WriteString("    # Complete providers after --provider\n")
	b.WriteString("    case \"$prev\" in\n")
	b.WriteString("        --provider)\n")
	b.WriteString(fmt.Sprintf("            COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n", providers))
	b.WriteString("            return 0\n")
	b.WriteString("            ;;\n")
	b.WriteString("        --permission-mode)\n")
	b.WriteString(fmt.Sprintf("            COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n", permModes))
	b.WriteString("            return 0\n")
	b.WriteString("            ;;\n")
	b.WriteString("        --model|-m)\n")
	b.WriteString(fmt.Sprintf("            COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n", strings.Join(g.Models, " ")))
	b.WriteString("            return 0\n")
	b.WriteString("            ;;\n")
	b.WriteString("        --sandbox)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"strict workspace off\" -- \"$cur\"))\n")
	b.WriteString("            return 0\n")
	b.WriteString("            ;;\n")
	b.WriteString("        --output-format|-o)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"text json stream-json\" -- \"$cur\"))\n")
	b.WriteString("            return 0\n")
	b.WriteString("            ;;\n")
	b.WriteString("        --input-format)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"text stream-json\" -- \"$cur\"))\n")
	b.WriteString("            return 0\n")
	b.WriteString("            ;;\n")
	b.WriteString("        --system-prompt-file|--append-system-prompt-file|--settings)\n")
	b.WriteString("            COMPREPLY=($(compgen -f -- \"$cur\"))\n")
	b.WriteString("            return 0\n")
	b.WriteString("            ;;\n")
	b.WriteString("        --add-dir|--cwd)\n")
	b.WriteString("            COMPREPLY=($(compgen -d -- \"$cur\"))\n")
	b.WriteString("            return 0\n")
	b.WriteString("            ;;\n")
	b.WriteString("    esac\n")
	b.WriteString("\n")
	b.WriteString("    # Complete slash commands when input starts with /\n")
	b.WriteString("    if [[ \"$cur\" == /* ]]; then\n")
	b.WriteString(fmt.Sprintf("        COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n", slashCmds))
	b.WriteString("        return 0\n")
	b.WriteString("    fi\n")
	b.WriteString("\n")
	b.WriteString("    # Complete flags when input starts with -\n")
	b.WriteString("    if [[ \"$cur\" == -* ]]; then\n")
	b.WriteString(fmt.Sprintf("        COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n", strings.Join(flags, " ")))
	b.WriteString("        return 0\n")
	b.WriteString("    fi\n")
	b.WriteString("\n")
	b.WriteString("    # Complete subcommands after hawk\n")
	b.WriteString("    if [[ ${COMP_CWORD} -eq 1 ]]; then\n")
	b.WriteString(fmt.Sprintf("        COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n", strings.Join(subcommands, " ")))
	b.WriteString("        return 0\n")
	b.WriteString("    fi\n")
	b.WriteString("\n")

	// Subcommand-specific completions
	b.WriteString("    # Subcommand-specific completions\n")
	b.WriteString("    local subcmd=\"${COMP_WORDS[1]}\"\n")
	b.WriteString("    case \"$subcmd\" in\n")
	for _, cmd := range g.Commands {
		if len(cmd.Subcommands) > 0 {
			subs := make([]string, 0, len(cmd.Subcommands))
			for _, sc := range cmd.Subcommands {
				subs = append(subs, sc.Name)
			}
			b.WriteString(fmt.Sprintf("        %s)\n", cmd.Name))
			b.WriteString(fmt.Sprintf("            COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n", strings.Join(subs, " ")))
			b.WriteString("            return 0\n")
			b.WriteString("            ;;\n")
		}
	}
	b.WriteString("    esac\n")
	b.WriteString("\n")
	b.WriteString("    return 0\n")
	b.WriteString("}\n")
	b.WriteString("\n")
	b.WriteString("complete -F _hawk_completions hawk\n")

	return b.String()
}

// GenerateZsh returns a complete zsh completion script for hawk.
func (g *CompletionGenerator) GenerateZsh() string {
	var b strings.Builder

	b.WriteString("#compdef hawk\n")
	b.WriteString("# zsh completion for hawk\n")
	b.WriteString("# Auto-generated by hawk completions generator\n\n")

	b.WriteString("_hawk() {\n")
	b.WriteString("    local -a commands\n")
	b.WriteString("    local -a global_flags\n")
	b.WriteString("\n")

	// Commands array
	b.WriteString("    commands=(\n")
	for _, cmd := range g.Commands {
		b.WriteString(fmt.Sprintf("        '%s:%s'\n", cmd.Name, escapeZsh(cmd.Description)))
	}
	b.WriteString("    )\n\n")

	// Global flags via _arguments
	b.WriteString("    _arguments -C \\\n")
	for i, f := range g.Flags {
		suffix := " \\"
		if i == len(g.Flags)-1 {
			suffix = ""
		}
		if f.Type == "bool" {
			if f.Short != "" {
				b.WriteString(fmt.Sprintf("        '(-%s --%s)'{-%s,--%s}'[%s]'%s\n", f.Short, f.Name, f.Short, f.Name, escapeZsh(f.Description), suffix))
			} else {
				b.WriteString(fmt.Sprintf("        '--%s[%s]'%s\n", f.Name, escapeZsh(f.Description), suffix))
			}
		} else {
			choices := ""
			if len(f.Choices) > 0 {
				choices = "(" + strings.Join(f.Choices, " ") + ")"
			}
			if f.Short != "" {
				b.WriteString(fmt.Sprintf("        '(-%s --%s)'{-%s,--%s}'[%s]:%s:%s'%s\n", f.Short, f.Name, f.Short, f.Name, escapeZsh(f.Description), f.Name, choices, suffix))
			} else {
				b.WriteString(fmt.Sprintf("        '--%s[%s]:%s:%s'%s\n", f.Name, escapeZsh(f.Description), f.Name, choices, suffix))
			}
		}
	}
	b.WriteString("        '1:command:->commands' \\\n")
	b.WriteString("        '*::arg:->args'\n")
	b.WriteString("\n")

	b.WriteString("    case $state in\n")
	b.WriteString("        commands)\n")
	b.WriteString("            _describe -t commands 'hawk commands' commands\n")
	b.WriteString("            ;;\n")
	b.WriteString("        args)\n")
	b.WriteString("            case $words[1] in\n")
	for _, cmd := range g.Commands {
		if len(cmd.Subcommands) > 0 {
			b.WriteString(fmt.Sprintf("                %s)\n", cmd.Name))
			b.WriteString("                    local -a subcmds\n")
			b.WriteString("                    subcmds=(\n")
			for _, sc := range cmd.Subcommands {
				b.WriteString(fmt.Sprintf("                        '%s:%s'\n", sc.Name, escapeZsh(sc.Description)))
			}
			b.WriteString("                    )\n")
			b.WriteString("                    _describe -t commands 'subcommands' subcmds\n")
			b.WriteString("                    ;;\n")
		}
	}
	b.WriteString("            esac\n")
	b.WriteString("            ;;\n")
	b.WriteString("    esac\n")
	b.WriteString("}\n\n")

	// Slash commands completion function
	b.WriteString("_hawk_slash_commands() {\n")
	b.WriteString("    local -a slash_commands\n")
	b.WriteString("    slash_commands=(\n")
	for _, sc := range g.SlashCommands {
		b.WriteString(fmt.Sprintf("        '%s'\n", sc))
	}
	b.WriteString("    )\n")
	b.WriteString("    compadd -a slash_commands\n")
	b.WriteString("}\n\n")

	// Provider completion function
	b.WriteString("_hawk_providers() {\n")
	b.WriteString("    local -a providers\n")
	b.WriteString("    providers=(\n")
	for _, p := range g.Providers {
		b.WriteString(fmt.Sprintf("        '%s'\n", p))
	}
	b.WriteString("    )\n")
	b.WriteString("    compadd -a providers\n")
	b.WriteString("}\n\n")

	b.WriteString("_hawk \"$@\"\n")

	return b.String()
}

// GenerateFish returns a complete fish shell completion script for hawk.
func (g *CompletionGenerator) GenerateFish() string {
	var b strings.Builder

	b.WriteString("# fish completion for hawk\n")
	b.WriteString("# Auto-generated by hawk completions generator\n\n")

	// Disable file completions by default
	b.WriteString("complete -c hawk -f\n\n")

	// Subcommands
	b.WriteString("# Subcommands\n")
	for _, cmd := range g.Commands {
		b.WriteString(fmt.Sprintf("complete -c hawk -n '__fish_use_subcommand' -a '%s' -d '%s'\n",
			cmd.Name, escapeFish(cmd.Description)))
	}
	b.WriteString("\n")

	// Subcommand-specific completions
	for _, cmd := range g.Commands {
		if len(cmd.Subcommands) > 0 {
			b.WriteString(fmt.Sprintf("# %s subcommands\n", cmd.Name))
			for _, sc := range cmd.Subcommands {
				b.WriteString(fmt.Sprintf("complete -c hawk -n '__fish_seen_subcommand_from %s' -a '%s' -d '%s'\n",
					cmd.Name, sc.Name, escapeFish(sc.Description)))
			}
			b.WriteString("\n")
		}
		// Command-specific flags
		if len(cmd.Flags) > 0 {
			b.WriteString(fmt.Sprintf("# %s flags\n", cmd.Name))
			for _, f := range cmd.Flags {
				if f.Short != "" {
					b.WriteString(fmt.Sprintf("complete -c hawk -n '__fish_seen_subcommand_from %s' -l '%s' -s '%s' -d '%s'",
						cmd.Name, f.Name, f.Short, escapeFish(f.Description)))
				} else {
					b.WriteString(fmt.Sprintf("complete -c hawk -n '__fish_seen_subcommand_from %s' -l '%s' -d '%s'",
						cmd.Name, f.Name, escapeFish(f.Description)))
				}
				if f.Type == "bool" {
					// no argument required
				} else {
					b.WriteString(" -r")
					if len(f.Choices) > 0 {
						b.WriteString(fmt.Sprintf(" -a '%s'", strings.Join(f.Choices, " ")))
					}
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	// Global flags
	b.WriteString("# Global flags\n")
	for _, f := range g.Flags {
		if f.Short != "" {
			b.WriteString(fmt.Sprintf("complete -c hawk -l '%s' -s '%s' -d '%s'",
				f.Name, f.Short, escapeFish(f.Description)))
		} else {
			b.WriteString(fmt.Sprintf("complete -c hawk -l '%s' -d '%s'",
				f.Name, escapeFish(f.Description)))
		}
		if f.Type == "bool" {
			// no argument required
		} else {
			b.WriteString(" -r")
			if len(f.Choices) > 0 {
				b.WriteString(fmt.Sprintf(" -a '%s'", strings.Join(f.Choices, " ")))
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Provider completions for --provider
	b.WriteString("# Provider completions\n")
	b.WriteString(fmt.Sprintf("complete -c hawk -l 'provider' -r -a '%s' -d 'LLM provider'\n",
		strings.Join(g.Providers, " ")))
	b.WriteString("\n")

	// Slash commands
	b.WriteString("# Slash commands (for interactive mode reference)\n")
	for _, sc := range g.SlashCommands {
		b.WriteString(fmt.Sprintf("complete -c hawk -a '%s' -d 'Slash command'\n", sc))
	}

	return b.String()
}

// GenerateJSON returns a machine-readable JSON completion spec for IDE integration.
func (g *CompletionGenerator) GenerateJSON() string {
	spec := struct {
		Name          string        `json:"name"`
		Version       string        `json:"version"`
		Commands      []CommandInfo `json:"commands"`
		GlobalFlags   []FlagInfo    `json:"global_flags"`
		SlashCommands []string      `json:"slash_commands"`
		Providers     []string      `json:"providers"`
		Models        []string      `json:"models"`
	}{
		Name:          "hawk",
		Version:       "1.0.0",
		Commands:      g.Commands,
		GlobalFlags:   g.Flags,
		SlashCommands: g.SlashCommands,
		Providers:     g.Providers,
		Models:        g.Models,
	}

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

// InstallCompletion returns the filesystem path where the completion script
// should be installed for the given shell. It does not write the file; the
// caller should decide whether to proceed.
func InstallCompletion(shell string) (string, error) {
	switch shell {
	case "bash":
		return bashInstallPath(), nil
	case "zsh":
		return zshInstallPath(), nil
	case "fish":
		return fishInstallPath(), nil
	default:
		return "", fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish)", shell)
	}
}

func bashInstallPath() string {
	// Prefer user-local path
	home, err := os.UserHomeDir()
	if err != nil {
		return "/etc/bash_completion.d/hawk"
	}
	localDir := filepath.Join(home, ".local", "share", "bash-completion", "completions")
	if info, err := os.Stat(localDir); err == nil && info.IsDir() {
		return filepath.Join(localDir, "hawk")
	}
	// On macOS, use homebrew path if available
	if runtime.GOOS == "darwin" {
		brewPath := "/usr/local/etc/bash_completion.d/hawk"
		return brewPath
	}
	// Fallback: try system-wide
	sysDir := "/etc/bash_completion.d"
	if info, err := os.Stat(sysDir); err == nil && info.IsDir() {
		return filepath.Join(sysDir, "hawk")
	}
	// Final fallback: user-local
	return filepath.Join(home, ".local", "share", "bash-completion", "completions", "hawk")
}

func zshInstallPath() string {
	// Check $fpath from environment
	fpath := os.Getenv("FPATH")
	if fpath != "" {
		parts := strings.Split(fpath, ":")
		for _, p := range parts {
			if p != "" {
				if info, err := os.Stat(p); err == nil && info.IsDir() {
					return filepath.Join(p, "_hawk")
				}
			}
		}
		// Use first entry even if it doesn't exist yet
		if parts[0] != "" {
			return filepath.Join(parts[0], "_hawk")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/usr/local/share/zsh/site-functions/_hawk"
	}
	return filepath.Join(home, ".zsh", "completions", "_hawk")
}

func fishInstallPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "hawk.fish")
	}
	return filepath.Join(home, ".config", "fish", "completions", "hawk.fish")
}

// escapeZsh escapes single quotes for zsh completion descriptions.
func escapeZsh(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// escapeFish escapes single quotes for fish completion descriptions.
func escapeFish(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}
