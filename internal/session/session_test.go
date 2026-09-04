package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
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

// TestSaveLoadRoundTripsEvents verifies that a version-1 session persists its
// event spine alongside messages, and that a version-0 session stays byte-compatible
// (no event lines, meta has no format_version).
func TestSaveLoadRoundTripsEvents(t *testing.T) {
	setTestSessionsDir(t, t.TempDir())

	mkEvent := func(typ eventlog.Type, seq uint64, data string) eventlog.WireEvent {
		return eventlog.WireEvent{
			Type: typ,
			Seq:  seq,
			Data: json.RawMessage([]byte(data)),
		}
	}

	s := &Session{
		ID:       "evt-session",
		Model:    "m",
		Provider: "p",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Events: []eventlog.WireEvent{
			mkEvent(eventlog.UserMessage, 1, `{"role":"user","content":"hi"}`),
			mkEvent(eventlog.AssistantMsg, 2, `{"role":"assistant","content":"hello"}`),
		},
	}
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(jsonlPathFor("evt-session"))
	if err != nil {
		t.Fatalf("read persisted session: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 4 { // meta + 1 message + 2 events
		t.Fatalf("persisted %d lines, want 4:\n%s", len(lines), raw)
	}
	if !strings.Contains(lines[0], `"format_version":1`) {
		t.Fatalf("meta missing format_version: %s", lines[0])
	}

	got, err := Load("evt-session")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Events == nil || len(got.Events) != 2 {
		t.Fatalf("loaded %d events, want 2", len(got.Events))
	}
	if len(got.Messages) != 1 {
		t.Fatalf("loaded %d messages, want 1", len(got.Messages))
	}

	// Version-0 (no events) stays byte-compatible.
	v0 := &Session{
		ID:       "v0-session",
		Messages: []Message{{Role: "user", Content: "plain"}},
	}
	if err := Save(v0); err != nil {
		t.Fatalf("Save v0: %v", err)
	}
	v0Raw, err := os.ReadFile(jsonlPathFor("v0-session"))
	if err != nil {
		t.Fatalf("read v0 session: %v", err)
	}
	if strings.Contains(string(v0Raw), "format_version") {
		t.Fatalf("v0 session unexpectedly contains format_version:\n%s", v0Raw)
	}
}

// TestLoadJSONLSkipsOversizeLine verifies the LOW finding fix: a single
// message line larger than the scanner buffer must not brick the whole
// session load — it is skipped+logged, and subsequent valid messages are
// still read.
func TestLoadJSONLSkipsOversizeLine(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())
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
