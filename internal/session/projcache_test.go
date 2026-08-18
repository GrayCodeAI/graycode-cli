package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjCacheSaveLoadRoundtrip(t *testing.T) {
	c := NewProjCacheWithDir(t.TempDir())
	state := []byte(`{"fold":{"count":7}}`)

	if err := c.Save("sess-1", 42, state); err != nil {
		t.Fatal(err)
	}
	got, seq, fresh := c.Load("sess-1")
	if !fresh {
		t.Fatal("row should be fresh")
	}
	if seq != 42 {
		t.Fatalf("seq = %d, want 42", seq)
	}
	if string(got) != string(state) {
		t.Fatalf("state = %s, want %s", got, state)
	}
}

func TestProjCacheSaveOverwritesRow(t *testing.T) {
	c := NewProjCacheWithDir(t.TempDir())
	if err := c.Save("sess-1", 10, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := c.Save("sess-1", 25, []byte(`{"a":2}`)); err != nil {
		t.Fatal(err)
	}
	got, seq, fresh := c.Load("sess-1")
	if !fresh || seq != 25 || string(got) != `{"a":2}` {
		t.Fatalf("load = (%s, %d, %v), want latest row", got, seq, fresh)
	}
}

func TestProjCacheMissingAndDiscard(t *testing.T) {
	c := NewProjCacheWithDir(t.TempDir())
	if _, _, fresh := c.Load("missing"); fresh {
		t.Fatal("missing row must not be fresh")
	}
	if err := c.Save("sess-1", 1, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := c.Discard("sess-1"); err != nil {
		t.Fatal(err)
	}
	if _, _, fresh := c.Load("sess-1"); fresh {
		t.Fatal("discarded row must not be fresh")
	}
	// Discarding a missing row is not an error.
	if err := c.Discard("sess-1"); err != nil {
		t.Fatalf("discard of missing row = %v, want nil", err)
	}
}

func TestProjCacheVersionMismatchDiscards(t *testing.T) {
	dir := t.TempDir()
	c1 := NewProjCacheWithVer(dir, 1)
	c2 := NewProjCacheWithVer(dir, 2)

	if err := c1.Save("sess-1", 5, []byte(`{"v1":true}`)); err != nil {
		t.Fatal(err)
	}
	// The version-2 reader must discard, never migrate.
	if _, _, fresh := c2.Load("sess-1"); fresh {
		t.Fatal("version-mismatched row must not be fresh")
	}
	if _, err := os.Stat(filepath.Join(dir, "sess-1.projcache.json")); !os.IsNotExist(err) {
		t.Fatalf("mismatched row was not discarded (stat err = %v)", err)
	}
	// And a version-1 reader after the discard sees nothing.
	if _, _, fresh := c1.Load("sess-1"); fresh {
		t.Fatal("row should be gone after discard")
	}
}

func TestProjCacheCorruptRowFailSoft(t *testing.T) {
	dir := t.TempDir()
	c := NewProjCacheWithDir(dir)
	if err := os.WriteFile(filepath.Join(dir, "sess-1.projcache.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, fresh := c.Load("sess-1"); fresh {
		t.Fatal("corrupt row must not be fresh (fail-soft)")
	}
	// A later save self-heals the row.
	if err := c.Save("sess-1", 9, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if _, seq, fresh := c.Load("sess-1"); !fresh || seq != 9 {
		t.Fatalf("self-heal load = (_, %d, %v)", seq, fresh)
	}
}

func TestProjCacheRejectsPathTraversalIDs(t *testing.T) {
	c := NewProjCacheWithDir(t.TempDir())
	for _, id := range []string{"", "..", "../evil", "a/b", "a\\b"} {
		if err := c.Save(id, 1, []byte(`{}`)); err == nil {
			t.Fatalf("Save(%q) should error", id)
		}
		if _, _, fresh := c.Load(id); fresh {
			t.Fatalf("Load(%q) must not be fresh", id)
		}
	}
}

func TestProjCacheSidecarBesideJSONL(t *testing.T) {
	dir := t.TempDir()
	c := NewProjCacheWithDir(dir)
	if err := c.Save("sess-1", 3, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	// The record lands beside the session JSONL under the session id.
	if _, err := os.Stat(filepath.Join(dir, "sess-1.projcache.json")); err != nil {
		t.Fatalf("sidecar record missing: %v", err)
	}
	// Atomic writes must not leave temp files behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("stray temp file left behind: %s", e.Name())
		}
	}
}
