package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// TokenEntry records a single token usage event.
type TokenEntry struct {
	Timestamp    time.Time
	InputTokens  int
	OutputTokens int
	Model        string
	ToolName     string
	CostUSD      float64
	Cumulative   int
}

// BudgetAlert represents a budget threshold notification.
type BudgetAlert struct {
	Level      string // "info", "warning", "critical"
	Message    string
	Triggered  time.Time
	Percentage float64
}

// TokenReporter provides real-time visibility into token spending and
// projections throughout a session.
type TokenReporter struct {
	Entries       []TokenEntry
	SessionBudget int
	SessionSpent  int
	Alerts        []BudgetAlert
	mu            sync.RWMutex
	startTime     time.Time
}

// NewTokenReporter creates a TokenReporter with the given session budget.
func NewTokenReporter(sessionBudget int) *TokenReporter {
	return &TokenReporter{
		Entries:       make([]TokenEntry, 0),
		SessionBudget: sessionBudget,
		Alerts:        make([]BudgetAlert, 0),
		startTime:     time.Now(),
	}
}

// Record logs a token usage event, updates cumulative totals, and checks
// budget thresholds.
func (tr *TokenReporter) Record(input, output int, model, tool string, cost float64) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.SessionSpent += input + output

	entry := TokenEntry{
		Timestamp:    time.Now(),
		InputTokens:  input,
		OutputTokens: output,
		Model:        model,
		ToolName:     tool,
		CostUSD:      cost,
		Cumulative:   tr.SessionSpent,
	}
	tr.Entries = append(tr.Entries, entry)

	// Check thresholds after recording.
	tr.checkThresholds()
}

