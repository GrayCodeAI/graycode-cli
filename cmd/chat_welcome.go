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

func (m *chatModel) rebuildWelcomeCache(opts ...any) {
	frame := m.eyeFrame
	if len(opts) > 0 {
		switch v := opts[0].(type) {
		case int:
			frame = v
		case bool:
			if v {
				frame = 2
			} else {
				frame = 0
			}
		}
	}
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
	m.welcomeCache = buildWelcomeMessageWithSnapshot(m.session, m.sessionID, m.registry, nil, m.settings, skillsCount, connectedMCPCount(m.registry), frame, width, height, m.welcomeDockerRunning(), m.welcomeStatusSnapshot(), m.containerEnabled, m.lastCommand)
}

// buildWelcomeMessage renders the branded inline HAWK welcome block.
func buildWelcomeMessage(sess *engine.Session, sessionID string, registry *tool.Registry, saved *session.Session, settings hawkconfig.Settings, skillsCount int, blinkClosed bool, width, height int, dockerRunning *bool) string {
	frame := 0
	if blinkClosed {
		frame = 2
	}
	return buildWelcomeMessageWithSnapshot(sess, sessionID, registry, saved, settings, skillsCount, connectedMCPCount(registry), frame, width, height, dockerRunning, loadWelcomeStatusSnapshot(), false, "")
}

func buildWelcomeMessageWithSnapshot(sess *engine.Session, sessionID string, registry *tool.Registry, saved *session.Session, settings hawkconfig.Settings, skillsCount, mcpCount int, eyeFrame int, width, height int, dockerRunning *bool, snapshot welcomeStatusSnapshot, containerMode bool, lastCommand string) string {
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
	var eyeGlyph string
	switch eyeFrame {
	case 1, 3:
		eyeGlyph = "|o\\/o|"
	case 2:
		eyeGlyph = "|-\\/-|"
	}
	if eyeGlyph != "" {
		art = append([]string(nil), hawkLogoArtLines...)
		for i, line := range art {
			art[i] = strings.Replace(line, "|0\\/0|", eyeGlyph, 1)
		}
	}

	// Inject the version into the hawk's body — centered in the lower gap.
	verStr := DisplayVersion()
	if verStr != "" && !strings.HasPrefix(verStr, "v") && !strings.HasPrefix(verStr, "V") {
		verStr = "v" + verStr
	}
	const verGap = 14
	if len(verStr) > verGap {
		verStr = verStr[:verGap]
	}
	verLeft := (verGap - len(verStr)) / 2
	verRight := verGap - len(verStr) - verLeft
	verWing := strings.Repeat(" ", verLeft) + verStr + strings.Repeat(" ", verRight)
	for i, line := range art {
		art[i] = strings.Replace(line, "(\\              /)", "(\\"+verWing+"/)", 1)
	}

	var b strings.Builder

	// Top breathing room so the wordmark isn't flush against the terminal edge.
	b.WriteString("\n")

	if tight {
		// Compact single-line wordmark for small terminals — version sits
		// inline so it's always visible even when the full hawk is hidden.
		verDisplay := DisplayVersion()
		if verDisplay != "" && !strings.HasPrefix(verDisplay, "v") && !strings.HasPrefix(verDisplay, "V") {
			verDisplay = "v" + verDisplay
		}
		compactArt := logoC + "HAWK" + rst + "  " + verDisplay
		b.WriteString(center(runewidth.StringWidth("HAWK   "+verDisplay), compactArt) + "\n")
	} else {
		artW := blockLinesWidth(art)
		for _, line := range art {
			b.WriteString(center(artW, logoC+line+rst) + "\n")
		}
	}

	modeBadge := welcomeModeBadge(dockerRunning)
	cpLine := ""
	if sess != nil {
		cpLine = welcomeControlPlaneLine(sess, dimC, rst, modeBadge != "")
	}
	modeLine := modeBadge
	if cpLine != "" {
		if modeBadge != "" {
			modeLine += "  ·  " + cpLine
		} else {
			modeLine = cpLine
		}
	}
	b.WriteString("\n")
	b.WriteString(center(visibleWidth(modeLine), modeLine) + "\n")

	indicators := welcomeIndicatorRow(skillsCount, snapshot.agentsOK, mcpCount, greenC, sepC, rst, markPresent, markNone)
	b.WriteByte('\n')
	b.WriteString(center(visibleWidth(indicators), indicators) + "\n")

	if resume := actLine(saved, sessionID); resume != "" {
		b.WriteString("\n")
		b.WriteString(center(runewidth.StringWidth(resume), dimC+resume+rst) + "\n")
	}

	return b.String()
}

