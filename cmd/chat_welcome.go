package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/eyrie/setup"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

func welcomeDockerSegment(dockerRunning *bool, greenC, redC, rst string) (segment string, visLen int) {
	if dockerRunning == nil {
		return "", 0
	}
	mark := redC + "×" + rst
	if *dockerRunning {
		mark = greenC + "✓" + rst
	}
	segment = "  Docker " + mark
	return segment, len("  Docker x")
}

func (m chatModel) welcomeDockerRunning() *bool {
	if !m.containerEnabled {
		return nil
	}
	if m.containerReady {
		ok := true
		return &ok
	}
	if m.containerErr != nil {
		ok := false
		return &ok
	}
	ok := sandbox.DockerAvailable()
	return &ok
}

func (m *chatModel) rebuildWelcomeCache(blinkClosed bool) {
	width := m.width
	if width <= 0 {
		width = 80
	}
	m.welcomeCache = buildWelcomeMessage(m.session, m.sessionID, m.registry, nil, m.settings, blinkClosed, width, m.welcomeDockerRunning())
}

func buildWelcomeMessage(sess *engine.Session, sessionID string, registry *tool.Registry, saved *session.Session, settings hawkconfig.Settings, blinkClosed bool, width int, dockerRunning *bool) string {
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

	verLine := fmt.Sprintf("v%s", DisplayVersion())
	b.WriteString("\n" + center(dimC+verLine+rst, len(verLine)) + "\n")

	setup := hawkconfig.EvaluateSetupCached(context.Background())
	needsSetup := setup.NeedsSetup
	if needsSetup {
		tip := "Run /config to add an API key, then type your first message"
		b.WriteString("\n" + center(boldC+tip+rst, len(tip)) + "\n")
	} else {
		tip := "TIP: /help for commands · /model to switch model"
		b.WriteString("\n" + center(boldC+tip+rst, len(tip)) + "\n")
		shortcuts := "ctrl+N next model · ctrl+L autonomy · esc cancel"
		b.WriteString(center(dimC+shortcuts+rst, len(shortcuts)) + "\n")
	}

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
	if dockerSeg, _ := welcomeDockerSegment(dockerRunning, greenC, redC, rst); dockerSeg != "" {
		indicators += dockerSeg
		indVis += "  Docker x"
	}
	b.WriteString("\n" + center(indicators, len(indVis)) + "\n")

	if hint := setup.Hint; hint != "" {
		b.WriteString("\n" + center(boldC+hint+rst, len(hint)) + "\n")
	}

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
	return envSummaryWithSelection(provider, model, true)
}

func envSummaryWithSelection(provider, model string, includeSelection bool) string {
	compiled, _ := setup.LoadCompiledCatalog(context.Background())
	var envKeys []string
	if compiled != nil {
		envKeys = catalog.DiscoveryEnvKeysFromCatalog(compiled)
	}
	sort.Strings(envKeys)
	var b strings.Builder
	if includeSelection {
		b.WriteString(fmt.Sprintf("Provider: %s\nModel: %s\n\n", provider, model))
	}
	b.WriteString(fmt.Sprintf("Credentials (%s):\n", credentials.PlatformSecretStoreName()))
	ctx := context.Background()
	for _, key := range envKeys {
		status := "missing"
		if credentials.HasSecret(ctx, key) {
			status = "set"
		}
		b.WriteString(fmt.Sprintf("  %s: %s\n", key, status))
	}
	return strings.TrimRight(b.String(), "\n")
}

func configCommandSummary(settings hawkconfig.Settings) string {
	_ = settings
	provider := displayConfigValue(hawkconfig.ActiveProvider(nil))
	model := displayConfigValue(hawkconfig.ActiveModel(nil))
	return fmt.Sprintf(`Setup (eyrie)

  /config  → API key + model

Current:
  provider: %s
  model:    %s
  keys:     %s

Model catalog and routing live in eyrie — hawk is the UI only.`, provider, model, configuredKeyList())
}

func apiKeyConfigSummary() string {
	return "API keys (" + credentials.PlatformSecretStoreName() + ")\n" + indentedAPIKeyLines()
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
	providers := client.Client(nil).GetProviders()
	sort.Strings(providers)
	var lines []string
	for _, provider := range providers {
		lines = append(lines, fmt.Sprintf("%s: %s", provider, hawkconfig.EnvKeyStatus(provider)))
	}
	return lines
}

func displayConfigValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(empty)"
	}
	return value
}
