package analytics

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Dashboard provides a local telemetry dashboard for graycode showing usage analytics,
// performance metrics, and session history in a terminal-friendly format.
type Dashboard struct {
	Sessions     []SessionSummary          `json:"sessions"`
	DailyStats   []DayStat                 `json:"daily_stats"`
	ModelUsage   map[string]DashModelStats `json:"model_usage"`
	ToolUsage    map[string]int            `json:"tool_usage"`
	TotalCostUSD float64                   `json:"total_cost_usd"`
	TotalTokens  int                       `json:"total_tokens"`
	ActiveDays   int                       `json:"active_days"`
}

// SessionSummary captures key metrics for a single session.
type SessionSummary struct {
	ID            string        `json:"id"`
	Date          time.Time     `json:"date"`
	Duration      time.Duration `json:"duration"`
	Model         string        `json:"model"`
	Provider      string        `json:"provider"`
	TokensUsed    int           `json:"tokens_used"`
	CostUSD       float64       `json:"cost_usd"`
	ToolCalls     int           `json:"tool_calls"`
	FilesModified int           `json:"files_modified"`
	Success       bool          `json:"success"`
}

// DayStat aggregates metrics for a single day.
type DayStat struct {
	Date        time.Time     `json:"date"`
	Sessions    int           `json:"sessions"`
	Tokens      int           `json:"tokens"`
	CostUSD     float64       `json:"cost_usd"`
	AvgDuration time.Duration `json:"avg_duration"`
}

// DashModelStats tracks usage and performance per model for the dashboard.
type DashModelStats struct {
	Requests     int     `json:"requests"`
	Tokens       int     `json:"tokens"`
	CostUSD      float64 `json:"cost_usd"`
	AvgLatencyMs int     `json:"avg_latency_ms"`
	ErrorRate    float64 `json:"error_rate"`
}

// NewDashboard creates an initialized empty Dashboard.
func NewDashboard() *Dashboard {
	return &Dashboard{
		Sessions:   make([]SessionSummary, 0),
		DailyStats: make([]DayStat, 0),
		ModelUsage: make(map[string]DashModelStats),
		ToolUsage:  make(map[string]int),
	}
}

// RecordSession adds a session summary to the dashboard and updates aggregates.
func (d *Dashboard) RecordSession(summary SessionSummary) {
	d.Sessions = append(d.Sessions, summary)
	d.TotalCostUSD += summary.CostUSD
	d.TotalTokens += summary.TokensUsed

	// Update model usage
	ms := d.ModelUsage[summary.Model]
	ms.Requests++
	ms.Tokens += summary.TokensUsed
	ms.CostUSD += summary.CostUSD
	d.ModelUsage[summary.Model] = ms

	// Update daily stats
	dayKey := summary.Date.Truncate(24 * time.Hour)
	found := false
	for i := range d.DailyStats {
		if d.DailyStats[i].Date.Equal(dayKey) {
			d.DailyStats[i].Sessions++
			d.DailyStats[i].Tokens += summary.TokensUsed
			d.DailyStats[i].CostUSD += summary.CostUSD
			totalDur := d.DailyStats[i].AvgDuration * time.Duration(d.DailyStats[i].Sessions-1)
			totalDur += summary.Duration
			d.DailyStats[i].AvgDuration = totalDur / time.Duration(d.DailyStats[i].Sessions)
			found = true
			break
		}
	}
	if !found {
		d.DailyStats = append(d.DailyStats, DayStat{
			Date:        dayKey,
			Sessions:    1,
			Tokens:      summary.TokensUsed,
			CostUSD:     summary.CostUSD,
			AvgDuration: summary.Duration,
		})
		d.ActiveDays++
	}
}

