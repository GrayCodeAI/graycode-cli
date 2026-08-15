package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/engine/git"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

var (
	// Status bar — each metric its own hue so the bar reads as a
	// strip of distinct information. Aliases for the global palette
	// (in case callers want to use these names directly).
	statusCWDColor    = cwdBlue
	statusBranchColor = branchYellow
	statusSpecColor   = infoSky
	statusTokenColor  = tokenSage
	statusCostColor   = costViolet
	statusPRColor     = lipgloss.Color("#56D4DD") // cyan — unique hue in the footer row

	statusCwdStyle          = lipgloss.NewStyle().Foreground(statusCWDColor).Inline(true)
	statusPRStyle           = lipgloss.NewStyle().Foreground(statusPRColor).Inline(true)
	statusBranchStyle       = lipgloss.NewStyle().Foreground(statusBranchColor).Inline(true)
	statusSpecStyle         = lipgloss.NewStyle().Foreground(statusSpecColor).Inline(true)
	statusTokenStyle        = lipgloss.NewStyle().Foreground(statusTokenColor).Inline(true)
	statusCostStyle         = lipgloss.NewStyle().Foreground(statusCostColor).Inline(true)
	statusClockStyle        = lipgloss.NewStyle().Foreground(hudLabelPink).Inline(true)
	statusFocusStyle        = lipgloss.NewStyle().Foreground(infoSky).Inline(true)
	statusDimStyle          = lipgloss.NewStyle().Foreground(dimColor).Inline(true)
	dryRunStyle             = lipgloss.NewStyle().Foreground(warnAmber).Bold(true).Inline(true)
	containerModeStyle      = lipgloss.NewStyle().Foreground(successTeal).Bold(true).Inline(true)
	containerModeErrStyle   = lipgloss.NewStyle().Foreground(errorCoral).Bold(true).Inline(true)
	containerModeMutedStyle = lipgloss.NewStyle().Foreground(dimColor).Bold(true).Inline(true)
)

// renderStatusBar renders the session stats footer below the input area.
// Returns 1 line normally, or 2 lines when the terminal is wide enough
// (width >= 120) and secondary operational state is present.
func renderStatusBar(m *chatModel, width int) []string {
	if width < 20 {
		width = 80
	}
	left := renderStatusBarLeft(m)
	right := renderStatusBarRight(m)
	// Two-line layout from 100 cols so control-plane chips are visible more often.
	if width >= 100 {
		primary := layoutFooterRow(renderStatusBarPrimaryLeft(m), renderStatusBarPrimaryRight(m), width)
		secondary := layoutFooterRow(renderStatusBarSecondaryLeft(m), renderStatusBarSecondaryRight(m), width)
		if secondary == "" || strings.TrimSpace(stripANSI(secondary)) == "" {
			return []string{primary}
		}
		return []string{primary, secondary}
	}
	// Narrow: fold a compact control-plane chip into the left cluster.
	left = mergeNarrowControlChip(m, left)
	return []string{layoutFooterRow(left, right, width)}
}

