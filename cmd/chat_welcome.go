package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

type welcomeStatusSnapshot struct {
	setup    hawkconfig.SetupState
	agentsOK bool
}

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
	// Avoid probing Docker before first paint. The async container bootstrap
	// path updates the welcome panel once it knows the real state.
	return nil
}

func loadWelcomeStatusSnapshot() welcomeStatusSnapshot {
	ctx := context.Background()
	return welcomeStatusSnapshot{
		setup:    hawkconfig.EvaluateSetupCached(ctx),
		agentsOK: hawkconfig.LoadAgentsMD() != "",
	}
}

func (m *chatModel) refreshWelcomeStatusSnapshot() {
	snapshot := loadWelcomeStatusSnapshot()
	m.welcomeSetupState = snapshot.setup
	m.welcomeAgentsOK = snapshot.agentsOK
}

func (m chatModel) welcomeStatusSnapshot() welcomeStatusSnapshot {
	return welcomeStatusSnapshot{
		setup:    m.welcomeSetupState,
		agentsOK: m.welcomeAgentsOK,
	}
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
	m.welcomeCache = buildWelcomeMessageWithSnapshot(m.session, m.sessionID, m.registry, nil, m.settings, skillsCount, connectedMCPCount(m.registry), blinkClosed, width, height, m.welcomeDockerRunning(), m.welcomeStatusSnapshot(), m.containerEnabled, m.lastCommand)
}

// buildWelcomeMessage renders the branded inline HAWK welcome block.
func buildWelcomeMessage(sess *engine.Session, sessionID string, registry *tool.Registry, saved *session.Session, settings hawkconfig.Settings, skillsCount int, blinkClosed bool, width, height int, dockerRunning *bool) string {
	return buildWelcomeMessageWithSnapshot(sess, sessionID, registry, saved, settings, skillsCount, connectedMCPCount(registry), blinkClosed, width, height, dockerRunning, loadWelcomeStatusSnapshot(), false, "")
}

func buildWelcomeMessageWithSnapshot(sess *engine.Session, sessionID string, registry *tool.Registry, saved *session.Session, settings hawkconfig.Settings, skillsCount, mcpCount int, blinkClosed bool, width, height int, dockerRunning *bool, snapshot welcomeStatusSnapshot, containerMode bool, lastCommand string) string {
	// Talon Gold is used for the HAWK wordmark. All escapes come from the
	// theme palette (theme.go) so a rebrand stays a one-file change.
	logoC := ansiOrange
	dimC := ansiDim
	// Indicator colors — same as the rest of the TUI palette (success
	// teal, error coral) so the ✓/× marks match the colors used
	// elsewhere for success/error states.
	greenC := ansiTeal
	sepC := ansiGrayDim
	rst := ansiReset

	// Status marks — green ✓ = present, dim ○ = none (not an error),
	// red × = actual problem (e.g. Docker enabled but not running). Using a
	// neutral mark for "none" avoids the alarming all-red look on a fresh repo.
	markPresent := greenC + icons.CheckBold() + rst
	markNone := sepC + "○" + rst

	totalW := width
	if totalW < 40 {
		totalW = 80
	}
	totalH := height
	if totalH <= 0 {
		totalH = 24
	}
	tight := totalH < 30 || totalW < 72

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
	if blinkClosed {
		art = append([]string(nil), hawkLogoArtLines...)
		for i, line := range art {
			art[i] = strings.Replace(line, "|0\\/0|", "|-\\/-|", 1)
		}
	}

	var b strings.Builder

	// Top breathing room so the wordmark isn't flush against the terminal edge.
	b.WriteString("\n")

	if tight {
		// Compact single-line wordmark for small terminals.
		compactArt := logoC + "HAWK" + rst
		b.WriteString(center(runewidth.StringWidth("HAWK"), compactArt) + "\n")
	} else {
		artW := blockLinesWidth(art)
		for _, line := range art {
			b.WriteString(center(artW, logoC+line+rst) + "\n")
		}
	}

	verLine := fmt.Sprintf("v%s", DisplayVersion())
	b.WriteByte('\n')

	// Execution mode stays beside the version: compact, but prominent enough
	// to preserve safety awareness before the first command runs.
	modeBadge := welcomeModeBadge(dockerRunning)
	modeLine := dimC + verLine + rst + "   " + modeBadge
	b.WriteString(center(runewidth.StringWidth(verLine)+3+visibleWidth(modeBadge), modeLine) + "\n")

	indicators := welcomeIndicatorRow(skillsCount, snapshot.agentsOK, mcpCount, greenC, sepC, rst, markPresent, markNone)
	b.WriteByte('\n')
	b.WriteString(center(visibleWidth(indicators), indicators) + "\n")

	if resume := actLine(saved, sessionID); resume != "" {
		b.WriteString("\n")
		b.WriteString(center(runewidth.StringWidth(resume), dimC+resume+rst) + "\n")
	}

	return b.String()
}

