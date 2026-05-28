package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/hooks/audit"
	"github.com/spf13/cobra"
)

var (
	auditDays    int
	auditFormat  string
	auditLimit   int
	auditProject string
	auditJSON    bool
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Analyze past sessions for wasteful patterns",
	Long: `Scan past agent transcripts to detect wasteful behaviors like
redundant cd commands, unnecessary cat/head usage, long sleep loops,
and other patterns that waste tokens and wall-clock time.

Reports what hawk would have caught with current policies enabled,
plus audit-only detectors that identify optimization opportunities.`,
	RunE: runAudit,
}

func init() {
	auditCmd.Flags().IntVar(&auditDays, "days", 7, "number of days to scan")
	auditCmd.Flags().StringVar(&auditFormat, "format", "text", "output format: text or json")
	auditCmd.Flags().IntVar(&auditLimit, "limit", 20, "max rows in table output")
	auditCmd.Flags().StringVar(&auditProject, "project", "", "restrict to specific project path")
	auditCmd.Flags().BoolVar(&auditJSON, "json", false, "output as JSON (alias for --format json)")
	rootCmd.AddCommand(auditCmd)
}

// AuditCount represents aggregated hits for one detector.
type AuditCount struct {
	Name      string   `json:"name"`
	Category  string   `json:"category"`
	Severity  string   `json:"severity"`
	Hits      int      `json:"hits"`
	Projects  int      `json:"projects"`
	Examples  []string `json:"examples"`
	FirstSeen string   `json:"first_seen,omitempty"`
	LastSeen  string   `json:"last_seen,omitempty"`
}

// AuditResult is the top-level audit output.
type AuditResult struct {
	Version   int          `json:"version"`
	ScannedAt string       `json:"scanned_at"`
	Days      int          `json:"days"`
	Sessions  int          `json:"sessions_scanned"`
	TotalHits int          `json:"total_hits"`
	Detectors []AuditCount `json:"detectors"`
}

func runAudit(cmd *cobra.Command, args []string) error {
	if auditJSON {
		auditFormat = "json"
	}

	// Discover session transcripts
	sessions, err := discoverSessions(auditDays, auditProject)
	if err != nil {
		return fmt.Errorf("discovering sessions: %w", err)
	}

	if len(sessions) == 0 {
		cmd.Println("No session transcripts found for the specified time period.")
		return nil
	}

	// Run audit detectors on each session
	detectors := audit.AllDetectors()
	counts := make(map[string]*AuditCount)
	totalHits := 0

	for _, sess := range sessions {
		events, err := loadSessionEvents(sess.Path)
		if err != nil {
			continue
		}

		state := make(audit.DetectorSessionState)

		for _, event := range events {
			for _, d := range detectors {
				hit := d.Detect(event, state)
				if hit == nil {
					continue
				}

				totalHits++
				key := hit.DetectorName
				if counts[key] == nil {
					counts[key] = &AuditCount{
						Name:     hit.DetectorName,
						Category: hit.Category,
						Severity: hit.Severity,
					}
				}
				c := counts[key]
				c.Hits++
				if len(c.Examples) < 3 {
					c.Examples = append(c.Examples, truncateStr(hit.Example, 80))
				}
				// Track timestamps
				ts := event.Timestamp.Format(time.RFC3339)
				if c.FirstSeen == "" || ts < c.FirstSeen {
					c.FirstSeen = ts
				}
				if c.LastSeen == "" || ts > c.LastSeen {
					c.LastSeen = ts
				}
			}
		}
	}

	// Count unique projects per detector
	for _, c := range counts {
		c.Projects = 1 // simplified — could track unique cwds
	}

	// Sort by hits descending
	var sorted []AuditCount
	for _, c := range counts {
		sorted = append(sorted, *c)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Hits > sorted[j].Hits
	})

	// Apply limit
	if auditLimit > 0 && len(sorted) > auditLimit {
		sorted = sorted[:auditLimit]
	}

	result := AuditResult{
		Version:   1,
		ScannedAt: time.Now().Format(time.RFC3339),
		Days:      auditDays,
		Sessions:  len(sessions),
		TotalHits: totalHits,
		Detectors: sorted,
	}

	if auditFormat == "json" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		cmd.Println(string(data))
		return nil
	}

	printAuditText(cmd, result)
	return nil
}

// SessionInfo represents a discovered session transcript.
type SessionInfo struct {
	Path      string
	CWD       string
	StartTime time.Time
}