// stripANSI removes CSI sequences for empty checks (status bar only).
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) {
				if s[j] >= 0x40 && s[j] <= 0x7e {
					j++
					break
				}
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// mergeNarrowControlChip appends mode·iso·trust when width is too small for a second row.
func mergeNarrowControlChip(m *chatModel, left string) string {
	chip := controlPlaneChip(m)
	if chip == "" {
		return left
	}
	if left == "" {
		return chip
	}
	return left + statusDimStyle.Render(" · ") + chip
}

func controlPlaneChip(m *chatModel) string {
	if m == nil || m.session == nil {
		return ""
	}
	chip := ""
	branch := cachedStatusBranch(m)
	if branch == "main" || branch == "master" {
		chip += dryRunStyle.Render("main!")
	}
	if m.session.AutoCommit() {
		chip += statusDimStyle.Render("  auto-commit")
	}
	return chip
}

// renderStatusBarPrimaryLeft — cwd, branch, spec stage.
func renderStatusBarPrimaryLeft(m *chatModel) string {
	cwd, ok := cachedStatusLeftCwd(m)
	if !ok {
		return ""
	}
	parts := []string{statusCwdStyle.Render(cwd)}
	if branch := cachedStatusBranch(m); branch != "" {
		parts = append(parts, statusBranchStyle.Render(icons.Branch()+" "+branch))
		if m != nil && len(m.statusLeftPRs) > 0 {
			parts = append(parts, statusPRStyle.Render(icons.PullRequest()+" "+strings.Join(m.statusLeftPRs, " ")))
		}
	}
	if stage := specStageForStatus(m); stage != "" {
		parts = append(parts, statusSpecStyle.Render(stage))
	}
	return strings.Join(parts, statusDimStyle.Render(" · "))
}

// renderStatusBarPrimaryRight — tokens, cost, duration.
func renderStatusBarPrimaryRight(m *chatModel) string {
	if m == nil || m.session == nil {
		return ""
	}
	tokens := m.session.CostValue().PromptTokens + m.session.CostValue().CompletionTokens
	tokenText := icons.Database() + " " + formatTokenCountCompact(tokens) + " tokens"
	costText := formatStatusCost(m.session.CostValue())
	parts := []string{
		statusTokenStyle.Render(tokenText),
		statusCostStyle.Render(costText),
	}
	sessionDur := time.Duration(0)
	if !m.sessionStartedAt.IsZero() {
		sessionDur = time.Since(m.sessionStartedAt)
	}
	parts = append(parts, statusClockStyle.Render(icons.ClockOutline()+" "+formatSessionDuration(sessionDur)))
	if m.waiting || m.manualCompacting {
		parts = append(parts, statusClockStyle.Render(formatSessionDuration(requestDuration(m))))
	}
	return strings.Join(parts, statusDimStyle.Render(" · "))
}

func formatStatusCost(c *engine.Cost) string {
	if c == nil {
		return icons.Ruby() + " $0.00"
	}
	return fmt.Sprintf("%s $%.3f", icons.Ruby(), c.TotalUSD())
}

func renderStatusBarSecondaryLeft(m *chatModel) string {
	return ""
}

// renderStatusBarSecondaryRight — errors, dry-run, vim, focus/pause, spend.
func renderStatusBarSecondaryRight(m *chatModel) string {
	if m == nil || m.session == nil {
		return ""
	}
	var parts []string
	if m.inScrollbackFocus() {
		parts = append(parts, statusFocusStyle.Render("⧉"))
	}
	if m.waiting && !m.streamFollow {
		parts = append(parts, statusDimStyle.Render(icons.Pause()))
	}
	// Container mode is already shown in the top footer row (containerFooterLeft),
	// so we only surface it here when there's an error to report.
	if m.containerEnabled && m.containerErr != nil {
		parts = append(parts, containerModeErrStyle.Render("▣ container error"))
	}
	if m.session != nil && m.session.PermSvc() != nil && m.session.PermSvc().DryRun() {
		parts = append(parts, dryRunStyle.Render(icons.Pause()+" DRY-RUN"))
	}
	// Always-visible compact spend on wide secondary row.
	if c := m.session.CostValue(); c != nil {
		if usd := c.TotalUSD(); usd > 0 {
			parts = append(parts, statusCostStyle.Render(fmt.Sprintf("$%.3f", usd)))
		} else if c.Total() > 0 {
			parts = append(parts, statusCostStyle.Render(fmt.Sprintf("%s %.2f", icons.Ruby(), c.Total())))
		}
	}
	if m.session.AutoCommit() {
		parts = append(parts, statusDimStyle.Render("auto-commit"))
	}
	if m.vim != nil && m.vim.IsEnabled() {
		parts = append(parts, statusDimStyle.Render(m.vim.ModeString()))
	}
	return strings.Join(parts, statusDimStyle.Render(" · "))
}

// statusBranchTTL bounds how long the cwd+branch segment is cached, so a
// branch switch shows up in the status bar within a few seconds.
const statusBranchTTL = 5 * time.Second

// statusPRTTL bounds how long open-PR numbers are cached. gh is a network
// call, so it is refreshed far less often than the branch lookup.
const statusPRTTL = 30 * time.Second

// prProvider is the lazily-detected git provider used for the status bar
// PR lookup. Detection runs once; the provider is reused across refreshes.
var (
	prProviderOnce sync.Once
	prProvider     *git.GitProvider
)

func cachedStatusPRProvider() *git.GitProvider {
	prProviderOnce.Do(func() {
		typ, owner, repo := git.DetectProvider("")
		if owner != "" && repo != "" {
			prProvider = git.NewGitProvider(typ, "", owner, repo)
		}
	})
	return prProvider
}

// fetchStatusLeftPRs refreshes the open-PR numbers for branch. Never
// blocks the TUI: failures and "gh not installed" degrade to nil.
func fetchStatusLeftPRs(branch string) []string {
	gp := cachedStatusPRProvider()
	if gp == nil {
		return nil
	}
	nums, err := gp.OpenPRNumbers(branch)
	if err != nil || len(nums) == 0 {
		return nil
	}
	out := make([]string, 0, len(nums))
	for _, n := range nums {
		out = append(out, fmt.Sprintf("#%d", n))
	}
	return out
}

func fetchStatusLeftPRsCmd(branch string) tea.Cmd {
	return func() tea.Msg {
		nums := fetchStatusLeftPRs(branch)
		return statusLeftPRsMsg{branch: branch, nums: nums}
	}
}

func (m *chatModel) refreshStatusBarLeft(force bool) (bool, tea.Cmd) {
	if m == nil {
		return false, nil
	}
	// Fast path: within TTL with a cached value — avoid the os.Getwd syscall
	// on every keystroke. cwd is only re-resolved when a refresh is actually
	// due or forced.
	if !force && m.statusLeftKey != "" && m.statusLeftVal != "" && time.Since(m.statusLeftAt) < statusBranchTTL {
		return false, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	if !force && m.statusLeftKey == cwd && m.statusLeftVal != "" && time.Since(m.statusLeftAt) < statusBranchTTL {
		return false, nil
	}
	branch := ""
	if b, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD"); err == nil && b != "" {
		branch = b
		if branch == "HEAD" {
			branch, _ = gitOutput("rev-parse", "--short", "HEAD")
		}
	}
	m.statusLeftKey = cwd
	m.statusLeftVal = shortenHomePath(cwd)
	m.statusLeftBranch = branch
	m.statusLeftAt = time.Now()
	var prCmd tea.Cmd
	if branch != "" && (force || time.Since(m.statusLeftPRAt) > statusPRTTL) {
		m.statusLeftPRAt = time.Now()
		prCmd = fetchStatusLeftPRsCmd(branch)
	}
	return true, prCmd
}

func renderStatusBarLeft(m *chatModel) string {
	cwd, ok := cachedStatusLeftCwd(m)
	if !ok {
		return ""
	}
	parts := []string{statusCwdStyle.Render(cwd)}
	if branch := cachedStatusBranch(m); branch != "" {
		parts = append(parts, statusBranchStyle.Render(icons.Branch()+" "+branch))
		if m != nil && len(m.statusLeftPRs) > 0 {
			parts = append(parts, statusPRStyle.Render(icons.PullRequest()+" "+strings.Join(m.statusLeftPRs, " ")))
		}
	}
	if stage := specStageForStatus(m); stage != "" {
		parts = append(parts, statusSpecStyle.Render(stage))
	}
	return strings.Join(parts, statusDimStyle.Render(" · "))
}

// specStageForStatus returns a short spec stage indicator for the status bar,
// or empty string if no spec workflow is active.
func specStageForStatus(m *chatModel) string {
	if m == nil || m.session == nil || m.session.PermSvc() == nil {
		return ""
	}
	stage := m.session.PermSvc().SpecStage()
	if stage == engine.SpecStageNone {
		return ""
	}
	label := specStageDisplayName(stage)
	phase, phases := m.session.PermSvc().SpecPhaseProgress()
	if stage == engine.SpecStageImplementing && phases > 0 {
		return fmt.Sprintf("%s %s %d/%d", icons.FileDocument(), label, phase, phases)
	}
	return fmt.Sprintf("%s %s", icons.FileDocument(), label)
}

func cachedStatusLeftCwd(m *chatModel) (string, bool) {
	if m == nil {
		cwd, err := os.Getwd()
		if err != nil {
			return ".", false
		}
		return shortenHomePath(cwd), true
	}
	if m.statusLeftVal == "" {
		return "", false
	}
	return m.statusLeftVal, true
}

func cachedStatusBranch(m *chatModel) string {
	if m == nil {
		branch, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
		if err != nil || branch == "" {
			return ""
		}
		if branch == "HEAD" {
			branch, _ = gitOutput("rev-parse", "--short", "HEAD")
		}
		return branch
	}
	return m.statusLeftBranch
}

func renderStatusBarRight(m *chatModel) string {
	if m == nil || m.session == nil {
		return ""
	}

	tokens := m.session.CostValue().PromptTokens + m.session.CostValue().CompletionTokens
	tokenText := icons.Database() + " " + formatTokenCountCompact(tokens) + " tokens"
	costText := formatStatusCost(m.session.CostValue())
	var meta []string
	if m.inScrollbackFocus() {
		meta = append(meta, statusFocusStyle.Render("⧉"))
	}
	if m.waiting && !m.streamFollow {
		meta = append(meta, statusDimStyle.Render(icons.Pause()))
	}

	parts := append(
		meta,
		statusTokenStyle.Render(tokenText),
		statusCostStyle.Render(costText),
	)

	// Context window usage — compact bar showing how full the context is.
	if m.session != nil {
		if used := sessionContextUsedTokens(m.session); used > 0 {
			if window := m.session.ContextWindowSize(); window > 0 {
				pct := int(float64(used) / float64(window) * 100)
				if pct > 999 {
					pct = 999
				}
				ctxText := fmt.Sprintf("ctx %d%%", pct)
				parts = append(parts, statusDimStyle.Render(ctxText))
			}
		}
	}

	sessionDur := time.Duration(0)
	if !m.sessionStartedAt.IsZero() {
		sessionDur = time.Since(m.sessionStartedAt)
	}
	clockText := icons.ClockOutline() + " " + formatSessionDuration(sessionDur)
	parts = append(parts, statusClockStyle.Render(clockText))

	if m.waiting || m.manualCompacting {
		timerText := formatSessionDuration(requestDuration(m))
		parts = append(parts, statusClockStyle.Render(timerText))
	}

	// Prominent dry-run indicator — safety-critical awareness.
	if m.session != nil && m.session.PermSvc() != nil && m.session.PermSvc().DryRun() {
		parts = append(parts, dryRunStyle.Render(icons.Pause()+" DRY-RUN"))
	}

	if m.vim != nil && m.vim.IsEnabled() {
		parts = append(parts, statusDimStyle.Render(m.vim.ModeString()))
	}
	// Persistent autonomy tier indicator — always visible for safety awareness.
	if m.session != nil && m.session.PermSvc() != nil {
		level := effectivePermissionTier(m.session)
		parts = append(parts, autonomyTierStyle(level).Render("◈ "+autonomyTierName(level)))
	}

	// Persistent Docker isolation indicator — safety-critical awareness.
	if m.containerReady {
		parts = append(parts, containerModeStyle.Render(icons.Shield()+" isolated"))
	} else if m.containerErr != nil || !m.containerEnabled {
		parts = append(parts, containerModeErrStyle.Render(icons.Alert()+" Docker required"))
	} else {
		parts = append(parts, containerModeMutedStyle.Render(icons.Container()+" starting"))
	}

	return strings.Join(parts, statusDimStyle.Render(" · "))
}

func sessionDuration(m *chatModel) time.Duration {
	if m == nil {
		return 0
	}
	start := m.sessionStartedAt
	if start.IsZero() {
		start = m.startedAt
	}
	if start.IsZero() {
		return 0
	}
	return time.Since(start)
}

func requestDuration(m *chatModel) time.Duration {
	if m == nil || m.startedAt.IsZero() {
		return 0
	}
	return time.Since(m.startedAt)
}

func shortenHomePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func formatSessionDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSecs := int(d.Seconds())
	if totalSecs < 60 {
		return fmt.Sprintf("%ds", totalSecs)
	}
	mins := totalSecs / 60
	secs := totalSecs % 60
	if mins >= 60 {
		return fmt.Sprintf("%dh %dm", mins/60, mins%60)
	}
	return fmt.Sprintf("%dm %ds", mins, secs)
}