// GetRemaining returns the number of tokens remaining in the session budget.
func (tr *TokenReporter) GetRemaining() int {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	remaining := tr.SessionBudget - tr.SessionSpent
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetUsageRate returns the token usage rate in tokens per minute over
// the session duration.
func (tr *TokenReporter) GetUsageRate() float64 {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	elapsed := time.Since(tr.startTime).Minutes()
	if elapsed <= 0 {
		return 0
	}
	return float64(tr.SessionSpent) / elapsed
}

// ProjectCompletion estimates the remaining time until the budget is
// exhausted at the current usage rate.
func (tr *TokenReporter) ProjectCompletion() time.Duration {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	elapsed := time.Since(tr.startTime).Minutes()
	if elapsed <= 0 || tr.SessionSpent == 0 {
		return 0
	}

	rate := float64(tr.SessionSpent) / elapsed // tokens per minute
	remaining := tr.SessionBudget - tr.SessionSpent
	if remaining <= 0 {
		return 0
	}

	minutesLeft := float64(remaining) / rate
	return time.Duration(minutesLeft * float64(time.Minute))
}

// RenderLive returns a compact single-block status display showing current
// token usage, rate, ETA, and last tool usage.
func (tr *TokenReporter) RenderLive() string {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	percentage := 0.0
	if tr.SessionBudget > 0 {
		percentage = float64(tr.SessionSpent) / float64(tr.SessionBudget) * 100
	}

	bar := tr.renderBar(percentage, 20)

	// Rate calculation
	elapsed := time.Since(tr.startTime).Minutes()
	var rate float64
	if elapsed > 0 {
		rate = float64(tr.SessionSpent) / elapsed
	}

	// ETA calculation
	eta := "N/A"
	remaining := tr.SessionBudget - tr.SessionSpent
	if rate > 0 && remaining > 0 {
		minutesLeft := float64(remaining) / rate
		eta = trFormatDuration(time.Duration(minutesLeft * float64(time.Minute)))
	} else if remaining <= 0 {
		eta = "exhausted"
	}

	// Total cost
	var totalCost float64
	for _, e := range tr.Entries {
		totalCost += e.CostUSD
	}

	// Last tool info
	lastTool := "N/A"
	lastTok := 0
	if len(tr.Entries) > 0 {
		last := tr.Entries[len(tr.Entries)-1]
		lastTool = last.ToolName
		lastTok = last.InputTokens + last.OutputTokens
	}

	lines := []string{
		fmt.Sprintf("Tokens: %s / %s [%s] %.1f%%",
			trFormatNumber(tr.SessionSpent), trFormatNumber(tr.SessionBudget), bar, percentage),
		fmt.Sprintf("Rate: %s tok/min | ETA: %s remaining",
			trFormatNumber(int(rate)), eta),
		fmt.Sprintf("Cost: $%.2f | Last: %s (%s tok)",
			totalCost, lastTool, trFormatNumber(lastTok)),
	}

	return strings.Join(lines, "\n")
}

// RenderBreakdown returns a detailed breakdown of token usage by input/output,
// model, and tool.
func (tr *TokenReporter) RenderBreakdown() string {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	var totalInput, totalOutput int
	modelTokens := make(map[string]int)
	toolTokens := make(map[string]int)

	for _, e := range tr.Entries {
		totalInput += e.InputTokens
		totalOutput += e.OutputTokens
		modelTokens[e.Model] += e.InputTokens + e.OutputTokens
		toolTokens[e.ToolName] += e.InputTokens + e.OutputTokens
	}

	total := totalInput + totalOutput
	if total == 0 {
		return "Token Breakdown:\n─────────────────\nNo token usage recorded."
	}

	var sb strings.Builder
	sb.WriteString("Token Breakdown:\n")
	sb.WriteString("─────────────────\n")
	sb.WriteString(fmt.Sprintf("Input:  %s (%d%%)\n", trFormatNumber(totalInput), trPercent(totalInput, total)))
	sb.WriteString(fmt.Sprintf("Output: %s (%d%%)\n", trFormatNumber(totalOutput), trPercent(totalOutput, total)))
	sb.WriteString("\nBy model:\n")

	// Sort models by token count descending.
	modelOrder := trSortedKeys(modelTokens)
	for _, m := range modelOrder {
		tok := modelTokens[m]
		sb.WriteString(fmt.Sprintf("  %s: %s (%d%%)\n", m, trFormatNumber(tok), trPercent(tok, total)))
	}

	sb.WriteString("\nBy tool:\n")

	// Sort tools by token count descending.
	toolOrder := trSortedKeys(toolTokens)
	for _, t := range toolOrder {
		tok := toolTokens[t]
		sb.WriteString(fmt.Sprintf("  %s: %s (%d%%)\n", t, trFormatNumber(tok), trPercent(tok, total)))
	}

	return strings.TrimRight(sb.String(), "\n")
}

// CheckBudget evaluates current spending against the session budget and
// returns an alert if a threshold has been newly crossed.
func (tr *TokenReporter) CheckBudget() *BudgetAlert {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	return tr.checkThresholdsReturn()
}

// FormatAlert returns a formatted string representation of a BudgetAlert.
func (tr *TokenReporter) FormatAlert(alert *BudgetAlert) string {
	if alert == nil {
		return ""
	}

	var icon string
	switch alert.Level {
	case "info":
		icon = "[INFO]"
	case "warning":
		icon = "[WARNING]"
	case "critical":
		icon = "[CRITICAL]"
	default:
		icon = "[ALERT]"
	}

	return fmt.Sprintf("%s %.1f%% budget used - %s", icon, alert.Percentage, alert.Message)
}

// GetHistory returns the last n token entries. If last exceeds the number
// of entries, all entries are returned.
func (tr *TokenReporter) GetHistory(last int) []TokenEntry {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	if last <= 0 {
		return nil
	}

	if last >= len(tr.Entries) {
		result := make([]TokenEntry, len(tr.Entries))
		copy(result, tr.Entries)
		return result
	}

	start := len(tr.Entries) - last
	result := make([]TokenEntry, last)
	copy(result, tr.Entries[start:])
	return result
}

// Reset clears all recorded entries, spent tokens, and alerts, keeping
// the session budget intact.
func (tr *TokenReporter) Reset() {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.Entries = make([]TokenEntry, 0)
	tr.SessionSpent = 0
	tr.Alerts = make([]BudgetAlert, 0)
	tr.startTime = time.Now()
}

// checkThresholds checks budget thresholds and appends any new alerts.
// Must be called with mu held.
func (tr *TokenReporter) checkThresholds() {
	if tr.SessionBudget <= 0 {
		return
	}

	pct := float64(tr.SessionSpent) / float64(tr.SessionBudget) * 100

	type threshold struct {
		pct   float64
		level string
		msg   string
	}

	thresholds := []threshold{
		{90, "critical", "Budget nearly exhausted. Consider wrapping up."},
		{75, "warning", "Three-quarters of token budget consumed."},
		{50, "info", "Half of token budget consumed."},
	}

	for _, th := range thresholds {
		if pct >= th.pct && !tr.hasAlertAtLevel(th.level) {
			tr.Alerts = append(tr.Alerts, BudgetAlert{
				Level:      th.level,
				Message:    th.msg,
				Triggered:  time.Now(),
				Percentage: pct,
			})
		}
	}
}

// checkThresholdsReturn checks and returns the highest new alert if any.
// Must be called with mu held.
func (tr *TokenReporter) checkThresholdsReturn() *BudgetAlert {
	if tr.SessionBudget <= 0 {
		return nil
	}

	pct := float64(tr.SessionSpent) / float64(tr.SessionBudget) * 100

	type threshold struct {
		pct   float64
		level string
		msg   string
	}

	thresholds := []threshold{
		{90, "critical", "Budget nearly exhausted. Consider wrapping up."},
		{75, "warning", "Three-quarters of token budget consumed."},
		{50, "info", "Half of token budget consumed."},
	}

	for _, th := range thresholds {
		if pct >= th.pct && !tr.hasAlertAtLevel(th.level) {
			alert := BudgetAlert{
				Level:      th.level,
				Message:    th.msg,
				Triggered:  time.Now(),
				Percentage: pct,
			}
			tr.Alerts = append(tr.Alerts, alert)
			return &alert
		}
	}

	// Return the highest existing alert if no new one.
	if pct >= 90 {
		return tr.findAlert("critical")
	}
	if pct >= 75 {
		return tr.findAlert("warning")
	}
	if pct >= 50 {
		return tr.findAlert("info")
	}

	return nil
}

func (tr *TokenReporter) hasAlertAtLevel(level string) bool {
	for _, a := range tr.Alerts {
		if a.Level == level {
			return true
		}
	}
	return false
}

func (tr *TokenReporter) findAlert(level string) *BudgetAlert {
	for i := len(tr.Alerts) - 1; i >= 0; i-- {
		if tr.Alerts[i].Level == level {
			return &tr.Alerts[i]
		}
	}
	return nil
}

// renderBar produces a progress bar string with filled and empty blocks.
func (tr *TokenReporter) renderBar(percentage float64, width int) string {
	filled := int(percentage / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// trFormatNumber formats an integer with comma separators.
func trFormatNumber(n int) string {
	if n < 0 {
		return "-" + trFormatNumber(-n)
	}

	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
		if len(s) > remainder {
			result.WriteString(",")
		}
	}

	for i := remainder; i < len(s); i += 3 {
		result.WriteString(s[i : i+3])
		if i+3 < len(s) {
			result.WriteString(",")
		}
	}

	return result.String()
}

// trFormatDuration converts a duration to a human-readable string like "2h 4m".
func trFormatDuration(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// trPercent computes the integer percentage of part relative to total.
func trPercent(part, total int) int {
	if total == 0 {
		return 0
	}
	return int(float64(part) / float64(total) * 100)
}

// trSortedKeys returns map keys sorted by value descending.
func trSortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort (maps are typically small).
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && m[keys[j]] > m[keys[j-1]]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
