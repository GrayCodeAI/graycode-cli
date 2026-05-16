package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/analytics"
	"github.com/spf13/cobra"
)

var (
	statsDays   int
	statsTop    int
	statsFormat string
	statsModels bool
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show usage statistics and cost analytics",
	Long: `Display aggregated usage statistics from session traces including
session counts, message totals, cost breakdowns, model usage, and tool
call frequency over a configurable time window.`,
	RunE: runStats,
}

func init() {
	statsCmd.Flags().IntVar(&statsDays, "days", 30, "number of days to include in statistics")
	statsCmd.Flags().IntVar(&statsTop, "top", 10, "number of top tools to display")
	statsCmd.Flags().StringVar(&statsFormat, "format", "text", "output format: text or json")
	statsCmd.Flags().BoolVar(&statsModels, "models", false, "show per-model breakdown")
	rootCmd.AddCommand(statsCmd)
}

type statsOutput struct {
	Period        string                `json:"period"`
	TotalSessions int                  `json:"total_sessions"`
	TotalMessages int                  `json:"total_messages"`
	TotalToolCalls int                 `json:"total_tool_calls"`
	TotalCostUSD  float64             `json:"total_cost_usd"`
	ActiveDays    int                  `json:"active_days"`
	AvgCostPerSession float64         `json:"avg_cost_per_session"`
	AvgCostPerDay     float64         `json:"avg_cost_per_day"`
	Models        map[string]modelStat `json:"models,omitempty"`
	TopTools      []toolStat           `json:"top_tools,omitempty"`
	DailyBreakdown []dayStat           `json:"daily_breakdown,omitempty"`
}

type modelStat struct {
	Requests int     `json:"requests"`
	Messages int     `json:"messages"`
	CostUSD  float64 `json:"cost_usd"`
}

type toolStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type dayStat struct {
	Date     string  `json:"date"`
	Sessions int     `json:"sessions"`
	Cost     float64 `json:"cost"`
}

