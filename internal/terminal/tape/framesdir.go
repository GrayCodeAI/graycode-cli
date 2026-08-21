// Package tape: fx `replay --frames-dir` parity (src/core/cli/cli_replay.zig)
//
// ExportFramesDir replays a tape and writes one artifact per frame into a
// directory mirroring fx's file layout and JSON schema:
//
//	<root>/frames/0001.json      per-frame metadata (index, timing, kind,
//	                            size, cursor, footer_candidates,
//	                            visible_markers)
//	<root>/frames/0001.grid.txt  the rendered terminal snapshot after the frame
//	<root>/manifest.json         tape header summary + aggregate frame stats
//
// File names and the JSON index are 1-based, matching fx. Each JSON artifact
// is a single compact line ending in '\n'.
package tape

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FramesDirSummary reports aggregate counts mirroring fx's manifest fields.
type FramesDirSummary struct {
	FrameCount  int
	ResizeCount int
	StdoutBytes int
}

// dirFrame mirrors fx's per-frame JSON artifact (cli_replay.zig
// writeFrameArtifacts).
type dirFrame struct {
	Index      int         `json:"index"`
	DeltaMS    int32       `json:"delta_ms"`
	ElapsedMS  int64       `json:"elapsed_ms"`
	Kind       string      `json:"kind"`
	PayloadLen int         `json:"payload_len"`
	Size       dirSize     `json:"size"`
	Cursor     dirCursor   `json:"cursor"`
	Footers    []dirFooter `json:"footer_candidates"`
	Markers    []string    `json:"visible_markers"`
}

type dirSize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

type dirCursor struct {
	Row     int  `json:"row"`
	Col     int  `json:"col"`
	Visible bool `json:"visible"`
}

// dirFooter mirrors fx's footer_candidates entries. Fx requires the input row
// to be framed between two divider rows, then emits indices offset by one to
// match its own output; we mirror fx's indices verbatim for compatibility.
type dirFooter struct {
	TopDivider    int `json:"top_divider"`
	Input         int `json:"input"`
	BottomDivider int `json:"bottom_divider"`
}

// dirManifest mirrors fx's manifest.json (cli_replay.zig writeFramesManifest).
type dirManifest struct {
	Cols        uint16 `json:"cols"`
	Rows        uint16 `json:"rows"`
	EpochMS     int64  `json:"epoch_ms"`
	Version     string `json:"version"`
	FrameCount  int    `json:"frame_count"`
	ResizeCount int    `json:"resize_count"`
	StdoutBytes int    `json:"stdout_bytes"`
	FramesDir   string `json:"frames_dir"`
}

// ExportFramesDir replays t and writes per-frame artifacts plus manifest.json
// under root, following fx `replay --frames-dir` exactly: index and timing are
// 1-based cumulative, and each frame's artifact reflects the grid state after
// applying that frame.
func ExportFramesDir(root string, t *Tape) (*FramesDirSummary, error) {
	framesPath := filepath.Join(root, "frames")
	if err := os.MkdirAll(framesPath, 0o755); err != nil {
		return nil, err
	}

	grid := NewGrid(int(t.Header.Cols), int(t.Header.Rows))
	var markers []string
	sum := &FramesDirSummary{}
	var elapsed int64

	for _, f := range t.Frames {
		sum.FrameCount++
		elapsed += int64(f.DeltaMS)
		switch f.Kind {
		case KindStdout:
			sum.StdoutBytes += len(f.Payload)
			grid.Feed(f.Payload)
		case KindResize:
			if len(f.Payload) >= 4 {
				cols := int(f.Payload[0]) | int(f.Payload[1])<<8
				rows := int(f.Payload[2]) | int(f.Payload[3])<<8
				grid.Resize(cols, rows)
				sum.ResizeCount++
			}
		case KindMarker:
			markers = append(markers, string(f.Payload))
		}
		if err := writeDirFrame(framesPath, sum.FrameCount, f, elapsed, grid, markers); err != nil {
			return nil, err
		}
	}

	m := dirManifest{
		Cols:        t.Header.Cols,
		Rows:        t.Header.Rows,
		EpochMS:     t.Header.EpochMS,
		Version:     t.Header.Version,
		FrameCount:  sum.FrameCount,
		ResizeCount: sum.ResizeCount,
		StdoutBytes: sum.StdoutBytes,
		FramesDir:   "frames",
	}
	manifest, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), append(manifest, '\n'), 0o644); err != nil {
		return nil, err
	}
	return sum, nil
}

// writeDirFrame writes NNNN.json and NNNN.grid.txt for one frame, mirroring
// fx's writeFrameArtifacts.
func writeDirFrame(dir string, index int, f Frame, elapsed int64, grid *Grid, markers []string) error {
	snapshot := grid.Snapshot()
	stem := fmt.Sprintf("%04d", index)

	frameJSON := dirFrame{
		Index:      index,
		DeltaMS:    f.DeltaMS,
		ElapsedMS:  elapsed,
		Kind:       f.Kind.String(),
		PayloadLen: len(f.Payload),
		Size:       dirSize{Cols: grid.Cols, Rows: grid.Rows},
		Cursor:     dirCursor{Row: grid.CursorRow(), Col: grid.CursorCol(), Visible: grid.CursorVisible()},
		Footers:    footerCandidates(snapshot),
		Markers:    visibleMarkers(snapshot, markers),
	}
	raw, err := json.Marshal(frameJSON)
	if err != nil {
		return err
	}

	base := filepath.Join(dir, stem)
	if err := os.WriteFile(base+".json", append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(base+".grid.txt", []byte(snapshot), 0o644)
}

// footerCandidates returns snapshot rows that look like framed input prompts,
// matching fx's isInputSnapshotRow + isDividerSnapshotRow logic and index skew.
func footerCandidates(snapshot string) []dirFooter {
	var lines []string
	for _, line := range strings.Split(snapshot, "\n") {
		if line = trimSnapshotRow(line); line != "" {
			lines = append(lines, line)
		}
	}
	out := make([]dirFooter, 0)
	for i, line := range lines {
		if !isInputSnapshotRow(line) {
			continue
		}
		if i == 0 || i+1 >= len(lines) {
			continue
		}
		if !isDividerSnapshotRow(lines[i-1]) || !isDividerSnapshotRow(lines[i+1]) {
			continue
		}
		// Fx emits {i, i+1, i+2} as {top, input, bottom}; mirror verbatim.
		out = append(out, dirFooter{TopDivider: i, Input: i + 1, BottomDivider: i + 2})
	}
	return out
}

// visibleMarkers returns the markers whose text appears in the snapshot,
// preserving order (fx writeVisibleMarkers).
func visibleMarkers(snapshot string, markers []string) []string {
	out := make([]string, 0)
	for _, m := range markers {
		if m != "" && strings.Contains(snapshot, m) {
			out = append(out, m)
		}
	}
	return out
}

func isInputSnapshotRow(line string) bool {
	text := trimSnapshotRow(line)
	if strings.HasPrefix(text, "❯") || strings.HasPrefix(text, ">") {
		return true
	}
	return strings.HasPrefix(text, "[") &&
		(strings.Contains(text, "] ❯") || strings.Contains(text, "] >"))
}

func isDividerSnapshotRow(line string) bool {
	text := trimSnapshotRow(line)
	return strings.Contains(text, "──") || strings.Contains(text, "━━") || strings.Contains(text, "══")
}

func trimSnapshotRow(line string) string {
	if len(line) >= 2 && line[0] == '|' && line[len(line)-1] == '|' {
		return strings.TrimRight(line[1:len(line)-1], " ")
	}
	return strings.TrimRight(line, " ")
}
