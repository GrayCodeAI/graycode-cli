package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/eyrie/setup"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

func welcomeDockerSegment(dockerRunning *bool, greenC, redC, rst string) (segment string, visLen int) {
	if dockerRunning == nil {
		return "", 0
	}
	mark := redC + "×" + rst
	if *dockerRunning {
		mark = greenC + icons.CheckBold() + " " + rst
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
	height := m.height
	if height <= 0 {
		height = 24
	}
	skillsCount := 0
	if m.pluginRuntime != nil {
		skillsCount = len(m.pluginRuntime.SmartSkills)
	}
	m.welcomeCache = buildWelcomeMessage(m.session, m.sessionID, m.registry, nil, m.settings, skillsCount, blinkClosed, width, height, m.welcomeDockerRunning())
}

// buildWelcomeMessage renders the branded inline HAWK welcome block.
func buildWelcomeMessage(sess *engine.Session, sessionID string, registry *tool.Registry, saved *session.Session, settings hawkconfig.Settings, skillsCount int, blinkClosed bool, width, height int, dockerRunning *bool) string {
	// Brand orange — used for both the HAWK wordmark and the mascot so
	// the welcome screen stays on theme.
	logoC := "\033[38;2;255;94;14m" // brand orange — WELCOME TO + HAWK wordmark
	mascotC := "\033[38;2;255;94;14m"
	dimC := "\033[2m"
	boldC := "\033[1m"
	// Indicator colors — same as the rest of the TUI palette (success
	// teal, error coral) so the ✓/× marks match the colors used
	// elsewhere for success/error states.
	greenC := "\033[38;2;78;205;196m" // successTeal
	redC := "\033[38;2;255;107;107m"  // errorCoral
	amberC := "\033[38;2;255;179;71m" // warnAmber
	sepC := "\033[38;2;102;102;102m"  // textDisabled — chip separators
	rst := "\033[0m"

	// Status marks — green ✓ = present, dim ○ = none (not an error),
	// red × = actual problem (e.g. Docker enabled but not running). Using a
	// neutral mark for "none" avoids the alarming all-red look on a fresh repo.
	markPresent := greenC + "+" + icons.CheckBold() + " " + rst
	markNone := sepC + "○" + rst

	totalW := width
	if totalW < 40 {
		totalW = 80
	}
	totalH := height
	if totalH <= 0 {
		totalH = 24
	}
	tight := totalH <= 20 || totalW < 72

	center := func(visW int, styled string) string {
		if visW <= 0 {
			visW = runewidth.StringWidth(styled)
		}
		pad := (totalW - visW) / 2
		if pad < 0 {
			pad = 0
		}
		return strings.Repeat(" ", pad) + styled
	}

	art := hawkLogoArtLines
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

	showMascot := totalW >= 60 && !tight

	var b strings.Builder

	// Top breathing room so the wordmark isn't flush against the terminal edge.
	b.WriteString("\n\n")

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
		b.WriteString(center(visW, combined) + "\n")
	}

	verLine := fmt.Sprintf("v%s", DisplayVersion())
	b.WriteByte('\n')
	b.WriteString(center(len(verLine), dimC+verLine+rst) + "\n")

	setup := hawkconfig.EvaluateSetupCached(context.Background())
	needsSetup := setup.NeedsSetup
	modeGuidance := welcomeModeGuidance(dockerRunning, tight)
	if needsSetup {
		if hint := setup.Hint; hint != "" {
			b.WriteByte('\n')
			b.WriteString(center(len(hint), amberC+hint+rst) + "\n")
		}
	}
	if needsSetup {
		quick := "Quick start: /config to connect a provider and pick a model · /help for commands"
		b.WriteByte('\n')
		b.WriteString(center(len(quick), boldC+quick+rst) + "\n")
		example := "Then ask: explain this repo · fix the failing test · add tests for cmd/eval"
		if tight {
			example = "Then ask: explain this repo · fix the failing test"
		}
		b.WriteString(center(runewidth.StringWidth(example), dimC+example+rst) + "\n")
		if modeGuidance != "" {
			b.WriteString(center(runewidth.StringWidth(modeGuidance), dimC+modeGuidance+rst) + "\n")
		}
	}
	if !needsSetup {
		tip := "TIP: Use /new to start a fresh session with clean context"
		if tight {
			tip = "TIP: /new starts a clean session"
		}
		b.WriteByte('\n')
		b.WriteString(center(len(tip), boldC+tip+rst) + "\n")
		shortcutsRow1 := "ctrl+N for new session · ctrl+L for autonomy"
		shortcutsRow2 := "/help for commands · /config for setup · /permissions for approvals"
		if tight {
			shortcutsRow1 = "ctrl+N new session · ctrl+L autonomy"
			shortcutsRow2 = "/help · /config · /permissions"
		}
		b.WriteByte('\n')
		b.WriteString(center(runewidth.StringWidth(shortcutsRow1), dimC+shortcutsRow1+rst) + "\n")
		b.WriteString(center(runewidth.StringWidth(shortcutsRow2), dimC+shortcutsRow2+rst) + "\n")
	}

	mcpCount := len(settings.MCPServers) + len(mcpServers)
	agentsOK := hawkconfig.LoadAgentsMD() != ""

	mark := func(present bool) string {
		if present {
			return markPresent
		}
		return markNone
	}
	skillMark := mark(skillsCount > 0)
	mcpMark := mark(mcpCount > 0)
	hawkMark := mark(agentsOK)

	indicators := fmt.Sprintf("Skills (%d) %s  MCPs (%d) %s  AGENTS.md %s", skillsCount, skillMark, mcpCount, mcpMark, hawkMark)
	indVis := fmt.Sprintf("Skills (%d) x  MCPs (%d) x  AGENTS.md x", skillsCount, mcpCount)
	if dockerSeg, _ := welcomeDockerSegment(dockerRunning, greenC, redC, rst); dockerSeg != "" {
		indicators += dockerSeg
		indVis += "  Docker x"
	}
	b.WriteByte('\n')
	b.WriteString(center(len(indVis), indicators) + "\n")

	if resume := actLine(saved, sessionID); resume != "" {
		b.WriteString("\n")
		b.WriteString(center(len(resume), dimC+resume+rst) + "\n")
	}

	return b.String()
}

