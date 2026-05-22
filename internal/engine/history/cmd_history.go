package history

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// CommandRecord represents a single command execution with metadata.
type CommandRecord struct {
	Command   string        `json:"command"`
	ExitCode  int           `json:"exit_code"`
	Duration  time.Duration `json:"duration"`
	Output    string        `json:"output"`
	Timestamp time.Time     `json:"timestamp"`
	SessionID string        `json:"session_id"`
	WorkDir   string        `json:"work_dir"`
}

// CommandFrequency tracks usage statistics for a command.
type CommandFrequency struct {
	Command     string        `json:"command"`
	Count       int           `json:"count"`
	AvgDuration time.Duration `json:"avg_duration"`
	FailRate    float64       `json:"fail_rate"`
}

// AliasSuggestion proposes an alias for a frequently used command.
type AliasSuggestion struct {
	Command string `json:"command"`
	Alias   string `json:"alias"`
	Count   int    `json:"count"`
	Reason  string `json:"reason"`
}

// CommandHistory tracks shell commands executed during sessions and provides insights.
type CommandHistory struct {
	Commands []CommandRecord `json:"commands"`
	Patterns map[string]int  `json:"patterns"`
	Failures map[string]int  `json:"failures"`
	mu       sync.RWMutex
}

// NewCommandHistory creates a new CommandHistory instance.
func NewCommandHistory() *CommandHistory {
	return &CommandHistory{
		Commands: make([]CommandRecord, 0),
		Patterns: make(map[string]int),
		Failures: make(map[string]int),
	}
}

// Record adds a command execution to history.
func (ch *CommandHistory) Record(cmd string, exitCode int, duration time.Duration, output string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	// Truncate output to 500 characters
	truncated := output
	if len(truncated) > 500 {
		truncated = truncated[:500] + "... (truncated)"
	}

	record := CommandRecord{
		Command:   cmd,
		ExitCode:  exitCode,
		Duration:  duration,
		Output:    truncated,
		Timestamp: time.Now(),
	}

	ch.Commands = append(ch.Commands, record)

	// Track patterns (command base)
	base := extractBaseCommand(cmd)
	ch.Patterns[base]++

	// Track failures
	if exitCode != 0 {
		ch.Failures[cmd]++
	}
}

// GetFrequent returns the most frequently used commands up to limit.
func (ch *CommandHistory) GetFrequent(limit int) []CommandFrequency {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	// Aggregate by command
	type cmdStats struct {
		count     int
		totalDur  time.Duration
		failCount int
	}
	stats := make(map[string]*cmdStats)

	for _, rec := range ch.Commands {
		s, ok := stats[rec.Command]
		if !ok {
			s = &cmdStats{}
			stats[rec.Command] = s
		}
		s.count++
		s.totalDur += rec.Duration
		if rec.ExitCode != 0 {
			s.failCount++
		}
	}

	// Convert to slice
	freqs := make([]CommandFrequency, 0, len(stats))
	for cmd, s := range stats {
		avgDur := time.Duration(0)
		if s.count > 0 {
			avgDur = s.totalDur / time.Duration(s.count)
		}
		failRate := 0.0
		if s.count > 0 {
			failRate = float64(s.failCount) / float64(s.count)
		}
		freqs = append(freqs, CommandFrequency{
			Command:     cmd,
			Count:       s.count,
			AvgDuration: avgDur,
			FailRate:    failRate,
		})
	}

	// Sort by count descending
	sort.Slice(freqs, func(i, j int) bool {
		return freqs[i].Count > freqs[j].Count
	})

	if limit > 0 && limit < len(freqs) {
		freqs = freqs[:limit]
	}

	return freqs
}

// GetFailing returns commands that frequently fail (exit code != 0).
func (ch *CommandHistory) GetFailing() []CommandRecord {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	var failing []CommandRecord
	for _, rec := range ch.Commands {
		if rec.ExitCode != 0 {
			failing = append(failing, rec)
		}
	}

	// Sort by timestamp descending (most recent first)
	sort.Slice(failing, func(i, j int) bool {
		return failing[i].Timestamp.After(failing[j].Timestamp)
	})

	return failing
}

