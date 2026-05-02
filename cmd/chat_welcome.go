package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/mattn/go-runewidth"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
	"github.com/GrayCodeAI/hawk/engine"
	"github.com/GrayCodeAI/hawk/session"
	"github.com/GrayCodeAI/hawk/tool"
)

func buildWelcomeMessage(sess *engine.Session, sessionID string, registry *tool.Registry, saved *session.Session, settings hawkconfig.Settings, blinkClosed bool, width int) string {
	logoC := "\033[38;2;255;94;14m"
	mascotC := "\033[38;2;255;94;14m"
	dimC := "\033[2m"
	boldC := "\033[1m"
	greenC := "\033[38;2;78;205;196m"
	redC := "\033[38;2;224;85;85m"
	rst := "\033[0m"

	totalW := width
	if totalW < 40 {
		totalW = 80
	}

	center := func(s string, visLen int) string {
		pad := (totalW - visLen) / 2
		if pad < 0 {
			pad = 0
		}
		return strings.Repeat(" ", pad) + s
	}

	art := []string{
		"██   ██  █████   ██     ██ ██   ██",
		"██   ██ ██   ██  ██     ██ ██  ██ ",
		"███████ ███████  ██  █  ██ █████  ",
		"██   ██ ██   ██  ██ ███ ██ ██  ██ ",
		"██   ██ ██   ██   ███ ███  ██   ██",
	}
	mascot := []string{
		"   ▄▄▄▄▄▄   ",
		" ▄█ ▄  ▄ █▄ ",
		" ███ ██ ███ ",
		"  ██ ██ ██  ",
		"  ▀▀    ▀▀  ",
	}
	if blinkClosed {
		mascot[1] = " ▄█ ─  ─ █▄ "
	}

	showMascot := totalW >= 60

	var b strings.Builder
	b.WriteString("\n\n\n\n")

	for i := 0; i < len(art); i++ {
		line := art[i]
		mLine := ""
		if showMascot && i < len(mascot) {
			mLine = mascot[i]
		}
		combined := logoC + line + rst
		visW := runewidth.StringWidth(line)
		if mLine != "" {
			combined += "    " + mascotC + mLine + rst
			visW += 4 + runewidth.StringWidth(mLine)
		}
		b.WriteString(center(combined, visW) + "\n")
	}

	verLine := fmt.Sprintf("v%s", version)
	b.WriteString("\n" + center(dimC+verLine+rst, len(verLine)) + "\n")

	tip := "TIP: Use /help to see all available commands"
	b.WriteString("\n" + center(boldC+tip+rst, len(tip)) + "\n")

	shortcuts := "shift+tab to cycle modes · ctrl+N to cycle models"
	b.WriteString("\n" + center(dimC+shortcuts+rst, len(shortcuts)) + "\n")
	shortcuts2 := "ctrl+L for autonomy · tab for reasoning"
	b.WriteString(center(dimC+shortcuts2+rst, len(shortcuts2)) + "\n")

	skillsCount := 0
	mcpCount := len(settings.MCPServers) + len(mcpServers)

	skillMark := redC + "×" + rst
	mcpMark := greenC + "✓" + rst
	if mcpCount == 0 {
		mcpMark = redC + "×" + rst
	}
	hawkMark := greenC + "✓" + rst
	if hawkconfig.LoadAgentsMD() == "" {
		hawkMark = redC + "×" + rst
	}

	indicators := fmt.Sprintf("Skills (%d) %s  MCPs (%d) %s  AGENTS.md %s", skillsCount, skillMark, mcpCount, mcpMark, hawkMark)
	indVis := fmt.Sprintf("Skills (%d) x  MCPs (%d) x  AGENTS.md x", skillsCount, mcpCount)
	b.WriteString("\n" + center(indicators, len(indVis)) + "\n")

	if resume := actLine(saved, sessionID); resume != "" {
		b.WriteString("\n")
		b.WriteString(center(dimC+resume+rst, len(resume)) + "\n")
	}

	return b.String()
}

func actLine(saved *session.Session, sessionID string) string {
	if saved != nil && len(sessionID) >= 8 {
		return "Resumed session " + sessionID[:8]
	}
	return ""
}

func permissionCommandArg(text, action string) string {
	prefix := "/permissions " + action
	if !strings.HasPrefix(text, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(text, prefix))
}

func toolListSummary(registry *tool.Registry) string {
	if registry == nil {
		return "No tools enabled."
	}
	tools := registry.EyrieTools()
	if len(tools) == 0 {
		return "No tools enabled."
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Enabled tools (%d):\n", len(tools)))
	for _, t := range tools {
		desc := t.Description
		if len(desc) > 96 {
			desc = desc[:96] + "..."
		}
		b.WriteString(fmt.Sprintf("  %s — %s\n", t.Name, desc))
	}
	return strings.TrimRight(b.String(), "\n")
}

func envSummary(provider, model string) string {
	envKeys := []string{
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"GEMINI_API_KEY",
		"OPENROUTER_API_KEY",
		"CANOPYWAVE_API_KEY",
		"XAI_API_KEY",
		"OPENCODEGO_API_KEY",
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Provider: %s\nModel: %s\n\nEnvironment:\n", provider, model))
	for _, key := range envKeys {
		status := "missing"
		if os.Getenv(key) != "" {
			status = "set"
		}
		b.WriteString(fmt.Sprintf("  %s: %s\n", key, status))
	}
	return strings.TrimRight(b.String(), "\n")
}

func configCommandSummary(settings hawkconfig.Settings) string {
	provider := displayConfigValue(settings.Provider)
	model := displayConfigValue(settings.Model)
	return fmt.Sprintf(`Configure Hawk

Run these commands:
  /config provider openai
  /model gpt-4o

Current:
  provider: %s
  model: %s
  configured keys: %s

API keys are set via environment variables (herm-style).
More:
  /config keys
  /config get <key>
  /config set <key> <value>`, provider, model, configuredKeyList())
}

func modelConfigSummary(provider, model string) string {
	return fmt.Sprintf("Provider: %s\nModel: %s\n\nUse\n  /model <name>\n  /config provider <name>", displayConfigValue(provider), displayConfigValue(model))
}

func apiKeyConfigSummary() string {
	return "API keys (from environment)\n" + indentedAPIKeyLines()
}

func configuredKeyList() string {
	var providers []string
	for _, line := range apiKeyStatusLines() {
		name, status, ok := strings.Cut(line, ": ")
		if ok && status == "set" {
			providers = append(providers, name)
		}
	}
	if len(providers) == 0 {
		return "(none)"
	}
	return strings.Join(providers, ", ")
}

func indentedAPIKeyLines() string {
	lines := apiKeyStatusLines()
	if len(lines) == 0 {
		return "  (empty)"
	}
	return "  " + strings.Join(lines, "\n  ")
}

func apiKeyStatusLines() []string {
	providers := client.NewEyrieClient(nil).GetProviders()
	sort.Strings(providers)
	var lines []string
	for _, provider := range providers {
		lines = append(lines, fmt.Sprintf("%s: %s", provider, hawkconfig.EnvKeyStatus(provider)))
	}
	return lines
}

func activeProviderKeyStatus(settings hawkconfig.Settings) string {
	provider := strings.TrimSpace(settings.Provider)
	if provider == "" {
		return "select provider first"
	}
	return hawkconfig.EnvKeyStatus(provider)
}

func displayConfigValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(empty)"
	}
	return value
}

func providerHint(provider, model string) string {
	return fmt.Sprintf("Provider: %s\nModel: %s", displayConfigValue(provider), displayConfigValue(model))
}
