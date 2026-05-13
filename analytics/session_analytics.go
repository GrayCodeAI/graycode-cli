package analytics

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// SessionData holds metrics for a single agent session.
type SessionData struct {
	ID            string        `json:"id"`
	StartTime     time.Time     `json:"start_time"`
	Duration      time.Duration `json:"duration"`
	Model         string        `json:"model"`
	Provider      string        `json:"provider"`
	TokensIn      int           `json:"tokens_in"`
	TokensOut     int           `json:"tokens_out"`
	CostUSD       float64       `json:"cost_usd"`
	ToolCalls     int           `json:"tool_calls"`
	FilesModified int           `json:"files_modified"`
	TestsPassed   bool          `json:"tests_passed"`
	Success       bool          `json:"success"`
	TaskType      string        `json:"task_type"`
}

// SessionAnalytics tracks and reports on agent behavior patterns over time.
type SessionAnalytics struct {
	Sessions []SessionData
	mu       sync.RWMutex
}

// NewSessionAnalytics creates a new SessionAnalytics instance.
func NewSessionAnalytics() *SessionAnalytics {
	return &SessionAnalytics{
		Sessions: []SessionData{},
	}
}

// Record adds a session data entry to the analytics store.
func (sa *SessionAnalytics) Record(data SessionData) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.Sessions = append(sa.Sessions, data)
}

// DailyReport generates a formatted report for the given date.
func (sa *SessionAnalytics) DailyReport(date time.Time) string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	year, month, day := date.Date()
	var daySessions []SessionData
	for _, s := range sa.Sessions {
		sy, sm, sd := s.StartTime.Date()
		if sy == year && sm == month && sd == day {
			daySessions = append(daySessions, s)
		}
	}

	if len(daySessions) == 0 {
		return fmt.Sprintf("Daily Report: %s\n%s\nNo sessions recorded.", date.Format("2006-01-02"), strings.Repeat("─", 25))
	}

	var totalDuration time.Duration
	var totalTokensIn, totalTokensOut int
	var totalCost float64
	var successCount int
	modelCount := make(map[string]int)
	taskCount := make(map[string]int)

	for _, s := range daySessions {
		totalDuration += s.Duration
		totalTokensIn += s.TokensIn
		totalTokensOut += s.TokensOut
		totalCost += s.CostUSD
		if s.Success {
			successCount++
		}
		modelCount[s.Model]++
		if s.TaskType != "" {
			taskCount[s.TaskType]++
		}
	}

	successPct := float64(successCount) / float64(len(daySessions)) * 100

	topModel := topKey(modelCount)
	topTask := topKey(taskCount)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Daily Report: %s\n", date.Format("2006-01-02")))
	sb.WriteString(strings.Repeat("─", 25))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Sessions: %d | Duration: %s\n", len(daySessions), fmtSessionDuration(totalDuration)))
	sb.WriteString(fmt.Sprintf("Tokens: %s in, %s out\n", formatTokenCount(totalTokensIn), formatTokenCount(totalTokensOut)))
	sb.WriteString(fmt.Sprintf("Cost: $%.2f\n", totalCost))
	sb.WriteString(fmt.Sprintf("Success: %.0f%% (%d/%d)\n", successPct, successCount, len(daySessions)))
	if topModel != "" {
		sb.WriteString(fmt.Sprintf("Top model: %s (%d sessions)\n", topModel, modelCount[topModel]))
	}
	if topTask != "" {
		sb.WriteString(fmt.Sprintf("Top task: %s (%d sessions)\n", topTask, taskCount[topTask]))
	}

	return sb.String()
}

