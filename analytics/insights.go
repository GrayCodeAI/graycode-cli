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

// InsightsFacet represents an extracted insight category.
type InsightsFacet struct {
	Category string   `json:"category"` // goals, outcomes, friction, success
	Items    []string `json:"items"`
}

// InsightsReport holds a complete cross-session analysis.
type InsightsReport struct {
	GeneratedAt     time.Time        `json:"generated_at"`
	SessionsScanned int              `json:"sessions_scanned"`
	DateRange       DateRange        `json:"date_range"`
	Facets          []InsightsFacet  `json:"facets"`
	TopPatterns     []string         `json:"top_patterns"`
	Recommendations []string         `json:"recommendations"`
	Stats           *SessionStats    `json:"stats,omitempty"`
}

// GenerateInsights analyzes session transcripts to extract patterns and recommendations.
func GenerateInsights(days int, analysisFn func(content string) ([]InsightsFacet, error)) (*InsightsReport, error) {
	sessDir := sessionsDir()
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		return nil, fmt.Errorf("reading sessions: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	var transcripts []string
	scanned := 0

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			continue
		}
		if filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(sessDir, e.Name()))
		if err != nil {
			continue
		}
		transcripts = append(transcripts, string(data))
		scanned++
	}

	if scanned == 0 {
		return nil, fmt.Errorf("no sessions found in the last %d days", days)
	}

	// Extract facets via LLM analysis if provided
	var facets []InsightsFacet
	if analysisFn != nil {
		combined := strings.Join(transcripts, "\n---SESSION BREAK---\n")
		if len(combined) > 50000 {
			combined = combined[:50000]
		}
		facets, err = analysisFn(combined)
		if err != nil {
			// Non-fatal: continue with stats-only report
			facets = nil
		}
	}

	// Compute patterns from tool usage
	patterns := extractPatterns(transcripts)

	report := &InsightsReport{
		GeneratedAt:     time.Now(),
		SessionsScanned: scanned,
		DateRange:       DateRange{Start: cutoff, End: time.Now()},
		Facets:          facets,
		TopPatterns:     patterns,
		Recommendations: generateRecommendations(patterns, scanned),
	}

	return report, nil
}

// ExportInsightsHTML generates an HTML report from insights.
func ExportInsightsHTML(report *InsightsReport) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><title>Hawk Insights</title>
<style>
body { font-family: -apple-system, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
h1 { color: #333; } h2 { color: #555; border-bottom: 1px solid #eee; }
.stat { display: inline-block; margin: 10px; padding: 15px; background: #f5f5f5; border-radius: 8px; }
.pattern { padding: 8px; margin: 4px 0; background: #e8f4fd; border-radius: 4px; }
.rec { padding: 8px; margin: 4px 0; background: #e8fde8; border-radius: 4px; }
</style></head><body>`)

	b.WriteString(fmt.Sprintf("<h1>Hawk Insights Report</h1><p>Generated: %s | Sessions: %d</p>",
		report.GeneratedAt.Format("2006-01-02 15:04"), report.SessionsScanned))

	if len(report.TopPatterns) > 0 {
		b.WriteString("<h2>Top Patterns</h2>")
		for _, p := range report.TopPatterns {
			b.WriteString(fmt.Sprintf(`<div class="pattern">%s</div>`, p))
		}
	}

	if len(report.Facets) > 0 {
		for _, f := range report.Facets {
			b.WriteString(fmt.Sprintf("<h2>%s</h2><ul>", f.Category))
			for _, item := range f.Items {
				b.WriteString(fmt.Sprintf("<li>%s</li>", item))
			}
			b.WriteString("</ul>")
		}
	}

	if len(report.Recommendations) > 0 {
		b.WriteString("<h2>Recommendations</h2>")
		for _, r := range report.Recommendations {
			b.WriteString(fmt.Sprintf(`<div class="rec">%s</div>`, r))
		}
	}

	b.WriteString("</body></html>")
	return b.String()
}

// SaveInsightsReport saves an HTML report to disk.
func SaveInsightsReport(report *InsightsReport) (string, error) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".hawk", "insights")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("insights_%s.html", report.GeneratedAt.Format("2006-01-02"))
	path := filepath.Join(dir, filename)

	html := ExportInsightsHTML(report)
	return path, os.WriteFile(path, []byte(html), 0o644)
}

func extractPatterns(transcripts []string) []string {
	toolFreq := make(map[string]int)
	for _, t := range transcripts {
		for _, line := range strings.Split(t, "\n") {
			var event map[string]interface{}
			if json.Unmarshal([]byte(line), &event) != nil {
				continue
			}
			if tool, ok := event["tool"].(string); ok {
				toolFreq[tool]++
			}
		}
	}

	type kv struct {
		k string
		v int
	}
	var sorted []kv
	for k, v := range toolFreq {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })

	var patterns []string
	for i, item := range sorted {
		if i >= 5 {
			break
		}
		patterns = append(patterns, fmt.Sprintf("Heavy %s usage (%d calls)", item.k, item.v))
	}
	return patterns
}

func generateRecommendations(patterns []string, sessionCount int) []string {
	var recs []string
	if sessionCount > 20 {
		recs = append(recs, "Consider using /compact more frequently to keep context fresh")
	}
	for _, p := range patterns {
		if strings.Contains(p, "Read") {
			recs = append(recs, "High file read count suggests exploring unfamiliar code — consider using /init to generate documentation")
		}
		if strings.Contains(p, "Bash") {
			recs = append(recs, "Frequent shell usage — consider adding common commands to allowed permissions to reduce prompts")
		}
	}
	return recs
}

func sessionsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "sessions")
}
