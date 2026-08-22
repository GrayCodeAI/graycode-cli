package fastcopy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTreeCopiesAllFiles(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	writeTree(t, src, map[string]string{
		"a.txt":          "alpha",
		"sub/b.txt":      "beta",
		"sub/deep/c.txt": "gamma",
		"other/d.txt":    stringsRepeat("x", 10000),
	})
	files, bytes, err := Tree(context.Background(), src, dst)
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if files != 4 {
		t.Fatalf("files = %d, want 4", files)
	}
	if bytes < 10010 {
		t.Fatalf("bytes = %d", bytes)
	}
	for _, rel := range []string{"a.txt", "sub/b.txt", "sub/deep/c.txt", "other/d.txt"} {
		got, err := os.ReadFile(filepath.Join(dst, rel)) // #nosec G304 -- test-owned path
		if err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
		want, _ := os.ReadFile(filepath.Join(src, rel)) // #nosec G304 -- test-owned path
		if string(got) != string(want) {
			t.Fatalf("%s content mismatch", rel)
		}
	}
}

func TestTreeEmptyDir(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	files, _, err := Tree(context.Background(), src, dst)
	if err != nil || files != 0 {
		t.Fatalf("files=%d err=%v", files, err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("dst not created: %v", err)
	}
}

func TestTreeSrcMustExist(t *testing.T) {
	if _, _, err := Tree(context.Background(), "/nonexistent-fastcopy-src", t.TempDir()); err == nil {
		t.Fatal("expected error for missing src")
	}
}

func TestShardForDeterministic(t *testing.T) {
	a := shardFor("sub")
	b := shardFor("sub")
	c := shardFor("other")
	if a != b || c >= shardCount || a >= shardCount {
		t.Fatalf("sharding broken: %d %d %d", a, b, c)
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