// RenderOverview produces a terminal-friendly overview of usage analytics.
func (d *Dashboard) RenderOverview() string {
	const width = 50
	var b strings.Builder

	topBorder := "+" + strings.Repeat("=", width) + "+"
	midBorder := "+" + strings.Repeat("-", width) + "+"
	botBorder := "+" + strings.Repeat("=", width) + "+"

	b.WriteString(topBorder + "\n")
	b.WriteString("|" + centerText("graycode Analytics Dashboard", width) + "|\n")
	b.WriteString(midBorder + "\n")

	// Summary stats
	totalSessions := len(d.Sessions)
	successCount := 0
	var totalDuration time.Duration
	for _, s := range d.Sessions {
		if s.Success {
			successCount++
		}
		totalDuration += s.Duration
	}
	var avgDuration time.Duration
	var successRate int
	if totalSessions > 0 {
		avgDuration = totalDuration / time.Duration(totalSessions)
		successRate = (successCount * 100) / totalSessions
	}

	b.WriteString(fmt.Sprintf("| Total Sessions: %-6d | Active Days: %-8d |\n", totalSessions, d.ActiveDays))
	b.WriteString(fmt.Sprintf("| Total Tokens: %-8s | Total Cost: $%-8.2f |\n", formatTokens(d.TotalTokens), d.TotalCostUSD))
	b.WriteString(fmt.Sprintf("| Avg Session: %-9s | Success Rate: %-5s |\n", formatDuration(avgDuration), fmt.Sprintf("%d%%", successRate)))
	b.WriteString(midBorder + "\n")

	// Last 7 days chart
	b.WriteString("| Last 7 Days:" + strings.Repeat(" ", width-len("| Last 7 Days:")+1) + "|\n")
	last7 := d.getLast7Days()
	maxSessions := 0
	for _, ds := range last7 {
		if ds.Sessions > maxSessions {
			maxSessions = ds.Sessions
		}
	}
	for _, ds := range last7 {
		dayName := ds.Date.Format("Mon")
		barLen := 0
		if maxSessions > 0 {
			barLen = (ds.Sessions * 20) / maxSessions
		}
		bar := strings.Repeat("#", barLen)
		sessWord := "sessions"
		if ds.Sessions == 1 {
			sessWord = "session"
		}
		line := fmt.Sprintf(" %s %-20s %d %s ($%.2f)", dayName, bar, ds.Sessions, sessWord, ds.CostUSD)
		if len(line) > width {
			line = line[:width]
		}
		b.WriteString("|" + padRight(line, width) + "|\n")
	}
	b.WriteString(midBorder + "\n")

	// Top models
	b.WriteString("| Top Models:" + strings.Repeat(" ", width-len("| Top Models:")+1) + "|\n")
	modelRanking := d.getModelRanking()
	for i, mr := range modelRanking {
		if i >= 3 {
			break
		}
		pct := 0
		if d.TotalTokens > 0 {
			pct = (mr.tokens * 100) / d.TotalTokens
		}
		line := fmt.Sprintf(" %d. %-22s %3d%% ($%.2f)", i+1, mr.name, pct, mr.cost)
		if len(line) > width {
			line = line[:width]
		}
		b.WriteString("|" + padRight(line, width) + "|\n")
	}
	b.WriteString(midBorder + "\n")

	// Top tools
	b.WriteString("| Top Tools:" + strings.Repeat(" ", width-len("| Top Tools:")+1) + "|\n")
	toolRanking := d.getToolRanking()
	var toolParts []string
	for i, tr := range toolRanking {
		if i >= 3 {
			break
		}
		toolParts = append(toolParts, fmt.Sprintf("%d. %s (%d)", i+1, tr.name, tr.count))
	}
	toolLine := " " + strings.Join(toolParts, "  ")
	if len(toolLine) > width {
		toolLine = toolLine[:width]
	}
	b.WriteString("|" + padRight(toolLine, width) + "|\n")
	b.WriteString(botBorder + "\n")

	return b.String()
}

