package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadLatestForCWD(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()

	old := &Session{ID: "old", CWD: cwd, UpdatedAt: time.Now().Add(-time.Hour), Messages: []Message{{Role: "user", Content: "old"}}}
	newer := &Session{ID: "new", CWD: cwd, UpdatedAt: time.Now(), Messages: []Message{{Role: "user", Content: "new"}}}
	if err := Save(old); err != nil {
		t.Fatal(err)
	}
	if err := Save(newer); err != nil {
		t.Fatal(err)
	}

	got, err := LoadLatestForCWD(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "new" {
		t.Fatalf("got %q, want new", got.ID)
	}
}

func TestSaveFillsCWD(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(cwd)
	defer os.Chdir(orig)

	if err := Save(&Session{ID: "session"}); err != nil {
		t.Fatal(err)
	}
	got, err := Load("session")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.Getwd()
	want, _ = filepath.Abs(want)
	if got.CWD != want {
		t.Fatalf("got cwd %q, want %q", got.CWD, want)
	}
}

// TestLoadJSONLSkipsOversizeLine verifies the LOW finding fix: a single
// message line larger than the scanner buffer must not brick the whole
// session load — it is skipped+logged, and subsequent valid messages are
// still read.
func TestLoadJSONLSkipsOversizeLine(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	dir := sessionsDir()
	_ = os.MkdirAll(dir, 0o750)

	id := "oversize-test"
	path := jsonlPathFor(id)
	var b strings.Builder
	b.WriteString(`{"type":"session_meta","id":"` + id + `","model":"m","provider":"p"}` + "\n")
	b.WriteString(`{"role":"user","content":"hello"}` + "\n")
	b.WriteString(strings.Repeat("x", 1024*1024*2) + "\n")
	b.WriteString(`{"role":"assistant","content":"there"}` + "\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load(id)
	if err != nil {
		t.Fatalf("Load returned error after oversize line: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("expected 2 messages (oversize skipped), got %d", len(got.Messages))
	}
	if got.Messages[0].Content != "hello" || got.Messages[1].Content != "there" {
		t.Fatalf("unexpected messages: %+v", got.Messages)
	}
}
