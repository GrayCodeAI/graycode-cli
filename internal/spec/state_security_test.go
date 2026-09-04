package spec

import (
	"testing"
)

func TestSpecSlugRejectsTraversal(t *testing.T) {
	for _, slug := range []string{"../escape", "nested/name", "/absolute", `..\\escape`, "", ".", ".."} {
		if _, err := specDir(t.TempDir(), slug); err == nil {
			t.Errorf("slug %q was accepted", slug)
		}
	}
}

func TestWriteStageMetaRejectsTraversal(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())
	if err := WriteStageMeta("../escape", "specify", "", ""); err == nil {
		t.Fatal("expected traversal slug to be rejected")
	}
}