func welcomeModeGuidance(dockerRunning *bool, tight bool) string {
	switch {
	case dockerRunning == nil:
		if tight {
			return "Host mode runs commands locally · /permissions changes approvals"
		}
		return "Host mode runs commands on your machine · /permissions changes approvals"
	case *dockerRunning:
		if tight {
			return "Container mode isolates tool execution · /permissions changes approvals"
		}
		return "Container mode isolates tool execution when available · /permissions changes approvals"
	default:
		if tight {
			return "Docker unavailable, so commands run locally · /permissions changes approvals"
		}
		return "Docker is unavailable, so Hawk runs commands on your machine · /permissions changes approvals"
	}
}

func actLine(saved *session.Session, sessionID string) string {
	if saved != nil && len(sessionID) >= 8 {
		return "Resumed session " + sessionID[:8]
	}
	return ""
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
	providerName := displayConfigValue(hawkconfig.ActiveProvider(context.Background()))
	modelName := displayConfigValue(hawkconfig.ActiveModel(context.Background()))
	return fmt.Sprintf(`Setup (eyrie)

  /config  → paste API key (OS keychain) + pick model
  /path    → verify readiness in TUI
  hawk path (CLI)

Current:
  provider: %s
  model:    %s
  keys:     %s

Model catalog and routing live in eyrie — hawk is the UI only.`, providerName, modelName, configuredKeyList())
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
	providers := types.NewClient(nil).GetProviders()
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
