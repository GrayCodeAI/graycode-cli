package tape

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempTape(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.fxtape")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write temp tape: %v", err)
	}
	return path
}

func TestInspectFileMetrics(t *testing.T) {
	path := writeTempTape(t, frameBytesForTest(t, &fakeClock{t: 1000}))
	st, err := InspectFile(path)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if st.Cols != 80 || st.Rows != 24 {
		t.Errorf("terminal = %dx%d, want 80x24", st.Cols, st.Rows)
	}
	if st.Version != "1.2" {
		t.Errorf("version = %q, want 1.2", st.Version)
	}
	if st.FrameCount != 2 {
		t.Errorf("FrameCount = %d, want 2", st.FrameCount)
	}
	if st.Kinds["stdout"] != 1 || st.Kinds["marker"] != 1 {
		t.Errorf("kinds = %v, want stdout:1 marker:1", st.Kinds)
	}
	if st.StdoutBytes == 0 {
		t.Error("StdoutBytes = 0, want > 0")
	}
	if st.Size != int64(len(frameBytesForTest(t, &fakeClock{t: 1000}))) {
		t.Errorf("Size mismatch")
	}
}

func TestCommitFileWritesArtifactAndRejectsDupes(t *testing.T) {
	data := frameBytesForTest(t, &fakeClock{t: 1000})
	path := writeTempTape(t, data)
	dir := t.TempDir()

	c, err := CommitFile(path, "demo", dir)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if c.Name != "demo" || c.Frames != 2 {
		t.Errorf("commit = %+v, want {name demo frames 2}", c)
	}
	if len(c.CommitID) != 12 {
		t.Errorf("CommitID = %q, want 12 hex chars", c.CommitID)
	}

	tapeBytes, err := os.ReadFile(c.Path)
	if err != nil {
		t.Fatalf("read committed tape: %v", err)
	}
	if string(tapeBytes) != string(data) {
		t.Error("committed tape bytes differ from source")
	}

	metaRaw, err := os.ReadFile(c.MetaPath)
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	var meta struct {
		Name     string `json:"name"`
		CommitID string `json:"commit_id"`
		SHA256   string `json:"sha256"`
		Frames   int    `json:"frame_count"`
	}
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	if meta.Name != "demo" || meta.CommitID != c.CommitID || meta.Frames != 2 {
		t.Errorf("meta = %+v, want name demo commit_id %s frames 2", meta, c.CommitID)
	}
	if len(meta.SHA256) != 64 {
		t.Errorf("sha256 = %q, want 64 hex", meta.SHA256)
	}

	// Duplicate name must not overwrite.
	if _, err := CommitFile(path, "demo", dir); err == nil {
		t.Fatal("expected duplicate commit to fail")
	}
}

func TestCommitValidation(t *testing.T) {
	if ValidCommitName("") || ValidCommitName("..") || ValidCommitName("a/b") || ValidCommitName("a b") {
		t.Error("invalid names accepted")
	}
	if !ValidCommitName("session-2026.01_a") {
		t.Error("valid name rejected")
	}

	// Unparseable source is rejected before any write.
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.fxtape")
	if err := os.WriteFile(bad, []byte("not a tape"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CommitFile(bad, "y", dir); err == nil {
		t.Fatal("expected commit of invalid tape to fail")
	}
	for _, p := range []string{filepath.Join(dir, "y.fxtape"), filepath.Join(dir, "y.meta.json")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("invalid tape should not write %s", p)
		}
	}
}

func TestDefaultTapesDirUsesEnv(t *testing.T) {
	t.Setenv("GRAYCODE_TAPES_DIR", "/tmp/ht")
	got := DefaultTapesDir()
	if got != "/tmp/ht" || !strings.Contains(got, "ht") {
		t.Errorf("DefaultTapesDir = %q, want env override", got)
	}
}