// RenderCostChart renders an ASCII bar chart of daily costs for the given number of days.
func (d *Dashboard) RenderCostChart(days int) string {
	var b strings.Builder
	b.WriteString("Daily Cost Chart (last " + fmt.Sprintf("%d", days) + " days)\n")
	b.WriteString(strings.Repeat("-", 50) + "\n")

	dailyData := d.getDailyDataForDays(days)
	if len(dailyData) == 0 {
		b.WriteString("No data available.\n")
		return b.String()
	}

	maxCost := 0.0
	for _, ds := range dailyData {
		if ds.CostUSD > maxCost {
			maxCost = ds.CostUSD
		}
	}

	for _, ds := range dailyData {
		barLen := 0
		if maxCost > 0 {
			barLen = int((ds.CostUSD / maxCost) * 30)
		}
		bar := strings.Repeat("#", barLen)
		b.WriteString(fmt.Sprintf("%s | %-30s $%.2f\n", ds.Date.Format("01/02"), bar, ds.CostUSD))
	}
	b.WriteString(strings.Repeat("-", 50) + "\n")
	return b.String()
}

// RenderTokenChart renders an ASCII bar chart of daily token usage for the given number of days.
func (d *Dashboard) RenderTokenChart(days int) string {
	var b strings.Builder
	b.WriteString("Daily Token Usage (last " + fmt.Sprintf("%d", days) + " days)\n")
	b.WriteString(strings.Repeat("-", 50) + "\n")

	dailyData := d.getDailyDataForDays(days)
	if len(dailyData) == 0 {
		b.WriteString("No data available.\n")
		return b.String()
	}

	maxTokens := 0
	for _, ds := range dailyData {
		if ds.Tokens > maxTokens {
			maxTokens = ds.Tokens
		}
	}

	for _, ds := range dailyData {
		barLen := 0
		if maxTokens > 0 {
			barLen = (ds.Tokens * 30) / maxTokens
		}
		bar := strings.Repeat("#", barLen)
		b.WriteString(fmt.Sprintf("%s | %-30s %s\n", ds.Date.Format("01/02"), bar, formatTokens(ds.Tokens)))
	}
	b.WriteString(strings.Repeat("-", 50) + "\n")
	return b.String()
}

