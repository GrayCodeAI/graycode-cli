package tape

import (
	"os"
	"path/filepath"
	"testing"
)

// Smoke: record a short terminal session to a real file, then parse and
// replay it, verifying the recorded output is reconstructed.
func TestRecordReplaySmoke(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.fxtape")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	clk := &fakeClock{t: 1_700_000_000_000}
	w, err := NewWriter(f, 120, 40, "graycode-test", clk)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a real session transcript.
	clk.t += 12
	_ = w.RecordStdout([]byte("$ git status\n"))
	clk.t += 40
	_ = w.RecordStdout([]byte("On branch main\nnothing to commit\n"))
	clk.t += 8
	_ = w.RecordResize(100, 30)
	clk.t += 5
	_ = w.RecordStdout([]byte("\x1b[33m$ echo done\x1b[0m\n"))
	clk.t += 7
	_ = w.RecordMarker("end-of-turn")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	// Different process path: read the file back and replay.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tp, err := Parse(data)
	if err != nil {
		t.Fatalf("parse recorded tape: %v", err)
	}
	if tp.Header.Cols != 120 || tp.Header.Rows != 40 || tp.Header.Version != "graycode-test" {
		t.Fatalf("bad header: %+v", tp.Header)
	}
	r, snap := ReplayTape(tp)
	if len(tp.Frames) != 5 {
		t.Fatalf("expected 5 frames, got %d", len(tp.Frames))
	}
	if len(r.Markers) != 1 || r.Markers[0] != "end-of-turn" {
		t.Fatalf("markers=%v", r.Markers)
	}
	for _, want := range []string{"$ git status", "On branch main", "nothing to commit", "$ echo done"} {
		if !containsLine(snap, want) {
			t.Fatalf("snapshot missing %q:\n%s", want, snap)
		}
	}
	// Resize honored.
	if r.Grid.Cols != 100 || r.Grid.Rows != 30 {
		t.Fatalf("grid not resized to 100x30, got %dx%d", r.Grid.Cols, r.Grid.Rows)
	}
}

func containsLine(s, line string) bool {
	for _, l := range splitLines(s) {
		if l == line {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}