// WeeklyTrend shows daily stats for the last 7 days with a sparkline.
func (sa *SessionAnalytics) WeeklyTrend() string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	now := time.Now()
	sparkChars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	type dayStats struct {
		date     time.Time
		sessions int
		cost     float64
		success  int
	}

	days := make([]dayStats, 7)
	for i := 0; i < 7; i++ {
		d := now.AddDate(0, 0, -(6 - i))
		days[i].date = d
	}

	for _, s := range sa.Sessions {
		for i := range days {
			sy, sm, sd := s.StartTime.Date()
			dy, dm, dd := days[i].date.Date()
			if sy == dy && sm == dm && sd == dd {
				days[i].sessions++
				days[i].cost += s.CostUSD
				if s.Success {
					days[i].success++
				}
			}
		}
	}

	// Build sparkline for session counts
	maxSessions := 0
	for _, d := range days {
		if d.sessions > maxSessions {
			maxSessions = d.sessions
		}
	}

	var sparkline strings.Builder
	for _, d := range days {
		if maxSessions == 0 {
			sparkline.WriteRune(sparkChars[0])
		} else {
			idx := int(float64(d.sessions) / float64(maxSessions) * float64(len(sparkChars)-1))
			if idx >= len(sparkChars) {
				idx = len(sparkChars) - 1
			}
			sparkline.WriteRune(sparkChars[idx])
		}
	}

	var sb strings.Builder
	sb.WriteString("Weekly Trend (last 7 days)\n")
	sb.WriteString(strings.Repeat("─", 30))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Sessions: %s\n", sparkline.String()))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%-12s %8s %8s %8s\n", "Date", "Sessions", "Cost", "Success"))

	for _, d := range days {
		successPct := "-"
		if d.sessions > 0 {
			successPct = fmt.Sprintf("%.0f%%", float64(d.success)/float64(d.sessions)*100)
		}
		sb.WriteString(fmt.Sprintf("%-12s %8d %7s %8s\n",
			d.date.Format("Mon 01/02"),
			d.sessions,
			fmt.Sprintf("$%.2f", d.cost),
			successPct,
		))
	}

	return sb.String()
}

// ModelComparison compares success rate, speed, and cost across models used.
func (sa *SessionAnalytics) ModelComparison() string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	type modelStats struct {
		sessions    int
		successes   int
		totalDur    time.Duration
		totalCost   float64
		totalTokens int
	}

	models := make(map[string]*modelStats)
	for _, s := range sa.Sessions {
		if s.Model == "" {
			continue
		}
		ms, ok := models[s.Model]
		if !ok {
			ms = &modelStats{}
			models[s.Model] = ms
		}
		ms.sessions++
		if s.Success {
			ms.successes++
		}
		ms.totalDur += s.Duration
		ms.totalCost += s.CostUSD
		ms.totalTokens += s.TokensIn + s.TokensOut
	}

	if len(models) == 0 {
		return "No model data available."
	}

	// Sort models by session count descending
	type modelEntry struct {
		name  string
		stats *modelStats
	}
	var entries []modelEntry
	for name, stats := range models {
		entries = append(entries, modelEntry{name, stats})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].stats.sessions > entries[j].stats.sessions
	})

	var sb strings.Builder
	sb.WriteString("Model Comparison\n")
	sb.WriteString(strings.Repeat("─", 60))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%-25s %8s %10s %10s\n", "Model", "Success", "Avg Time", "Avg Cost"))
	sb.WriteString(strings.Repeat("─", 60))
	sb.WriteString("\n")

	for _, e := range entries {
		successRate := float64(e.stats.successes) / float64(e.stats.sessions) * 100
		avgDur := e.stats.totalDur / time.Duration(e.stats.sessions)
		avgCost := e.stats.totalCost / float64(e.stats.sessions)
		sb.WriteString(fmt.Sprintf("%-25s %7.0f%% %10s %9s\n",
			e.name,
			successRate,
			fmtSessionDuration(avgDur),
			fmt.Sprintf("$%.3f", avgCost),
		))
	}

	return sb.String()
}

// ProductivityScore returns a composite score (0-100) based on success rate,
// efficiency (tokens per file modified), and speed.
func (sa *SessionAnalytics) ProductivityScore() float64 {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	if len(sa.Sessions) == 0 {
		return 0
	}

	var successCount int
	var totalTokens int
	var totalFiles int
	var totalDuration time.Duration
	var sessionCount int

	for _, s := range sa.Sessions {
		sessionCount++
		if s.Success {
			successCount++
		}
		totalTokens += s.TokensIn + s.TokensOut
		totalFiles += s.FilesModified
		totalDuration += s.Duration
	}

	// Success rate component (0-40 points)
	successRate := float64(successCount) / float64(sessionCount)
	successScore := successRate * 40

	// Efficiency component (0-30 points): lower tokens per file is better
	var efficiencyScore float64
	if totalFiles > 0 {
		tokensPerFile := float64(totalTokens) / float64(totalFiles)
		// Baseline: 5000 tokens/file is average, below is good
		efficiencyScore = math.Max(0, math.Min(30, 30*(1-tokensPerFile/10000)))
	}

	// Speed component (0-30 points): faster sessions score higher
	var speedScore float64
	if sessionCount > 0 {
		avgMinutes := totalDuration.Minutes() / float64(sessionCount)
		// Baseline: 15 min average is good, longer penalizes
		speedScore = math.Max(0, math.Min(30, 30*(1-avgMinutes/30)))
	}

	score := successScore + efficiencyScore + speedScore
	return math.Round(score*100) / 100
}