func runStats(cmd *cobra.Command, args []string) error {
	traces, err := analytics.GetTraces()
	if err != nil {
		return fmt.Errorf("loading traces: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -statsDays)

	// Filter traces by date
	var filtered []*analytics.SessionTrace
	for _, t := range traces {
		if t.StartTime.After(cutoff) {
			filtered = append(filtered, t)
		}
	}

	if len(filtered) == 0 {
		cmd.Println("No session data found for the specified time period.")
		cmd.Println("Sessions are recorded automatically when you use hawk.")
		return nil
	}

	// Aggregate statistics
	output := aggregateStats(filtered, statsDays)

	if statsFormat == "json" {
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		cmd.Println(string(data))
		return nil
	}

	// Text output
	printStatsText(cmd, output)
	return nil
}

func aggregateStats(traces []*analytics.SessionTrace, days int) *statsOutput {
	out := &statsOutput{
		Period:        fmt.Sprintf("last %d days", days),
		TotalSessions: len(traces),
		Models:        make(map[string]modelStat),
	}

	toolCounts := make(map[string]int)
	dailyMap := make(map[string]*dayStat)
	activeDays := make(map[string]bool)

	for _, t := range traces {
		out.TotalMessages += t.MessageCount
		out.TotalToolCalls += t.ToolCalls
		out.TotalCostUSD += t.CostUSD

		// Model breakdown
		ms := out.Models[t.Model]
		ms.Requests++
		ms.Messages += t.MessageCount
		ms.CostUSD += t.CostUSD
		out.Models[t.Model] = ms

		// Tool calls are aggregated as a single count per session
		// (individual tool names not available in traces, tracked as total)
		if t.ToolCalls > 0 {
			toolCounts[t.Model+"/tools"] += t.ToolCalls
		}

		// Daily breakdown
		dayKey := t.StartTime.Format("2006-01-02")
		activeDays[dayKey] = true
		ds, ok := dailyMap[dayKey]
		if !ok {
			ds = &dayStat{Date: dayKey}
			dailyMap[dayKey] = ds
		}
		ds.Sessions++
		ds.Cost += t.CostUSD
	}

	out.ActiveDays = len(activeDays)

	if out.TotalSessions > 0 {
		out.AvgCostPerSession = out.TotalCostUSD / float64(out.TotalSessions)
	}
	if out.ActiveDays > 0 {
		out.AvgCostPerDay = out.TotalCostUSD / float64(out.ActiveDays)
	}

	// Sort tools by count
	for name, count := range toolCounts {
		out.TopTools = append(out.TopTools, toolStat{Name: name, Count: count})
	}
	sort.Slice(out.TopTools, func(i, j int) bool {
		return out.TopTools[i].Count > out.TopTools[j].Count
	})

	// Sort daily breakdown
	for _, ds := range dailyMap {
		out.DailyBreakdown = append(out.DailyBreakdown, *ds)
	}
	sort.Slice(out.DailyBreakdown, func(i, j int) bool {
		return out.DailyBreakdown[i].Date < out.DailyBreakdown[j].Date
	})

	return out
}

func printStatsText(cmd *cobra.Command, out *statsOutput) {
	w := cmd.OutOrStdout()

		_, _ = fmt.Fprintf(w, "\n")
		_, _ = fmt.Fprintf(w, "══════════════════════════════════════════════════\n")
		_, _ = fmt.Fprintf(w, "  Hawk Usage Statistics (%s)\n", out.Period)
		_, _ = fmt.Fprintf(w, "══════════════════════════════════════════════════\n")

	// Overview section
		_, _ = fmt.Fprintf(w, "\n")
		_, _ = fmt.Fprintf(w, "─── Overview ───\n")
		_, _ = fmt.Fprintf(w, "  Sessions:     %d\n", out.TotalSessions)
		_, _ = fmt.Fprintf(w, "  Messages:     %d\n", out.TotalMessages)
		_, _ = fmt.Fprintf(w, "  Tool calls:   %d\n", out.TotalToolCalls)
		_, _ = fmt.Fprintf(w, "  Active days:  %d\n", out.ActiveDays)

	// Cost section
		_, _ = fmt.Fprintf(w, "\n")
		_, _ = fmt.Fprintf(w, "─── Cost ───\n")
		_, _ = fmt.Fprintf(w, "  Total cost:       $%.4f\n", out.TotalCostUSD)
		_, _ = fmt.Fprintf(w, "  Avg cost/session: $%.4f\n", out.AvgCostPerSession)
		_, _ = fmt.Fprintf(w, "  Avg cost/day:     $%.4f\n", out.AvgCostPerDay)

	// Models section
	if statsModels && len(out.Models) > 0 {
		_, _ = fmt.Fprintf(w, "\n")
		_, _ = fmt.Fprintf(w, "─── Models ───\n")
		_, _ = fmt.Fprintf(w, "  %-30s %8s %10s\n", "MODEL", "REQUESTS", "COST")
		_, _ = fmt.Fprintf(w, "  %-30s %8s %10s\n", strings.Repeat("─", 30), strings.Repeat("─", 8), strings.Repeat("─", 10))

		// Sort models by cost descending
		type modelEntry struct {
			name string
			stat modelStat
		}
		var models []modelEntry
		for name, stat := range out.Models {
			models = append(models, modelEntry{name, stat})
		}
		sort.Slice(models, func(i, j int) bool {
			return models[i].stat.CostUSD > models[j].stat.CostUSD
		})

		for _, m := range models {
		_, _ = fmt.Fprintf(w, "  %-30s %8d %10s\n", m.name, m.stat.Requests, fmt.Sprintf("$%.4f", m.stat.CostUSD))
		}
	}

	// Top Tools section
	if len(out.TopTools) > 0 {
		_, _ = fmt.Fprintf(w, "\n")
		_, _ = fmt.Fprintf(w, "─── Top Tools ───\n")

		limit := statsTop
		if limit > len(out.TopTools) {
			limit = len(out.TopTools)
		}

		// Find max count for bar scaling
		maxCount := 0
		for i := 0; i < limit; i++ {
			if out.TopTools[i].Count > maxCount {
				maxCount = out.TopTools[i].Count
			}
		}

		barWidth := 30
		for i := 0; i < limit; i++ {
			t := out.TopTools[i]
			barLen := int(math.Round(float64(t.Count) / float64(maxCount) * float64(barWidth)))
			if barLen < 1 {
				barLen = 1
			}
			bar := strings.Repeat("█", barLen)
		_, _ = fmt.Fprintf(w, "  %-20s %s %d\n", t.Name, bar, t.Count)
		}
	}

		_, _ = fmt.Fprintf(w, "\n")
}