func discoverSessions(days int, projectFilter string) ([]SessionInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	var sessions []SessionInfo

	// Scan hawk sessions directory
	hawkDir := filepath.Join(home, ".hawk", "sessions")
	entries, err := os.ReadDir(hawkDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(hawkDir, e.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			continue
		}
		sessions = append(sessions, SessionInfo{
			Path:      path,
			StartTime: info.ModTime(),
		})
	}

	return sessions, nil
}

func loadSessionEvents(path string) ([]audit.ToolEvent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var events []audit.ToolEvent
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry struct {
			Type      string                 `json:"type"`
			Timestamp string                 `json:"timestamp"`
			Tool      string                 `json:"tool"`
			Input     map[string]interface{} `json:"input"`
			CWD       string                 `json:"cwd"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// Only process tool_use events
		if entry.Type != "tool_use" && entry.Tool == "" {
			continue
		}

		ts, _ := time.Parse(time.RFC3339, entry.Timestamp)
		events = append(events, audit.ToolEvent{
			ToolName:  entry.Tool,
			ToolInput: entry.Input,
			CWD:       entry.CWD,
			Timestamp: ts,
		})
	}

	return events, nil
}

func printAuditText(cmd *cobra.Command, result AuditResult) {
	w := cmd.OutOrStdout()

	_, _ = fmt.Fprintf(w, "\n")
	_, _ = fmt.Fprintf(w, "═══════════════════════════════════════════════════════════════\n")
	_, _ = fmt.Fprintf(w, "  Hawk Audit Report\n")
	_, _ = fmt.Fprintf(w, "═══════════════════════════════════════════════════════════════\n")
	_, _ = fmt.Fprintf(w, "\n")
	_, _ = fmt.Fprintf(w, "  Scanned:     %d sessions (last %d days)\n", result.Sessions, result.Days)
	_, _ = fmt.Fprintf(w, "  Total hits:  %d\n", result.TotalHits)
	_, _ = fmt.Fprintf(w, "  Scanned at:  %s\n", result.ScannedAt)

	if len(result.Detectors) == 0 {
		_, _ = fmt.Fprintf(w, "\n  No wasteful patterns detected. Great job!\n\n")
		return
	}

	_, _ = fmt.Fprintf(w, "\n")
	_, _ = fmt.Fprintf(w, "─── Detected Patterns ───\n\n")
	_, _ = fmt.Fprintf(w, "  %-30s %6s %8s  %s\n", "DETECTOR", "HITS", "SEVERITY", "EXAMPLE")
	_, _ = fmt.Fprintf(w, "  %-30s %6s %8s  %s\n", strings.Repeat("─", 30), strings.Repeat("─", 6), strings.Repeat("─", 8), strings.Repeat("─", 30))

	for _, d := range result.Detectors {
		example := ""
		if len(d.Examples) > 0 {
			example = d.Examples[0]
		}
		_, _ = fmt.Fprintf(w, "  %-30s %6d %8s  %s\n", d.Name, d.Hits, d.Severity, example)
	}

	_, _ = fmt.Fprintf(w, "\n")
	_, _ = fmt.Fprintf(w, "─── Remediation Tips ───\n\n")

	for _, d := range result.Detectors {
		switch d.Name {
		case "redundant-cd-cwd":
			_, _ = fmt.Fprintf(w, "  • Remove `cd <cwd> &&` prefixes — commands already run in cwd\n")
		case "prefer-edit-over-read-cat":
			_, _ = fmt.Fprintf(w, "  • Use the Read tool instead of `cat`/`head`/`tail` on source files\n")
		case "prefer-edit-over-sed-awk":
			_, _ = fmt.Fprintf(w, "  • Use the Edit tool instead of `sed`/`awk` on source files\n")
		case "prefer-write-over-heredoc":
			_, _ = fmt.Fprintf(w, "  • Use the Write tool instead of `cat << EOF > file`\n")
		case "sleep-polling-loop":
			_, _ = fmt.Fprintf(w, "  • Replace long sleep/polling with explicit wait signals\n")
		case "find-from-root":
			_, _ = fmt.Fprintf(w, "  • Scope `find` to the project directory, not `/` or `/home`\n")
		case "git-commit-no-verify":
			_, _ = fmt.Fprintf(w, "  • Remove `--no-verify` to let pre-commit hooks run\n")
		case "reread-after-edit":
			_, _ = fmt.Fprintf(w, "  • Don't re-read files immediately after editing them\n")
		}
	}

	_, _ = fmt.Fprintf(w, "\n")
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
