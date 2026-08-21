package tape

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// frameBytesForTest builds a tiny tape with a stdout frame (a prompt banner
// plus a DECTCEM cursor-hide) and a marker frame, without Close so no trailing
// resize frame is appended.
func frameBytesForTest(t *testing.T, clk *fakeClock) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := NewWriter(&buf, 80, 24, "1.2", clk)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	clk.t = 1010
	if err := w.RecordStdout([]byte("────────────────\r\n❯ hello world\r\n────────────────\r\n\x1b[?25l")); err != nil {
		t.Fatalf("record stdout: %v", err)
	}
	clk.t = 1030
	if err := w.RecordMarker("hello"); err != nil {
		t.Fatalf("record marker: %v", err)
	}
	return buf.Bytes()
}

func TestExportFramesDirArtifacts(t *testing.T) {
	tp, err := Parse(frameBytesForTest(t, &fakeClock{t: 1000}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	dir := t.TempDir()
	sum, err := ExportFramesDir(dir, tp)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if sum.FrameCount != 2 {
		t.Errorf("FrameCount = %d, want 2", sum.FrameCount)
	}
	if sum.ResizeCount != 0 {
		t.Errorf("ResizeCount = %d, want 0", sum.ResizeCount)
	}
	if sum.StdoutBytes == 0 {
		t.Error("StdoutBytes = 0, want > 0")
	}

	// manifest.json: compact, ends with newline, matches fx schema.
	mraw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.HasSuffix(string(mraw), "\n") {
		t.Error("manifest.json missing trailing newline")
	}
	var m struct {
		Cols        float64 `json:"cols"`
		Rows        float64 `json:"rows"`
		FrameCount  float64 `json:"frame_count"`
		ResizeCount float64 `json:"resize_count"`
		StdoutBytes float64 `json:"stdout_bytes"`
		FramesDir   string  `json:"frames_dir"`
	}
	if err := json.Unmarshal(mraw, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if m.Cols != 80 || m.Rows != 24 || m.FrameCount != 2 || m.ResizeCount != 0 || m.FramesDir != "frames" {
		t.Errorf("manifest fields wrong: %+v", m)
	}
	if m.StdoutBytes == 0 {
		t.Errorf("manifest StdoutBytes = 0, want > 0")
	}

	// Frame 1: stdout with the prompt banner; cursor hidden; footer + marker.
	f1, err := os.ReadFile(filepath.Join(dir, "frames", "0001.json"))
	if err != nil {
		t.Fatalf("read 0001.json: %v", err)
	}
	if !strings.HasSuffix(string(f1), "\n") {
		t.Error("0001.json missing trailing newline")
	}
	var d map[string]any
	if err := json.Unmarshal(f1, &d); err != nil {
		t.Fatalf("unmarshal 0001.json: %v", err)
	}
	if d["index"].(float64) != 1 {
		t.Errorf("index = %v, want 1", d["index"])
	}
	if d["kind"] != "stdout" {
		t.Errorf("kind = %v, want stdout", d["kind"])
	}
	cursor := d["cursor"].(map[string]any)
	if cursor["visible"] != false {
		t.Errorf("cursor.visible = %v, want false after CSI ?25l", cursor["visible"])
	}
	foot := d["footer_candidates"].([]any)
	if len(foot) != 1 {
		t.Fatalf("footer_candidates = %d entries, want 1", len(foot))
	}
	f0 := foot[0].(map[string]any)
	// fx emits {top: i, input: i+1, bottom: i+2} for a row framed by dividers.
	if f0["top_divider"].(float64) != 1 || f0["input"].(float64) != 2 || f0["bottom_divider"].(float64) != 3 {
		t.Errorf("footer = %+v, want {top:1 input:2 bottom:3}", f0)
	}
	mark := d["visible_markers"].([]any)
	if len(mark) != 0 {
		t.Errorf("frame1 visible_markers = %v, want empty (marker not reached yet)", mark)
	}

	gt, err := os.ReadFile(filepath.Join(dir, "frames", "0001.grid.txt"))
	if err != nil {
		t.Fatalf("read 0001.grid.txt: %v", err)
	}
	if !strings.Contains(string(gt), "❯ hello world") {
		t.Errorf("grid.txt missing prompt, got:\n%s", gt)
	}

	// Frame 2: marker artifact.
	f2, err := os.ReadFile(filepath.Join(dir, "frames", "0002.json"))
	if err != nil {
		t.Fatalf("read 0002.json: %v", err)
	}
	var d2 map[string]any
	if err := json.Unmarshal(f2, &d2); err != nil {
		t.Fatalf("unmarshal 0002.json: %v", err)
	}
	if d2["index"].(float64) != 2 || d2["kind"] != "marker" {
		t.Errorf("0002 = {index:%v kind:%v}, want {2 marker}", d2["index"], d2["kind"])
	}
	mark2 := d2["visible_markers"].([]any)
	if len(mark2) != 1 || mark2[0] != "hello" {
		t.Errorf("frame2 visible_markers = %v, want [hello]", mark2)
	}
	if _, err := os.Stat(filepath.Join(dir, "frames", "0002.grid.txt")); err != nil {
		t.Errorf("0002.grid.txt missing: %v", err)
	}
}
