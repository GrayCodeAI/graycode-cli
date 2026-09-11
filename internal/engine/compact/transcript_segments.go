package compact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// Compaction transcript segments, adopted from grok-build's
// xai-compaction-transcript: when compaction summarizes old turns away, the
// verbatim turns are persisted as self-contained markdown segments under the
// session store so full history remains retrievable after the fact. An INDEX.md
// makes segments discoverable without parsing every file.

// CompactionDetail controls how much per-turn detail lands in a segment.
type CompactionDetail int

const (
	// SegmentNone: stats only, no verbatim turns.
	SegmentNone CompactionDetail = iota
	// SegmentMinimal: one-line signature per turn.
	SegmentMinimal
	// SegmentBalanced: tool calls + truncated responses + full text.
	SegmentBalanced
	// SegmentVerbose: full verbatim turns (default).
	SegmentVerbose
)

// ParseCompactionDetail parses a user-facing detail level name.
func ParseCompactionDetail(s string) (CompactionDetail, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none":
		return SegmentNone, true
	case "minimal":
		return SegmentMinimal, true
	case "balanced":
		return SegmentBalanced, true
	case "verbose", "":
		return SegmentVerbose, true
	default:
		return SegmentVerbose, false
	}
}

const (
	segmentsDirName = "compaction"
	indexFileName   = "INDEX.md"
	segmentPrefix   = "segment_"

	// segmentMaxBytes caps one segment's verbatim body; overflow turns are
	// omitted behind an explicit truncation notice.
	segmentMaxBytes = 512 * 1024

	balancedTextChars     = 2000
	balancedResponseChars = 500

	perTurnOverheadBytes = 64
)

var segmentFileRe = regexp.MustCompile(`^segment_(\d+)\.md$`)

// SegmentsDir is the per-session directory holding compaction segments.
func SegmentsDir(sessionID string) string {
	return filepath.Join(storage.SessionsDir(), sessionID, segmentsDirName)
}

// IndexPath is the per-session segment index file.
func IndexPath(sessionID string) string {
	return filepath.Join(SegmentsDir(sessionID), indexFileName)
}

// RenderSegmentToMarkdown renders messages into a self-contained markdown
// segment. Pure: no I/O.
func RenderSegmentToMarkdown(msgs []types.EyrieMessage, segIndex int, detail CompactionDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Compaction segment %d\n\n", segIndex)
	fmt.Fprintf(&b, "Recorded: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "Turns: %d | Detail: %s\n", len(msgs), detail.String())

	if detail == SegmentNone || len(msgs) == 0 {
		b.WriteString("\n(verbatim turns not recorded at this detail level)\n")
		return b.String()
	}

	var body strings.Builder
	used := 0
	omitted := 0
	for _, m := range msgs {
		entry := renderTurn(m, detail)
		cost := len(entry) + perTurnOverheadBytes
		if used+cost > segmentMaxBytes {
			omitted++
			continue
		}
		used += cost
		body.WriteString(entry)
	}
	b.WriteString("\n---\n\n")
	b.WriteString(body.String())
	if omitted > 0 {
		fmt.Fprintf(&b, "\n[... TRUNCATED at %d bytes, %d turns omitted ...]\n", segmentMaxBytes, omitted)
	}
	return b.String()
}

func renderTurn(m types.EyrieMessage, detail CompactionDetail) string {
	var b strings.Builder
	role := strings.ToUpper(m.Role)
	fmt.Fprintf(&b, "## %s\n", role)

	text := strings.TrimSpace(m.Content)
	switch detail {
	case SegmentMinimal:
		if text != "" {
			fmt.Fprintf(&b, "%s\n", oneLine(text))
		}
		for _, tu := range m.ToolUse {
			fmt.Fprintf(&b, "- tool: %s\n", tu.Name)
		}
	default:
		if text != "" {
			if detail == SegmentBalanced {
				text = truncateRunesStr(text, balancedTextChars)
			}
			fmt.Fprintf(&b, "%s\n", text)
		}
		for _, tu := range m.ToolUse {
			args := summarizeToolArgs(tu.Arguments, detail)
			fmt.Fprintf(&b, "- tool: %s(%s)\n", tu.Name, args)
		}
		for _, tr := range m.ToolResults {
			out := tr.Content
			if detail == SegmentBalanced {
				out = truncateRunesStr(out, balancedResponseChars)
			}
			fmt.Fprintf(&b, "- result: %s\n", oneLine(out))
		}
	}
	b.WriteString("\n")
	return b.String()
}

func summarizeToolArgs(args map[string]interface{}, detail CompactionDetail) string {
	if detail == SegmentBalanced {
		keys := make([]string, 0, len(args))
		for k := range args {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=…", k))
		}
		return strings.Join(parts, ", ")
	}
	// encoding/json emits map keys in sorted order, giving deterministic args
	// digests across runs.
	if raw, err := json.Marshal(args); err == nil {
		s := string(raw)
		if len(s) > 300 {
			s = s[:300] + "…"
		}
		return s
	}
	return "…"
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func truncateRunesStr(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// NextSegmentIndex derives the next segment number from existing files on
// disk, falling back to the INDEX rows when files were pruned.
func NextSegmentIndex(sessionID string) int {
	max := -1
	entries, err := os.ReadDir(SegmentsDir(sessionID))
	if err == nil {
		for _, e := range entries {
			m := segmentFileRe.FindStringSubmatch(e.Name())
			if m == nil {
				continue
			}
			if n, err := strconv.Atoi(m[1]); err == nil && n > max {
				max = n
			}
		}
	}
	return max + 1
}

// WriteCompactionSegment persists msgs as the next segment and appends an
// INDEX.md row. It returns the written segment path. Callers should treat
// errors as non-fatal: segment persistence must never block compaction.
func WriteCompactionSegment(sessionID string, msgs []types.EyrieMessage, detail CompactionDetail) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("compact: empty session id")
	}
	dir := SegmentsDir(sessionID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("compact: create segments dir: %w", err)
	}
	idx := NextSegmentIndex(sessionID)
	path := filepath.Join(dir, fmt.Sprintf("%s%04d.md", segmentPrefix, idx))
	content := RenderSegmentToMarkdown(msgs, idx, detail)
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil { // #nosec G306 -- session-owned transcript segment
		return "", fmt.Errorf("compact: write segment: %w", err)
	}
	if err := appendIndexRow(sessionID, idx, len(msgs), len(content)); err != nil {
		return path, fmt.Errorf("compact: segment written but index update failed: %w", err)
	}
	return path, nil
}

// appendIndexRow appends (creating with a header when absent) an INDEX.md row.
func appendIndexRow(sessionID string, segIndex, turns, bytes int) error {
	path := IndexPath(sessionID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		header := "# Compaction segments\n\n| segment | turns | bytes | recorded |\n|---|---|---|---|\n"
		if err := os.WriteFile(path, []byte(header), 0o640); err != nil { // #nosec G306 -- session-owned index
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o640) // #nosec G304 -- path derived from session storage root
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	row := fmt.Sprintf("| [%d](%s%04d.md) | %d | %d | %s |\n",
		segIndex, segmentPrefix, segIndex, turns, bytes, time.Now().UTC().Format(time.RFC3339))
	_, err = f.WriteString(row)
	return err
}

// String implements fmt.Stringer for logs.
func (d CompactionDetail) String() string {
	switch d {
	case SegmentNone:
		return "none"
	case SegmentMinimal:
		return "minimal"
	case SegmentBalanced:
		return "balanced"
	default:
		return "verbose"
	}
}
