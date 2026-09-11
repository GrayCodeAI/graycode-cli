package swift

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteReportToPathPrivate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	if err := WriteReportToPath(path, sampleSnapshot()); err != nil {
		t.Fatalf("WriteReportToPath: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "# hawk swift") {
		t.Errorf("report content missing header")
	}
}

func TestWriteReportFileUniqueness(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	p1, err := WriteReportFile(sampleSnapshot())
	if err != nil {
		t.Fatalf("WriteReportFile: %v", err)
	}
	p2, err := WriteReportFile(sampleSnapshot())
	if err != nil {
		t.Fatalf("WriteReportFile: %v", err)
	}
	if p1 == p2 {
		t.Errorf("expected unique file names, got %q twice", p1)
	}
	if filepath.Dir(p1) != dir {
		t.Errorf("report %q not written to TMPDIR %q", p1, dir)
	}
	for _, p := range []string{p1, p2} {
		if !strings.HasPrefix(filepath.Base(p), "hawk-swift-") {
			t.Errorf("unexpected file name %q", p)
		}
		if !strings.HasSuffix(p, ".md") {
			t.Errorf("unexpected extension %q", p)
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestFileNameSuffixFormat(t *testing.T) {
	t.Parallel()
	s := fileNameSuffix(sampleSnapshot().Timestamp, "abc123")
	if s != "2026-08-21-120000-abc123" {
		t.Errorf("suffix = %q", s)
	}
}