// GetSlow returns commands that took longer than the given threshold.
func (ch *CommandHistory) GetSlow(threshold time.Duration) []CommandRecord {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	var slow []CommandRecord
	for _, rec := range ch.Commands {
		if rec.Duration > threshold {
			slow = append(slow, rec)
		}
	}

	// Sort by duration descending (slowest first)
	sort.Slice(slow, func(i, j int) bool {
		return slow[i].Duration > slow[j].Duration
	})

	return slow
}

// SuggestAlias returns alias suggestions for commands used more than minCount times.
func (ch *CommandHistory) SuggestAlias(minCount int) []AliasSuggestion {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	// Count exact commands
	counts := make(map[string]int)
	for _, rec := range ch.Commands {
		counts[rec.Command]++
	}

	var suggestions []AliasSuggestion
	for cmd, count := range counts {
		if count < minCount {
			continue
		}
		// Only suggest for commands with arguments (longer commands benefit more from aliases)
		if !strings.Contains(cmd, " ") {
			continue
		}
		alias := generateAlias(cmd)
		suggestions = append(suggestions, AliasSuggestion{
			Command: cmd,
			Alias:   alias,
			Count:   count,
			Reason:  fmt.Sprintf("Used %d times — alias saves %d characters per invocation", count, len(cmd)-len(alias)),
		})
	}

	// Sort by count descending
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Count > suggestions[j].Count
	})

	return suggestions
}

// DetectPatterns analyzes command history for notable patterns and returns insights.
func (ch *CommandHistory) DetectPatterns() []string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	var patterns []string

	// Pattern 1: Sequential command pairs
	pairCounts := make(map[string]int)
	for i := 0; i < len(ch.Commands)-1; i++ {
		pair := ch.Commands[i].Command + " && " + ch.Commands[i+1].Command
		pairCounts[pair]++
	}
	for pair, count := range pairCounts {
		if count >= 3 {
			parts := strings.SplitN(pair, " && ", 2)
			patterns = append(patterns, fmt.Sprintf(
				"You frequently run `%s` after `%s` — consider combining", parts[1], parts[0],
			))
		}
	}

	// Pattern 2: High failure rates
	cmdTotal := make(map[string]int)
	cmdFails := make(map[string]int)
	for _, rec := range ch.Commands {
		cmdTotal[rec.Command]++
		if rec.ExitCode != 0 {
			cmdFails[rec.Command]++
		}
	}
	for cmd, fails := range cmdFails {
		total := cmdTotal[cmd]
		if total >= 3 {
			rate := float64(fails) / float64(total) * 100
			if rate >= 30 {
				patterns = append(patterns, fmt.Sprintf(
					"Command `%s` fails %.0f%% of the time — check for issues", cmd, rate,
				))
			}
		}
	}

	// Pattern 3: Most common command
	var maxCmd string
	var maxCount int
	for cmd, count := range cmdTotal {
		if count > maxCount {
			maxCount = count
			maxCmd = cmd
		}
	}
	if maxCmd != "" && maxCount >= 3 {
		patterns = append(patterns, fmt.Sprintf(
			"`%s` is your most common command (%d times)", maxCmd, maxCount,
		))
	}

	return patterns
}

