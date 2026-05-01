package analytics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionStats holds aggregated statistics for a user's sessions.
type SessionStats struct {
	TotalSessions   int                    `json:"total_sessions"`
	TotalMessages   int                    `json:"total_messages"`
	TotalDuration   time.Duration          `json:"total_duration"`
	TotalTokens     int64                  `json:"total_tokens"`
	TotalCost       float64                `json:"total_cost"`
	ToolUsage       map[string]int         `json:"tool_usage"`
	ModelUsage      map[string]ModelStats  `json:"model_usage"`
	LanguageStats   map[string]int         `json:"language_stats"`
	GitCommits      int                    `json:"git_commits"`
	ActivityHeatmap [7][24]int             `json:"activity_heatmap"` // [weekday][hour]
	PeakDay         time.Weekday           `json:"peak_day"`
	PeakHour        int                    `json:"peak_hour"`
	DateRange       DateRange              `json:"date_range"`
}

// ModelStats tracks usage per model.
type ModelStats struct {
	Model       string  `json:"model"`
	Tokens      int64   `json:"tokens"`
	Requests    int     `json:"requests"`
	AvgLatency  float64 `json:"avg_latency_ms"`
	Cost        float64 `json:"cost"`
}

// DateRange represents the time span of statistics.
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ComputeStats aggregates statistics from session event logs.
func ComputeStats(days int) (*SessionStats, error) {
	logDir := eventLogDir()
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, fmt.Errorf("reading event logs: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	stats := &SessionStats{
		ToolUsage:     make(map[string]int),
		ModelUsage:    make(map[string]ModelStats),
		LanguageStats: make(map[string]int),
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(logDir, e.Name()))
		if err != nil {
			continue
		}

		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			stats.processEvent(event)
		}
	}

	stats.computePeaks()
	stats.DateRange = DateRange{Start: cutoff, End: time.Now()}
	return stats, nil
}

func (s *SessionStats) processEvent(event map[string]interface{}) {
	eventType, _ := event["type"].(string)

	switch eventType {
	case "session_start":
		s.TotalSessions++
	case "message":
		s.TotalMessages++
	case "tool_use":
		if tool, ok := event["tool"].(string); ok {
			s.ToolUsage[tool]++
		}
	case "api_request":
		if model, ok := event["model"].(string); ok {
			ms := s.ModelUsage[model]
			ms.Model = model
			ms.Requests++
			if tokens, ok := event["tokens"].(float64); ok {
				ms.Tokens += int64(tokens)
				s.TotalTokens += int64(tokens)
			}
			if cost, ok := event["cost"].(float64); ok {
				ms.Cost += cost
				s.TotalCost += cost
			}
			s.ModelUsage[model] = ms
		}
	case "git_commit":
		s.GitCommits++
	}

	// Update heatmap from timestamp
	if ts, ok := event["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			s.ActivityHeatmap[t.Weekday()][t.Hour()]++
		}
	}
}

func (s *SessionStats) computePeaks() {
	maxDay := 0
	for day := 0; day < 7; day++ {
		total := 0
		for hour := 0; hour < 24; hour++ {
			total += s.ActivityHeatmap[day][hour]
		}
		if total > maxDay {
			maxDay = total
			s.PeakDay = time.Weekday(day)
		}
	}

	maxHour := 0
	for hour := 0; hour < 24; hour++ {
		total := 0
		for day := 0; day < 7; day++ {
			total += s.ActivityHeatmap[day][hour]
		}
		if total > maxHour {
			maxHour = total
			s.PeakHour = hour
		}
	}
}

// FormatStats produces a human-readable stats report.
func FormatStats(s *SessionStats) string {
	var b strings.Builder

	b.WriteString("═══ Usage Statistics ═══\n\n")
	b.WriteString(fmt.Sprintf("  Sessions:    %d\n", s.TotalSessions))
	b.WriteString(fmt.Sprintf("  Messages:    %d\n", s.TotalMessages))
	b.WriteString(fmt.Sprintf("  Tokens:      %dk\n", s.TotalTokens/1000))
	b.WriteString(fmt.Sprintf("  Cost:        $%.2f\n", s.TotalCost))
	b.WriteString(fmt.Sprintf("  Git commits: %d\n", s.GitCommits))
	b.WriteString(fmt.Sprintf("  Peak day:    %s\n", s.PeakDay))
	b.WriteString(fmt.Sprintf("  Peak hour:   %d:00\n", s.PeakHour))

	// Top tools
	b.WriteString("\n─── Top Tools ───\n")
	type toolCount struct {
		name  string
		count int
	}
	var tools []toolCount
	for name, count := range s.ToolUsage {
		tools = append(tools, toolCount{name, count})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].count > tools[j].count })
	for i, tc := range tools {
		if i >= 10 {
			break
		}
		b.WriteString(fmt.Sprintf("  %-15s %d\n", tc.name, tc.count))
	}

	// Model usage
	b.WriteString("\n─── Models ───\n")
	for _, ms := range s.ModelUsage {
		b.WriteString(fmt.Sprintf("  %-30s %5d reqs  %6dk tokens  $%.2f\n",
			ms.Model, ms.Requests, ms.Tokens/1000, ms.Cost))
	}

	return b.String()
}

func eventLogDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "events")
}