// UsagePatterns returns human-readable insights about usage behavior.
func (sa *SessionAnalytics) UsagePatterns() []string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	if len(sa.Sessions) == 0 {
		return []string{"No usage data available."}
	}

	var patterns []string

	// Most active hour
	hourCount := make(map[int]int)
	for _, s := range sa.Sessions {
		hourCount[s.StartTime.Hour()]++
	}
	peakHour := 0
	peakCount := 0
	for h, c := range hourCount {
		if c > peakCount {
			peakHour = h
			peakCount = c
		}
	}
	endHour := (peakHour + 2) % 24
	patterns = append(patterns, fmt.Sprintf("Most active: %d-%dam", peakHour, endHour))

	// Average session duration
	var totalDur time.Duration
	for _, s := range sa.Sessions {
		totalDur += s.Duration
	}
	avgDur := totalDur / time.Duration(len(sa.Sessions))
	patterns = append(patterns, fmt.Sprintf("Avg session: %s", fmtSessionDurationShort(avgDur)))

	// Preferred model
	modelCount := make(map[string]int)
	for _, s := range sa.Sessions {
		if s.Model != "" {
			modelCount[s.Model]++
		}
	}
	if len(modelCount) > 0 {
		topModel := topKey(modelCount)
		pct := float64(modelCount[topModel]) / float64(len(sa.Sessions)) * 100
		// Extract short name from model
		shortName := shortModelName(topModel)
		patterns = append(patterns, fmt.Sprintf("Preferred model: %s (%.0f%%)", shortName, pct))
	}

	// Most common task type
	taskCount := make(map[string]int)
	for _, s := range sa.Sessions {
		if s.TaskType != "" {
			taskCount[s.TaskType]++
		}
	}
	if len(taskCount) > 0 {
		topTask := topKey(taskCount)
		patterns = append(patterns, fmt.Sprintf("Top task type: %s", topTask))
	}

	// Provider preference
	providerCount := make(map[string]int)
	for _, s := range sa.Sessions {
		if s.Provider != "" {
			providerCount[s.Provider]++
		}
	}
	if len(providerCount) > 1 {
		topProvider := topKey(providerCount)
		patterns = append(patterns, fmt.Sprintf("Primary provider: %s", topProvider))
	}

	return patterns
}

// CostProjection estimates cost for the given number of days ahead based on
// historical average daily spending.
func (sa *SessionAnalytics) CostProjection(daysAhead int) float64 {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	if len(sa.Sessions) == 0 || daysAhead <= 0 {
		return 0
	}

	var totalCost float64
	var earliest, latest time.Time
	for i, s := range sa.Sessions {
		totalCost += s.CostUSD
		if i == 0 || s.StartTime.Before(earliest) {
			earliest = s.StartTime
		}
		if i == 0 || s.StartTime.After(latest) {
			latest = s.StartTime
		}
	}

	// Calculate the span in days
	span := latest.Sub(earliest).Hours() / 24
	if span < 1 {
		span = 1
	}

	dailyAvg := totalCost / span
	return math.Round(dailyAvg*float64(daysAhead)*100) / 100
}

// FormatOverview returns a comprehensive overview string combining key metrics.
func (sa *SessionAnalytics) FormatOverview() string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	if len(sa.Sessions) == 0 {
		return "No session data available."
	}

	var totalCost float64
	var totalTokensIn, totalTokensOut int
	var totalDuration time.Duration
	var successCount int
	var totalToolCalls int
	var totalFiles int

	for _, s := range sa.Sessions {
		totalCost += s.CostUSD
		totalTokensIn += s.TokensIn
		totalTokensOut += s.TokensOut
		totalDuration += s.Duration
		totalToolCalls += s.ToolCalls
		totalFiles += s.FilesModified
		if s.Success {
			successCount++
		}
	}

	successRate := float64(successCount) / float64(len(sa.Sessions)) * 100
	avgDur := totalDuration / time.Duration(len(sa.Sessions))

	var sb strings.Builder
	sb.WriteString("Session Analytics Overview\n")
	sb.WriteString(strings.Repeat("═", 35))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Total sessions:  %d\n", len(sa.Sessions)))
	sb.WriteString(fmt.Sprintf("Total duration:  %s\n", fmtSessionDuration(totalDuration)))
	sb.WriteString(fmt.Sprintf("Avg duration:    %s\n", fmtSessionDuration(avgDur)))
	sb.WriteString(fmt.Sprintf("Success rate:    %.1f%%\n", successRate))
	sb.WriteString(fmt.Sprintf("Total cost:      $%.2f\n", totalCost))
	sb.WriteString(fmt.Sprintf("Total tokens:    %s in / %s out\n", formatTokenCount(totalTokensIn), formatTokenCount(totalTokensOut)))
	sb.WriteString(fmt.Sprintf("Tool calls:      %d\n", totalToolCalls))
	sb.WriteString(fmt.Sprintf("Files modified:  %d\n", totalFiles))
	sb.WriteString(fmt.Sprintf("Productivity:    %.1f/100\n", sa.productivityScoreUnlocked()))

	return sb.String()
}

