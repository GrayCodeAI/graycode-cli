package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeHL(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func hlHash(s string) string { return Anchor(s) }

func TestReadAnchored(t *testing.T) {
	p := writeHL(t, "alpha\nbeta\ngamma\n")
	lines, err := ReadAnchored(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("lines = %d", len(lines))
	}
	if lines[0].Line != 1 || lines[0].Content != "alpha" {
		t.Fatalf("lines[0] = %+v", lines[0])
	}
	if lines[0].Hash == lines[1].Hash {
		t.Fatal("distinct lines share a hash")
	}
	out := RenderAnchored(lines)
	if !strings.Contains(out, "L1:"+hlHash("alpha")+"|alpha") {
		t.Fatalf("render = %q", out)
	}
}

func TestApplyReplace(t *testing.T) {
	p := writeHL(t, "one\ntwo\nthree\n")
	err := ApplyEdits(p, []EditOp{{Line: 2, Hash: hlHash("two"), Op: "replace", Text: "TWO"}})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "one\nTWO\nthree" {
		t.Fatalf("content = %q", got)
	}
}

func TestApplyInsertAfterAndDelete(t *testing.T) {
	p := writeHL(t, "a\nb\nc\n")
	err := ApplyEdits(p, []EditOp{
		{Line: 1, Hash: hlHash("a"), Op: "insert_after", Text: "a2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "a\na2\nb\nc" {
		t.Fatalf("after insert: %q", got)
	}
	// Delete the inserted line.
	err = ApplyEdits(p, []EditOp{{Line: 2, Hash: hlHash("a2"), Op: "delete"}})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(p)
	if string(got) != "a\nb\nc" {
		t.Fatalf("after delete: %q", got)
	}
}

func TestApplyAtomicRejectsBadHash(t *testing.T) {
	p := writeHL(t, "one\ntwo\n")
	err := ApplyEdits(p, []EditOp{
		{Line: 1, Hash: hlHash("one"), Op: "replace", Text: "OK"},
		{Line: 2, Hash: hlHash("WRONG"), Op: "replace", Text: "BAD"},
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
	// Nothing applied (atomic).
	got, _ := os.ReadFile(p)
	if string(got) != "one\ntwo\n" {
		t.Fatalf("partial application detected: %q", got)
	}
}

func TestShiftedAnchorRecovery(t *testing.T) {
	p := writeHL(t, "l1\nl2\nl3\nl4\nl5\n")
	// Simulate drift: insert a line above l3 so it shifts from L3 to L4.
	if err := ApplyEdits(p, []EditOp{{Line: 1, Hash: hlHash("l1"), Op: "insert_after", Text: "inserted"}}); err != nil {
		t.Fatal(err)
	}
	// The agent still references l3's OLD position (line 3), but its hash is valid.
	err := ApplyEdits(p, []EditOp{{Line: 3, Hash: hlHash("l3"), Op: "replace", Text: "L3-REPLACED"}})
	if err != nil {
		t.Fatalf("shifted anchor not recovered: %v", err)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "L3-REPLACED") {
		t.Fatalf("recovery did not apply edit: %q", got)
	}
}

func TestAnchorBeyondWindowFails(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&sb, "line-%d\n", i)
	}
	p := writeHL(t, sb.String())
	// line-15 was at L16; after inserting 10 lines above it drifted beyond ±5.
	// We simulate by asking for L6 with hash of "line-15".
	err := ApplyEdits(p, []EditOp{{Line: 6, Hash: hlHash("line-15"), Op: "replace", Text: "X"}})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected rejection for out-of-window drift, got %v", err)
	}
}

func TestMultipleEditsBottomUp(t *testing.T) {
	p := writeHL(t, "a\nb\nc\nd\n")
	err := ApplyEdits(p, []EditOp{
		{Line: 4, Hash: hlHash("d"), Op: "replace", Text: "D"},
		{Line: 1, Hash: hlHash("a"), Op: "delete"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	// Delete L1 first would shift d from L4→L3; bottom-up ordering avoids this.
	if string(got) != "b\nc\nD" {
		t.Fatalf("bottom-up order wrong: %q", got)
	}
}

func TestEditValidation(t *testing.T) {
	cases := []struct {
		e    EditOp
		want string
	}{
		{EditOp{Op: "nope"}, "unknown op"},
		{EditOp{Line: 1, Hash: "abc12345", Op: "replace"}, "requires text"},
		{EditOp{Line: -1, Op: "replace", Text: "x"}, "line must be >= 1"},
		{EditOp{Line: 1, Op: "replace", Text: "x"}, "hash is required"},
	}
	for _, c := range cases {
		if err := c.e.Validate(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("EditOp %+v: err=%v want~%q", c.e, err, c.want)
		}
	}
}
