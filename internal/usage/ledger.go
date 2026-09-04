// Package usage provides an append-only, on-disk ledger of per-generation LLM
// usage, mirroring fx's `~/.fx/usage.jsonl` format. Sessions record one line
// per model generation; the `usage` command summarizes the ledger over a
// rolling window.
package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

// SchemaVersion is the on-disk ledger format version.
const SchemaVersion = 1

// Record is a single model generation's token and spend figure.
type Record struct {
	ID                     string  `json:"id,omitempty"`
	CreatedAtMS            int64   `json:"created_at_ms"`
	Model                  string  `json:"model"`
	Provider               string  `json:"provider,omitempty"`
	InputTokens            int     `json:"input_tokens"`
	OutputTokens           int     `json:"output_tokens"`
	CacheReadTokens        int     `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens       int     `json:"cache_write_tokens,omitempty"`
	ReasoningTokens        int     `json:"reasoning_tokens,omitempty"`
	BillableWebSearchCalls int     `json:"billable_web_search_calls,omitempty"`
	TotalCost              float64 `json:"total_cost"`
}

// LedgerPath returns the on-disk location of the usage ledger.
func LedgerPath() string {
	return filepath.Join(storage.StateDir(), "usage", "usage.jsonl")
}

// coverageMarker is written once, at the head of a fresh ledger, to mark the
// tracking window start (fx parity).
const coverageMarker = `{"schema_version":1,"kind":"coverage","started_at_ms":`

// Append records one generation to the default ledger, creating it (with the
// coverage marker) if absent. It is safe for concurrent callers.
func Append(r Record) error {
	return appendTo(LedgerPath(), r)
}

// appendTo writes a generation record to an explicit ledger path.
func appendTo(path string, r Record) error {
	if r.CreatedAtMS == 0 {
		r.CreatedAtMS = time.Now().UnixMilli()
	}
	line, err := json.Marshal(struct {
		SchemaVersion int    `json:"schema_version"`
		Kind          string `json:"kind"`
		Fact          Record `json:"fact"`
	}{SchemaVersion, "generation", r})
	if err != nil {
		return fmt.Errorf("usage: marshal record: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("usage: mkdir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("usage: open ledger: %w", err)
	}
	defer func() { _ = f.Close() }()

	if fi, err := f.Stat(); err == nil && fi.Size() == 0 {
		if _, err := f.WriteString(coverageMarker + fmt.Sprintf("%d}", r.CreatedAtMS) + "\n"); err != nil {
			return fmt.Errorf("usage: write coverage marker: %w", err)
		}
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("usage: append record: %w", err)
	}
	return nil
}

// Read loads every generation record from the default ledger, skipping the
// coverage marker. A missing ledger yields an empty slice, not an error.
func Read() ([]Record, error) {
	return ReadFrom(LedgerPath())
}

// ReadFrom loads generation records from an explicit ledger path.
func ReadFrom(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, nil
		}
		return nil, fmt.Errorf("usage: open ledger: %w", err)
	}
	defer func() { _ = f.Close() }()

	out := make([]Record, 0)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.Contains(line, `"kind":"coverage"`) {
			continue
		}
		var wrapped struct {
			Fact Record `json:"fact"`
		}
		if err := json.Unmarshal([]byte(line), &wrapped); err != nil {
			// Tolerate a malformed line (e.g. interrupted write) rather than
			// failing the whole report.
			continue
		}
		out = append(out, wrapped.Fact)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("usage: read ledger: %w", err)
	}
	return out, nil
}

// ModelUsage aggregates one model across the window.
type ModelUsage struct {
	Model            string  `json:"model"`
	Generations      int     `json:"generations"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	ReasoningTokens  int     `json:"reasoning_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
}

// Summary is the full report for a window.
type Summary struct {
	Generations  int          `json:"generations"`
	SinceMS      int64        `json:"since_ms"`
	UntilMS      int64        `json:"until_ms"`
	ByModel      []ModelUsage `json:"by_model"`
	TotalTokens  int          `json:"total_tokens"`
	TotalCostUSD float64      `json:"total_cost_usd"`
}

// Summarize aggregates records whose CreatedAtMS falls within [now-dur, now].
// Records at or after `since` (unix ms) are included.
func Summarize(records []Record, since int64) Summary {
	byModel := map[string]*ModelUsage{}
	order := []string{}
	totalTokens, totalCost := 0, 0.0

	for _, r := range records {
		if since > 0 && r.CreatedAtMS < since {
			continue
		}
		m, ok := byModel[r.Model]
		if !ok {
			m = &ModelUsage{Model: r.Model}
			byModel[r.Model] = m
			order = append(order, r.Model)
		}
		m.Generations++
		m.InputTokens += r.InputTokens
		m.OutputTokens += r.OutputTokens
		m.CacheReadTokens += r.CacheReadTokens
		m.CacheWriteTokens += r.CacheWriteTokens
		m.ReasoningTokens += r.ReasoningTokens
		m.TotalTokens += r.InputTokens + r.OutputTokens + r.CacheReadTokens + r.CacheWriteTokens + r.ReasoningTokens
		m.TotalCostUSD += r.TotalCost
		totalTokens += r.InputTokens + r.OutputTokens + r.CacheReadTokens + r.CacheWriteTokens + r.ReasoningTokens
		totalCost += r.TotalCost
	}

	sum := Summary{Generations: len(recordsFor(records, since)), ByModel: make([]ModelUsage, 0, len(order)), TotalTokens: totalTokens, TotalCostUSD: totalCost}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	for _, name := range order {
		sum.ByModel = append(sum.ByModel, *byModel[name])
	}
	return sum
}

func recordsFor(records []Record, since int64) []Record {
	var out []Record
	for _, r := range records {
		if since == 0 || r.CreatedAtMS >= since {
			out = append(out, r)
		}
	}
	return out
}

// ParsePeriod converts an fx-style period flag ("24h", "7d", "30d") into a
// duration and the unix-ms start of the window.
func ParsePeriod(s string) (sinceMS int64, d time.Duration, err error) {
	switch strings.TrimSpace(s) {
	case "", "24h":
		d = 24 * time.Hour
	case "7d":
		d = 7 * 24 * time.Hour
	case "30d":
		d = 30 * 24 * time.Hour
	default:
		return 0, 0, fmt.Errorf("usage: invalid period %q (want 24h, 7d, or 30d)", s)
	}
	now := time.Now()
	return now.Add(-d).UnixMilli(), d, nil
}
