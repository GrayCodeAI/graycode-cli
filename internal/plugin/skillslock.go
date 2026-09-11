package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GrayCodeAI/hawk/internal/installtxn"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

// Skills lockfile, modeled on Autohand Code CLI's skills-lock.json:
// a per-scope record of every installed community skill with its source
// and content hash, so installs are auditable and drift is detectable.

const skillsLockVersion = 1

// SkillsLockEntry pins one installed skill.
type SkillsLockEntry struct {
	Source       string `json:"source"`               // e.g. "owner/repo"
	SourceType   string `json:"source_type"`          // "github"
	SkillPath    string `json:"skill_path,omitempty"` // path inside the repo
	Commit       string `json:"commit,omitempty"`     // HEAD sha at clone time
	ComputedHash string `json:"computed_hash"`        // sha256 of the installed SKILL.md
}

// SkillsLock is the lockfile document.
type SkillsLock struct {
	Version int                        `json:"version"`
	Skills  map[string]SkillsLockEntry `json:"skills"`
}

// SkillsLockPath returns the lockfile location for an install scope.
func SkillsLockPath(scope string) string {
	var base string
	if scope == "project" {
		base = filepath.Join(storage.ProjectStateDir("."), "skills")
	} else {
		base = filepath.Join(storage.StateDir(), "skills")
	}
	return filepath.Join(base, "skills-lock.json")
}

// LoadSkillsLock reads the lockfile for a scope; a missing file yields an
// empty lock, not an error.
func LoadSkillsLock(scope string) (*SkillsLock, error) {
	data, err := os.ReadFile(SkillsLockPath(scope)) // #nosec G304 -- path is derived from the fixed per-scope skills dir, not raw external input
	if err != nil {
		if os.IsNotExist(err) {
			return &SkillsLock{Version: skillsLockVersion, Skills: map[string]SkillsLockEntry{}}, nil
		}
		return nil, fmt.Errorf("read skills lock: %w", err)
	}
	var lock SkillsLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parse skills lock: %w", err)
	}
	if lock.Skills == nil {
		lock.Skills = map[string]SkillsLockEntry{}
	}
	lock.Version = skillsLockVersion
	return &lock, nil
}

// Save writes the lockfile atomically.
func (l *SkillsLock) Save(scope string) error {
	l.Version = skillsLockVersion
	path := SkillsLockPath(scope)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("encode skills lock: %w", err)
	}
	return installtxn.WriteFileAtomically(path, append(data, '\n'), 0o600)
}

// Set records or updates the entry for one skill.
func (l *SkillsLock) Set(name string, entry SkillsLockEntry) {
	if l.Skills == nil {
		l.Skills = map[string]SkillsLockEntry{}
	}
	l.Skills[name] = entry
}

// Delete removes a skill from the lock; returns false when absent.
func (l *SkillsLock) Delete(name string) bool {
	if _, ok := l.Skills[name]; !ok {
		return false
	}
	delete(l.Skills, name)
	return true
}

// HashSkillContent hashes the final SKILL.md bytes written to disk.
func HashSkillContent(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