type mcpServerNamed interface {
	MCPServerName() string
}

func connectedMCPCount(registry *tool.Registry) int {
	if registry == nil {
		return 0
	}
	servers := make(map[string]struct{})
	for _, candidate := range registry.PrimaryTools() {
		mcpTool, ok := candidate.(mcpServerNamed)
		if !ok || mcpTool.MCPServerName() == "" {
			continue
		}
		servers[mcpTool.MCPServerName()] = struct{}{}
	}
	return len(servers)
}

func welcomeIndicatorRow(skillsCount int, agentsOK bool, mcpCount int, activeC, idleC, rst, markPresent, markNone string) string {
	skillsColor, skillsMark := idleC, markNone
	if skillsCount > 0 {
		skillsColor, skillsMark = activeC, markPresent
	}

	agentsColor, agentsMark := idleC, markNone
	if agentsOK {
		agentsColor, agentsMark = activeC, markPresent
	}

	mcpColor, mcpMark := idleC, markNone
	if mcpCount > 0 {
		mcpColor, mcpMark = activeC, markPresent
	}
	return fmt.Sprintf(
		"%s%s%s %sSkills (%d)%s %s  ·  %s%s%s AGENTS.md %s  ·  %s%s%s %sMCPs (%d)%s %s",
		skillsColor, icons.Bolt(), rst,
		skillsColor, skillsCount, rst, skillsMark,
		agentsColor, icons.Robot(), rst, agentsMark,
		mcpColor, icons.Network(), rst,
		mcpColor, mcpCount, rst, mcpMark,
	)
}

// welcomeModeBadge returns a prominent, colored badge indicating the
// current execution mode. Uses inverse video (colored background, dark
// text) so it stands out from the dim guidance text.
func welcomeModeBadge(dockerRunning *bool) string {
	rst := ansiReset
	switch {
	case dockerRunning == nil:
		// Startup — Talon Gold background, dark text.
		return "\033[48;2;255;215;0m\033[30m " + icons.Container() + " CONTAINER · STARTING \033[0m" + rst
	case *dockerRunning:
		// Ready — teal communicates healthy isolation.
		return "\033[48;2;78;205;196m\033[30m " + icons.Shield() + " CONTAINER · DOCKER · ISOLATED \033[0m" + rst
	default:
		// Failure — no host fallback exists.
		return "\033[48;2;255;107;107m\033[30m " + icons.Alert() + " CONTAINER · DOCKER REQUIRED \033[0m" + rst
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
		if runes := []rune(desc); len(runes) > 96 {
			desc = string(runes[:96]) + "..."
		}
		b.WriteString(fmt.Sprintf("  %s — %s\n", t.Name, desc))
	}
	return strings.TrimRight(b.String(), "\n")
}

func envSummary(provider, model string) string {
	return envSummaryWithSelection(provider, model, true)
}

func envSummaryWithSelection(provider, model string, includeSelection bool) string {
	var providers []string
	for _, gateway := range hawkconfig.GatewayStatuses(context.Background(), provider, model) {
		providers = append(providers, gateway.ID)
	}
	sort.Strings(providers)
	var b strings.Builder
	if includeSelection {
		b.WriteString(fmt.Sprintf("Provider: %s\nModel: %s\n\n", provider, model))
	}
	b.WriteString(fmt.Sprintf("Credentials (%s):\n", hawkconfig.CredentialStoreName()))
	for _, providerID := range providers {
		b.WriteString(fmt.Sprintf("  %s: %s\n", providerID, hawkconfig.EnvKeyStatus(providerID)))
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
	return "API keys (" + hawkconfig.CredentialStoreName() + ")\n" + indentedAPIKeyLines()
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
	providers := hawkconfig.AllSetupGateways()
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