// FormatSummary returns a formatted string summary of command history.
func (ch *CommandHistory) FormatSummary() string {
	ch.mu.RLock()
	cmdCount := len(ch.Commands)
	ch.mu.RUnlock()

	if cmdCount == 0 {
		return "Command History: (empty)\n"
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Command History (last %d):\n", cmdCount))
	sb.WriteString("──────────────────────────────\n")

	// Most used
	frequent := ch.GetFrequent(3)
	if len(frequent) > 0 {
		sb.WriteString("Most used:\n")
		for _, f := range frequent {
			avgSec := f.AvgDuration.Seconds()
			sb.WriteString(fmt.Sprintf("  %-22s (%dx, avg %.1fs)\n", f.Command, f.Count, avgSec))
		}
		sb.WriteString("\n")
	}

	// Failing
	failStats := ch.getFailStats()
	if len(failStats) > 0 {
		sb.WriteString("Failing:\n")
		for _, fs := range failStats {
			sb.WriteString(fmt.Sprintf("  %-22s (%d/%d failed, %.0f%%)\n",
				fs.cmd, fs.fails, fs.total, fs.rate*100))
		}
		sb.WriteString("\n")
	}

	// Slow commands (threshold: 5 seconds)
	slow := ch.GetSlow(5 * time.Second)
	if len(slow) > 0 {
		sb.WriteString("Slow:\n")
		// Deduplicate and average
		slowAvg := make(map[string][]time.Duration)
		for _, s := range slow {
			slowAvg[s.Command] = append(slowAvg[s.Command], s.Duration)
		}
		for cmd, durs := range slowAvg {
			var total time.Duration
			for _, d := range durs {
				total += d
			}
			avg := total / time.Duration(len(durs))
			sb.WriteString(fmt.Sprintf("  %-22s (avg %.1fs)\n", cmd, avg.Seconds()))
		}
		sb.WriteString("\n")
	}

	// Suggestions
	aliases := ch.SuggestAlias(3)
	detectedPatterns := ch.DetectPatterns()
	if len(aliases) > 0 || len(detectedPatterns) > 0 {
		sb.WriteString("Suggestions:\n")
		for _, a := range aliases {
			sb.WriteString(fmt.Sprintf("  • Alias: %s → %s\n", a.Alias, a.Command))
		}
		for _, p := range detectedPatterns {
			sb.WriteString(fmt.Sprintf("  • Pattern: %s\n", p))
		}
	}

	return sb.String()
}

// SearchCommands returns commands matching the query string.
func (ch *CommandHistory) SearchCommands(query string) []CommandRecord {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	query = strings.ToLower(query)
	var results []CommandRecord
	for _, rec := range ch.Commands {
		if strings.Contains(strings.ToLower(rec.Command), query) ||
			strings.Contains(strings.ToLower(rec.Output), query) ||
			strings.Contains(strings.ToLower(rec.WorkDir), query) {
			results = append(results, rec)
		}
	}

	return results
}

// Clear removes all command history.
func (ch *CommandHistory) Clear() {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.Commands = make([]CommandRecord, 0)
	ch.Patterns = make(map[string]int)
	ch.Failures = make(map[string]int)
}

// failStat holds aggregated failure info for a command.
type failStat struct {
	cmd   string
	fails int
	total int
	rate  float64
}

// getFailStats returns aggregated failure statistics for commands with failures.
func (ch *CommandHistory) getFailStats() []failStat {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	cmdTotal := make(map[string]int)
	cmdFails := make(map[string]int)
	for _, rec := range ch.Commands {
		cmdTotal[rec.Command]++
		if rec.ExitCode != 0 {
			cmdFails[rec.Command]++
		}
	}

	var stats []failStat
	for cmd, fails := range cmdFails {
		total := cmdTotal[cmd]
		rate := float64(fails) / float64(total)
		stats = append(stats, failStat{
			cmd:   cmd,
			fails: fails,
			total: total,
			rate:  rate,
		})
	}

	// Sort by failure rate descending
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].rate > stats[j].rate
	})

	return stats
}

// extractBaseCommand returns the first word of a command string.
func extractBaseCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return cmd
	}
	return parts[0]
}

// generateAlias creates a short alias from a command string.
func generateAlias(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return cmd
	}

	// Use first letter of each word, up to 4 characters
	var alias strings.Builder
	for i, part := range parts {
		if i >= 4 {
			break
		}
		// For paths like ./..., use the base command portion
		if strings.HasPrefix(part, "-") || strings.HasPrefix(part, ".") {
			continue
		}
		if len(part) > 0 {
			alias.WriteByte(part[0])
		}
	}

	result := alias.String()
	if result == "" {
		// Fallback: first two chars of first part + first char of second
		if len(parts) >= 2 {
			p1 := parts[0]
			p2 := parts[1]
			if len(p1) >= 2 {
				result = p1[:2]
			} else {
				result = p1
			}
			if len(p2) > 0 {
				result += string(p2[0])
			}
		} else {
			result = parts[0]
		}
	}

	return result
}
