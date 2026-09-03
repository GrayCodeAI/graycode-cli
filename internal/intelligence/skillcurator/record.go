package skillcurator

import (
	"path/filepath"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

// RecordSkillUsage is a best-effort, non-blocking integration point for the
// Skill tool: it records a skill invocation in the user-scoped curator so the
// auto-archive review has accurate usage data. It never errors — skill
// execution must not be interrupted by curator bookkeeping.
func RecordSkillUsage(name string) {
	if name == "" {
		return
	}
	dir := filepath.Join(storage.StateDir(), "skills")
	c, err := New(Config{SkillsDir: dir, StateFile: filepath.Join(dir, ".curator_state.json")})
	if err != nil {
		return
	}
	c.RecordUse(name)
}
