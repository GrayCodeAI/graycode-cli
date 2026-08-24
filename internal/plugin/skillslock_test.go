package plugin

import (
	"os"
	"testing"
)

func TestSkillsLockRoundTrip(t *testing.T) {
	lockPath := SkillsLockPath("user")

	lock, err := LoadSkillsLock("user")
	if err != nil {
		t.Fatalf("load missing lock: %v", err)
	}
	if len(lock.Skills) != 0 {
		t.Fatalf("expected empty lock, got %+v", lock.Skills)
	}

	lock.Set("go-review", SkillsLockEntry{
		Source:       "GrayCodeAI/hawk-community-skills",
		SourceType:   "github",
		SkillPath:    "skills/go-review/SKILL.md",
		Commit:       "abc123",
		ComputedHash: HashSkillContent([]byte("# hi")),
	})
	if err := lock.Save("user"); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(lockPath) // #nosec G304 -- test-owned temp path
	if err != nil {
		t.Fatalf("lockfile not written: %v", err)
	}
	if data[len(data)-1] != '\n' {
		t.Fatal("lockfile should end with newline")
	}

	reloaded, err := LoadSkillsLock("user")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := reloaded.Skills["go-review"]
	if !ok || got.Commit != "abc123" || got.SourceType != "github" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if reloaded.Version != skillsLockVersion {
		t.Fatalf("version should be normalized on load, got %d", reloaded.Version)
	}
}

func TestSkillsLockDelete(t *testing.T) {
	l := &SkillsLock{Skills: map[string]SkillsLockEntry{}}
	l.Set("a", SkillsLockEntry{ComputedHash: "x"})
	if !l.Delete("a") {
		t.Fatal("Delete should report removal")
	}
	if l.Delete("a") {
		t.Fatal("second Delete should be false")
	}
	if l.Delete("never-there") {
		t.Fatal("Delete of absent skill should be false")
	}
}

func TestHashSkillContentStable(t *testing.T) {
	h1 := HashSkillContent([]byte("same"))
	h2 := HashSkillContent([]byte("same"))
	if h1 != h2 || len(h1) != 64 {
		t.Fatalf("hash should be stable sha256 hex, got %q vs %q", h1, h2)
	}
	if HashSkillContent([]byte("other")) == h1 {
		t.Fatal("different content must hash differently")
	}
}