// welcomeControlPlaneLine renders the work-mode · isolation · folder-trust
// indicator on the welcome screen (moved out of the footer bar). When the
// CONTAINER badge is shown, the redundant iso segment is dropped.
func welcomeControlPlaneLine(sess *engine.Session, dimC, rst string, badgeShown bool) string {
	work := sess.WorkMode()
	// Use the denser terminal glyphs here. The UI already has the semantic
	// text; these icons should add contrast, not vanish into the line height.
	modeIcon := icons.Terminal()
	modeLabel := "Action Mode"
	modeColor := ansiCyan
	switch work {
	case engine.WorkModePlan:
		modeIcon = icons.Brain()
		modeLabel = "Planning Mode"
		modeColor = ansiMagenta
	case engine.WorkModeReview:
		modeIcon = icons.Magnify()
		modeLabel = "Review Mode"
		modeColor = ansiAmber
	}

	isoIcon := icons.Container()
	isoColor := ansiAmber
	iso := sess.Isolation().ShortLabel()

	isoSeg := "  ·  " + isoColor + isoIcon + " " + iso + rst
	if badgeShown {
		isoSeg = ""
	}

	tr := engine.ProjectTrust("")
	var trustIcon string
	trustColor := dimC
	if !tr.Enforced {
		trustIcon = icons.CircleOutline()
		trustColor = dimC
	} else if tr.Trusted {
		trustIcon = icons.CheckDecagram()
		trustColor = ansiVividGreen
	} else if tr.Blocked {
		trustIcon = icons.CloseCircle()
		trustColor = ansiCoral
	} else {
		trustIcon = icons.CloseThick()
		trustColor = ansiAmber
	}
	trustLabel := tr.String()
	switch trustLabel {
	case "trusted":
		trustLabel = "Trusted"
	case "blocked":
		trustLabel = "Blocked"
	case "":
		trustLabel = "Untrusted"
	default:
		if len(trustLabel) > 0 {
			trustLabel = strings.ToUpper(trustLabel[:1]) + trustLabel[1:]
		}
	}

	return modeColor + modeIcon + " " + modeLabel + rst +
		isoSeg +
		"  ·  " + trustColor + trustIcon + " " + trustLabel + rst
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
		skillsColor, skillsMark = ansiLightPink, markPresent
	}

	agentsColor, agentsMark := idleC, markNone
	if agentsOK {
		agentsColor, agentsMark = ansiMagenta, markPresent
	}

	mcpColor, mcpMark := idleC, markNone
	if mcpCount > 0 {
		mcpColor, mcpMark = ansiCyan, markPresent
	}
	return fmt.Sprintf(
		"%s%s Skills (%d)%s %s  ·  %s%s AGENTS.md%s %s  ·  %s%s MCPs (%d)%s %s",
		skillsColor, icons.Bolt(), skillsCount, rst, skillsMark,
		agentsColor, icons.Robot(), rst, agentsMark,
		mcpColor, icons.Network(), mcpCount, rst, mcpMark,
	)
}

// welcomeModeBadge returns a prominent, colored badge indicating the
// current execution mode. No background fill — bright foreground colors
// (gold for starting, container blue for ready, coral for required) keep it readable
// on any theme.
func welcomeModeBadge(dockerRunning *bool) string {
	rst := ansiReset
	switch {
	case dockerRunning == nil:
		// Startup — Talon Gold, bold, timer = waiting for the sandbox.
		return "\033[1m" + ansiOrange + icons.Timer() + " Container Starting" + rst
	case *dockerRunning:
		// Ready — container blue communicates healthy isolation.
		return "\033[1m" + ansiContBlue + icons.Shield() + " Container" + rst
	default:
		// Failure — no host fallback exists.
		return "\033[1m" + ansiCoral + icons.Alert() + " Container Required" + rst
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
	registered := len(registry.PrimaryTools())
	if len(tools) == 0 {
		return "No tools enabled."
	}
	var b strings.Builder
	if registered > len(tools) {
		b.WriteString(fmt.Sprintf("Model-visible tools (%d of %d registered — lazy surface):\n", len(tools), registered))
	} else {
		b.WriteString(fmt.Sprintf("Enabled tools (%d):\n", len(tools)))
	}
	for _, t := range tools {
		desc := t.Description
		if runes := []rune(desc); len(runes) > 96 {
			desc = string(runes[:96]) + "..."
		}
		b.WriteString(fmt.Sprintf("  %s — %s\n", t.Name, desc))
	}
	if registered > len(tools) {
		b.WriteString("\nUnlock more: ToolSearch with query select:<ToolName>")
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