// RenderModelBreakdown renders a pie-chart-like breakdown with percentages for each model.
func (d *Dashboard) RenderModelBreakdown() string {
	var b strings.Builder
	b.WriteString("Model Usage Breakdown\n")
	b.WriteString(strings.Repeat("=", 50) + "\n")

	ranking := d.getModelRanking()
	if len(ranking) == 0 {
		b.WriteString("No model data available.\n")
		return b.String()
	}

	totalRequests := 0
	for _, mr := range ranking {
		totalRequests += mr.requests
	}

	for _, mr := range ranking {
		pct := 0
		if totalRequests > 0 {
			pct = (mr.requests * 100) / totalRequests
		}
		barLen := pct / 2
		bar := strings.Repeat("#", barLen)
		ms := d.ModelUsage[mr.name]
		b.WriteString(fmt.Sprintf("%-24s [%-50s] %3d%%\n", mr.name, bar, pct))
		b.WriteString(fmt.Sprintf("  Requests: %-6d Tokens: %-10s Cost: $%.2f\n", ms.Requests, formatTokens(ms.Tokens), ms.CostUSD))
		if ms.AvgLatencyMs > 0 {
			b.WriteString(fmt.Sprintf("  Avg Latency: %dms  Error Rate: %.1f%%\n", ms.AvgLatencyMs, ms.ErrorRate*100))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// RenderRecentSessions renders a table of the last n sessions.
func (d *Dashboard) RenderRecentSessions(n int) string {
	var b strings.Builder
	b.WriteString("Recent Sessions\n")
	b.WriteString(strings.Repeat("=", 90) + "\n")
	b.WriteString(fmt.Sprintf("%-12s %-20s %-8s %-10s %-8s %-8s %-7s\n",
		"ID", "Date", "Duration", "Model", "Tokens", "Cost", "Status"))
	b.WriteString(strings.Repeat("-", 90) + "\n")

	start := 0
	if len(d.Sessions) > n {
		start = len(d.Sessions) - n
	}
	sessions := d.Sessions[start:]

	// Display in reverse chronological order
	for i := len(sessions) - 1; i >= 0; i-- {
		s := sessions[i]
		status := "FAIL"
		if s.Success {
			status = "OK"
		}
		id := s.ID
		if len(id) > 10 {
			id = id[:10]
		}
		model := s.Model
		if len(model) > 18 {
			model = model[:18]
		}
		b.WriteString(fmt.Sprintf("%-12s %-20s %-8s %-10s %-8s $%-7.2f %-7s\n",
			id,
			s.Date.Format("2006-01-02 15:04"),
			formatDuration(s.Duration),
			model,
			formatTokens(s.TokensUsed),
			s.CostUSD,
			status))
	}
	b.WriteString(strings.Repeat("-", 90) + "\n")
	b.WriteString(fmt.Sprintf("Showing %d of %d sessions\n", len(sessions), len(d.Sessions)))
	return b.String()
}

// Export serializes the dashboard data to JSON.
func (d *Dashboard) Export() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}

// Import deserializes JSON data into the dashboard.
func (d *Dashboard) Import(data []byte) error {
	var imported Dashboard
	if err := json.Unmarshal(data, &imported); err != nil {
		return fmt.Errorf("importing dashboard data: %w", err)
	}
	d.Sessions = imported.Sessions
	d.DailyStats = imported.DailyStats
	d.ModelUsage = imported.ModelUsage
	d.ToolUsage = imported.ToolUsage
	d.TotalCostUSD = imported.TotalCostUSD
	d.TotalTokens = imported.TotalTokens
	d.ActiveDays = imported.ActiveDays
	return nil
}

// --- Helper types and functions ---

type modelRank struct {
	name     string
	tokens   int
	cost     float64
	requests int
}

type toolRank struct {
	name  string
	count int
}

func (d *Dashboard) getModelRanking() []modelRank {
	var ranking []modelRank
	for name, ms := range d.ModelUsage {
		ranking = append(ranking, modelRank{name: name, tokens: ms.Tokens, cost: ms.CostUSD, requests: ms.Requests})
	}
	sort.Slice(ranking, func(i, j int) bool { return ranking[i].tokens > ranking[j].tokens })
	return ranking
}

func (d *Dashboard) getToolRanking() []toolRank {
	var ranking []toolRank
	for name, count := range d.ToolUsage {
		ranking = append(ranking, toolRank{name: name, count: count})
	}
	sort.Slice(ranking, func(i, j int) bool { return ranking[i].count > ranking[j].count })
	return ranking
}

func (d *Dashboard) getLast7Days() []DayStat {
	now := time.Now().Truncate(24 * time.Hour)
	result := make([]DayStat, 7)
	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		result[6-i] = DayStat{Date: day}
		for _, ds := range d.DailyStats {
			if ds.Date.Equal(day) {
				result[6-i] = ds
				break
			}
		}
	}
	return result
}

func (d *Dashboard) getDailyDataForDays(days int) []DayStat {
	now := time.Now().Truncate(24 * time.Hour)
	cutoff := now.AddDate(0, 0, -(days - 1))
	var result []DayStat
	for _, ds := range d.DailyStats {
		if !ds.Date.Before(cutoff) && !ds.Date.After(now) {
			result = append(result, ds)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Date.Before(result[j].Date) })
	return result
}

func centerText(text string, width int) string {
	if len(text) >= width {
		return text[:width]
	}
	padding := (width - len(text)) / 2
	return strings.Repeat(" ", padding) + text + strings.Repeat(" ", width-padding-len(text))
}

func padRight(text string, width int) string {
	if len(text) >= width {
		return text[:width]
	}
	return text + strings.Repeat(" ", width-len(text))
}

func formatTokens(tokens int) string {
	switch {
	case tokens >= 1000000:
		return fmt.Sprintf("%.1fM", float64(tokens)/1000000)
	case tokens >= 1000:
		return fmt.Sprintf("%.1fk", float64(tokens)/1000)
	default:
		return fmt.Sprintf("%d", tokens)
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
