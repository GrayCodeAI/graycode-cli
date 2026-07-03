package cmd

import (
	"fmt"
	"strings"
	"time"
)

// GenerateManPage produces a man page in roff format for hawk.
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
	b.WriteString("It supports multiple LLM providers and features a TUI with slash commands,\n")
	b.WriteString("tool execution, session management, and plugin support.\n")

	// Options
	b.WriteString(".SH OPTIONS\n")
	options := []struct{ flag, desc string }{
		{"-m, --model MODEL", "Model to use (e.g. claude-sonnet-4-20250514)"},
		{"-p, --print", "Print mode: send prompt and exit"},
		{"--prompt PROMPT", "Send a single prompt and exit"},
		{"--provider PROVIDER", "LLM provider (anthropic, openai, gemini, etc.)"},
		{"-r, --resume ID", "Resume a saved session by ID"},
		{"-c, --continue", "Continue the most recent conversation"},
		{"--output-format FORMAT", "Output format: text, json, or stream-json"},
		{"--input-format FORMAT", "Input format: text or stream-json"},
		{"--system-prompt TEXT", "Custom system prompt"},
		{"--system-prompt-file FILE", "Read system prompt from file"},
		{"--append-system-prompt TEXT", "Append text to system prompt"},
		{"--sandbox MODE", "Permission sandbox: strict, workspace, or off"},
		{"--max-turns N", "Maximum agentic turns in non-interactive mode"},
		{"--max-budget-usd AMOUNT", "Maximum estimated API spend in USD"},
		{"--tools TOOLS", "Comma-separated tool list"},
		{"--add-dir DIR", "Additional directory to include in context"},
		{"--mcp CMD", "MCP server command"},
		{"-v, --version", "Show version"},
	}
	for _, opt := range options {
		b.WriteString(fmt.Sprintf(".TP\n\\fB%s\\fR\n%s\n", opt.flag, opt.desc))
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

	// Credentials (stored in OS secret service — use /config, not .env)
	b.WriteString(".SH CREDENTIALS\n")
	b.WriteString("API keys are stored in the OS secret service (macOS Keychain or Linux GNOME Keyring / KWallet).\n")
	b.WriteString("Use \\fBhawk\\fR and \\fB/config\\fR to save keys; hawk does not read API keys from .env files.\n")
	b.WriteString(".TP\n\\fBhawk credentials status\\fR\nShow secure storage status\n")
	b.WriteString(".TP\n\\fBhawk credentials remove <provider|env-var>\\fR\nRemove a stored API key from the OS secret store\n")
	b.WriteString(".TP\n\\fB/config key remove\\fR\nRemove a stored API key via interactive picker\n")
	b.WriteString(".TP\n\\fBhawk credentials migrate\\fR\nImport legacy plaintext credential files into the OS store\n")

	// Environment (non-secret overrides only)
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