// Export exports session data in the specified format ("csv" or "json").
func (sa *SessionAnalytics) Export(format string) string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	switch strings.ToLower(format) {
	case "json":
		data, err := json.MarshalIndent(sa.Sessions, "", "  ")
		if err != nil {
			return fmt.Sprintf(`{"error": "%s"}`, err.Error())
		}
		return string(data)
	case "csv":
		var sb strings.Builder
		w := csv.NewWriter(&sb)
		// Header
		_ = w.Write([]string{
			"id", "start_time", "duration_sec", "model", "provider",
			"tokens_in", "tokens_out", "cost_usd", "tool_calls",
			"files_modified", "tests_passed", "success", "task_type",
		})
		for _, s := range sa.Sessions {
			_ = w.Write([]string{
				s.ID,
				s.StartTime.Format(time.RFC3339),
				fmt.Sprintf("%.0f", s.Duration.Seconds()),
				s.Model,
				s.Provider,
				fmt.Sprintf("%d", s.TokensIn),
				fmt.Sprintf("%d", s.TokensOut),
				fmt.Sprintf("%.4f", s.CostUSD),
				fmt.Sprintf("%d", s.ToolCalls),
				fmt.Sprintf("%d", s.FilesModified),
				fmt.Sprintf("%t", s.TestsPassed),
				fmt.Sprintf("%t", s.Success),
				s.TaskType,
			})
		}
		w.Flush()
		return sb.String()
	default:
		return fmt.Sprintf("unsupported format: %s (use \"csv\" or \"json\")", format)
	}
}

// productivityScoreUnlocked calculates the score without locking (caller must hold lock).
func (sa *SessionAnalytics) productivityScoreUnlocked() float64 {
	if len(sa.Sessions) == 0 {
		return 0
	}

	var successCount int
	var totalTokens int
	var totalFiles int
	var totalDuration time.Duration
	var sessionCount int

	for _, s := range sa.Sessions {
		sessionCount++
		if s.Success {
			successCount++
		}
		totalTokens += s.TokensIn + s.TokensOut
		totalFiles += s.FilesModified
		totalDuration += s.Duration
	}

	successRate := float64(successCount) / float64(sessionCount)
	successScore := successRate * 40

	var efficiencyScore float64
	if totalFiles > 0 {
		tokensPerFile := float64(totalTokens) / float64(totalFiles)
		efficiencyScore = math.Max(0, math.Min(30, 30*(1-tokensPerFile/10000)))
	}

	var speedScore float64
	if sessionCount > 0 {
		avgMinutes := totalDuration.Minutes() / float64(sessionCount)
		speedScore = math.Max(0, math.Min(30, 30*(1-avgMinutes/30)))
	}

	score := successScore + efficiencyScore + speedScore
	return math.Round(score*100) / 100
}

// --- Helpers ---

func topKey(m map[string]int) string {
	var top string
	var maxCount int
	for k, v := range m {
		if v > maxCount {
			top = k
			maxCount = v
		}
	}
	return top
}

func fmtSessionDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func fmtSessionDurationShort(d time.Duration) string {
	minutes := int(d.Minutes())
	if minutes < 1 {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	return fmt.Sprintf("%d minutes", minutes)
}

func formatTokenCount(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%dK", n/1000)
	}
	return fmt.Sprintf("%d", n)
}

func shortModelName(model string) string {
	// Extract a friendly short name from model identifiers like "claude-sonnet-4-6"
	parts := strings.Split(model, "-")
	for _, p := range parts {
		switch p {
		case "sonnet", "opus", "haiku", "gpt", "gemini":
			return p
		}
	}
	// Fallback: return last meaningful segment
	if len(parts) > 1 {
		return parts[len(parts)-2]
	}
	return model
}
