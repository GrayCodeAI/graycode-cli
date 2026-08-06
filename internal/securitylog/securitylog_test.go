package securitylog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendAndVerify(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	ev1, err := l.Append(SeverityInfo, "tool_exec", "wrote file", "Write", "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	ev2, err := l.Append(SeverityCritical, "denied", "blocked sensitive path", "Bash", "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	if ev1.Seq != 1 || ev2.Seq != 2 {
		t.Fatalf("seq mismatch: %d %d", ev1.Seq, ev2.Seq)
	}
	if ev1.PrevHash != "" {
		t.Fatalf("first entry should link to genesis, got %q", ev1.PrevHash)
	}
	if ev2.PrevHash != ev1.Hash {
		t.Fatalf("second entry must chain to first: got %q want %q", ev2.PrevHash, ev1.Hash)
	}

	count, err := Verify(dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 verified entries, got %d", count)
	}
}

func TestReopenContinuesChain(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(SeverityInfo, "test", "first", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	l2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := l2.Append(SeverityWarning, "test", "second", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := l2.Close(); err != nil {
		t.Fatal(err)
	}

	if ev.Seq != 2 {
		t.Fatalf("expected seq 2 after reopen, got %d", ev.Seq)
	}
	if _, err := Verify(dir); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(SeverityInfo, "tool_exec", "original", "Write", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(SeverityInfo, "tool_exec", "second", "Write", ""); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, logFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), "second", "TAMPERED", 1)
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Verify(dir); err == nil {
		t.Fatal("expected verification to fail after tampering")
	}
}

func TestVerifyDetectsTruncation(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(SeverityInfo, "tool_exec", "one", "Write", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(SeverityInfo, "tool_exec", "two", "Write", ""); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, logFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Keep only the first line — the chain now dangles.
	lines := strings.SplitN(string(data), "\n", 2)
	if err := os.WriteFile(path, []byte(lines[0]+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Verify(dir); err == nil {
		t.Fatal("expected verification to fail after truncation")
	}
}

func TestAppendAfterCloseFails(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(SeverityInfo, "test", "x", "", ""); err == nil {
		t.Fatal("expected append after close to fail")
	}
}
