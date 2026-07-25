package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var manpageCmd = &cobra.Command{
	Use:   "manpage",
	Short: "Generate man page in roff format",
	Long:  "Generate a man page for hawk in roff format and print it to stdout.\nRedirect to a file in your man path, e.g.: hawk manpage > /usr/local/share/man/man1/hawk.1",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprint(cmd.OutOrStdout(), GenerateManPage())
		return nil
	},
}

// GenerateManPage produces a man page in roff format for hawk.
// The OPTIONS section is generated from the live Cobra flag set so it
// never drifts from the actual CLI surface.
func GenerateManPage() string {
	date := time.Now().Format("January 2006")
	ver := version
	if ver == "" {
		ver = "dev"
	}

	var b strings.Builder

	// Header
	b.WriteString(fmt.Sprintf(`.TH HAWK 1 "%s" "%s" "User Commands"`, date, ver))
	b.WriteString("\n")

	// Name
	b.WriteString(".SH NAME\nhawk \\- AI coding agent powered by eyrie\n")

	// Synopsis
	b.WriteString(".SH SYNOPSIS\n")
	b.WriteString(".B hawk\n[\\fIOPTIONS\\fR] [\\fIPROMPT\\fR]\n")

	// Description
	b.WriteString(".SH DESCRIPTION\n")
	b.WriteString("hawk is an AI coding agent that reads, writes, and runs code in your terminal.\n")
	b.WriteString("It connects to 75+ LLM providers through eyrie, executes tools (file I/O,\n")
	b.WriteString("shell, git, web search), and manages sessions from a keyboard-driven TUI\n")
	b.WriteString("or headless mode for scripts and CI.\n")

	// Options — generated from the live Cobra flag set
	b.WriteString(".SH OPTIONS\n")
	rootCmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		var flagStr string
		if f.Shorthand != "" {
			flagStr = fmt.Sprintf("\\fB-%s\\fR, \\fB--%s\\fR", f.Shorthand, f.Name)
		} else {
			flagStr = fmt.Sprintf("\\fB--%s\\fR", f.Name)
		}
		// Add value placeholder for non-bool flags
		if f.Value.Type() != "bool" {
			flagStr += " \\fI" + strings.ToUpper(f.Name) + "\\fR"
		}
		usage := f.Usage
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "[]" {
			usage += fmt.Sprintf(" (default: %s)", f.DefValue)
		}
		b.WriteString(fmt.Sprintf(".TP\n%s\n%s\n", flagStr, usage))
	})

	// Subcommands
	b.WriteString(".SH COMMANDS\n")
	for _, sub := range rootCmd.Commands() {
		if sub.Hidden {
			continue
		}
		b.WriteString(fmt.Sprintf(".TP\n\\fBhawk %s\\fR\n%s\n", sub.Use, sub.Short))
	}

	// Slash Commands
	b.WriteString(".SH SLASH COMMANDS\n")
	b.WriteString("In interactive mode, type / followed by a command:\n")
	slashCmds := []struct{ cmd, desc string }{
		{"/help", "Show available commands"},
		{"/config", "Open configuration wizard"},
		{"/model NAME", "Switch model"},
		{"/clear", "Clear conversation"},
		{"/compact", "Compact conversation history"},
		{"/history", "List saved sessions"},
		{"/resume ID", "Resume a session"},
		{"/commit", "Auto-commit changes"},
		{"/review", "Code review"},
		{"/doctor", "Run diagnostics"},
		{"/tools", "List enabled tools"},
		{"/quit", "Exit hawk"},
	}
	for _, sc := range slashCmds {
		b.WriteString(fmt.Sprintf(".TP\n\\fB%s\\fR\n%s\n", sc.cmd, sc.desc))
	}

	// Files
	b.WriteString(".SH FILES\n")
	b.WriteString(".TP\n\\fBHawk user config directory\\fR\nGlobal configuration files\n")
	b.WriteString(".TP\n\\fBAGENTS.md\\fR\nProject instructions file\n")
	b.WriteString(".TP\n\\fBHawk user state directory\\fR\nSaved session data, plans, skills, and runtime state\n")

	// Credentials
	b.WriteString(".SH CREDENTIALS\n")
	b.WriteString("API keys are stored in the OS secret service (macOS Keychain or Linux GNOME Keyring / KWallet).\n")
	b.WriteString("Use \\fBhawk\\fR and \\fB/config\\fR to save keys; hawk does not read API keys from .env files.\n")
	b.WriteString(".TP\n\\fBhawk credentials status\\fR\nShow secure storage status\n")
	b.WriteString(".TP\n\\fBhawk credentials remove <provider|env-var>\\fR\nRemove a stored API key from the OS secret store\n")
	b.WriteString(".TP\n\\fBhawk credentials migrate\\fR\nImport legacy plaintext credential files into the OS store\n")

	// Environment
	b.WriteString(".SH ENVIRONMENT\n")
	b.WriteString("Non-secret overrides (optional):\n")
	envVars := []struct{ env, desc string }{
		{"OPENAI_MODEL", "Override default OpenAI model"},
		{"OLLAMA_BASE_URL", "Ollama server URL (also saved via /config for Ollama)"},
		{"HAWK_CONFIG_DIR", "Override hawk config directory"},
	}
	for _, ev := range envVars {
		b.WriteString(fmt.Sprintf(".TP\n\\fB%s\\fR\n%s\n", ev.env, ev.desc))
	}

	// Authors
	b.WriteString(".SH AUTHORS\nGrayCode AI <https://github.com/GrayCodeAI/hawk>\n")

	return b.String()
}