func formatTokenCountWithCommas(tokens int) string {
	p := message.NewPrinter(language.English)
	return p.Sprintf("%d tokens", tokens)
}

func renderContainerFooterLeft(m chatModel) string {
	bold, dim := containerFooterLeft(m)

	if m.containerErr != nil || !m.containerEnabled {
		return containerErrStyle.Bold(true).Render(bold) + containerErrStyle.Render(dim)
	}
	return containerLabelStyle.Render(bold) + renderContainerFooterDetail(dim, m.session)
}

func renderContainerFooterDetail(detail string, sess *engine.Session) string {
	if detail == "" {
		return ""
	}
	statusStyle := lipgloss.NewStyle().Foreground(textPlaceholder).Inline(true)
	sep := " · "
	status, tierPart, found := strings.Cut(detail, sep)
	if !found {
		return statusStyle.Render(detail)
	}
	var level engine.AutonomyLevel
	if sess != nil && sess.PermSvc().Autonomy() != 0 {
		level = sess.PermSvc().Autonomy()
	} else {
		level = autonomyLevelForTierName(tierPart)
	}
	return statusStyle.Render(status) + configMutedStyle().Inline(true).Render(sep) + autonomyTierStyle(level).Render(strings.TrimSpace(tierPart))
}

// containerFooterLeft is the bold + dim text on the top footer row (left side).
func containerFooterLeft(m chatModel) (bold, dim string) {
	bold = icons.Container() + " Docker:"
	if !m.containerEnabled {
		return bold, " required · agent tools locked"
	}
	if m.containerErr != nil {
		return bold, " unavailable · press r to retry"
	}
	if m.containerReady && strings.TrimSpace(m.containerStatus) != "" {
		tier := "Builder"
		if m.session != nil && m.session.PermSvc().Autonomy() != 0 {
			tier = autonomyTierName(m.session.PermSvc().Autonomy())
		}
		status := shortenFooterContainerStatus(strings.TrimSpace(m.containerStatus))
		if stage := currentSpecStage(m.session); stage != engine.SpecStageNone && stage != engine.SpecStageImplementing {
			return bold, fmt.Sprintf(" %s · %s · spec:%s", status, tier, specStageDisplayName(stage))
		}
		return bold, fmt.Sprintf(" %s · %s", status, tier)
	}
	if strings.TrimSpace(m.containerStatus) != "" {
		status := strings.TrimSpace(m.containerStatus)
		if !m.containerReady && m.containerErr == nil {
			// Show activity indicator during active boot so the user sees progress.
			return bold, fmt.Sprintf(" ◐ %s", status)
		}
		return bold, " " + status
	}
	if !m.containerReady && m.containerErr == nil {
		return bold, " ◐ starting…"
	}
	return bold, " starting…"
}

func statusLineSummary(m *chatModel) string {
	if m == nil || m.session == nil {
		return "no active session"
	}
	cwd, _ := os.Getwd()
	branch, _ := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if branch == "HEAD" {
		branch, _ = gitOutput("rev-parse", "--short", "HEAD")
	}
	gw, model, _ := m.connectionStatusParts()
	tokens := m.session.CostValue().PromptTokens + m.session.CostValue().CompletionTokens
	return fmt.Sprintf(
		"Status line (footer)\n  cwd: %s\n  branch: %s\n  gateway: %s\n  model: %s\n  tokens: %s\n  cost: $%.2f\n  duration: %s\n  %s",
		shortenHomePath(cwd),
		strings.TrimSpace(branch),
		gw,
		model,
		formatTokenCountWithCommas(tokens),
		m.session.CostValue().Total(),
		formatSessionDuration(sessionDuration(m)),
		m.session.CostValue().Summary(),
	)
}
