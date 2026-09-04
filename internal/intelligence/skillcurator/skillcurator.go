// Package skillcurator maintains a coding agent's skill collection over time,
// adopting Hermes Agent's curator design. It records per-skill usage, runs an
// inactivity-triggered review, and auto-transitions agent-created skills
// through lifecycle states (Active -> Archived) under hard invariants:
//
//   - only agent-created skills are ever touched (installed third-party skills
//     are left alone);
//   - nothing is ever deleted — archiving moves the skill to a recoverable
//     .archive/ directory;
//   - explicitly pinned skills bypass all auto-transitions;
//   - the review is best-effort and never blocks the agent.
package skillcurator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Status is a skill's lifecycle state.
type Status string

const (
	StatusActive   Status = "active"
	StatusPinned   Status = "pinned"
	StatusArchived Status = "archived"
)

// Skill is the curator's view of one agent-created skill file.
type Skill struct {
	Name     string
	Path     string // current location of SKILL.md (or its dir)
	Category string
	Version  string
	Status   Status
	LastUsed time.Time
	UseCount int
}

// Config controls the curator's behavior.
type Config struct {
	// SkillsDir is the user-scoped agent-created skills directory.
	SkillsDir string
	// StateFile is where curator + usage state persists. Defaults to
	// <SkillsDir>/.curator_state.json.
	StateFile string
	// IdleDaysBeforeArchive archives an Active skill unused this long.
	IdleDaysBeforeArchive int
	// IntervalHours between auto reviews (inactivity-triggered).
	IntervalHours int
}

func (c *Config) normalize() {
	if c.SkillsDir == "" {
		c.SkillsDir = "~/.graycode/skills"
	}
	if c.IdleDaysBeforeArchive <= 0 {
		c.IdleDaysBeforeArchive = 30
	}
	if c.IntervalHours <= 0 {
		c.IntervalHours = 7 * 24 // weekly
	}
	if c.StateFile == "" {
		c.StateFile = filepath.Join(c.SkillsDir, ".curator_state.json")
	}
}

// Curator tracks usage and runs lifecycle transitions over agent-created
// skills. Thread-safe.
type Curator struct {
	mu     sync.Mutex
	cfg    Config
	state  state
	pinned map[string]bool // resolved at load from state.Pinned
}

type state struct {
	Version   int               `json:"version"`
	LastRunAt time.Time         `json:"last_run_at,omitempty"`
	Pinned    []string          `json:"pinned,omitempty"`
	Usage     map[string]*Usage `json:"usage,omitempty"`
}

// Usage records observed use of a skill.
type Usage struct {
	LastUsed time.Time `json:"last_used"`
	UseCount int       `json:"use_count"`
}

// New creates a curator and loads persisted state. Missing state is fine.
func New(cfg Config) (*Curator, error) {
	cfg.normalize()
	c := &Curator{cfg: cfg, state: state{Usage: map[string]*Usage{}}}
	c.pinned = map[string]bool{}
	if err := c.load(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Curator) load() error {
	raw, err := os.ReadFile(c.cfg.StateFile) // #nosec G304 -- curator-owned state path
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var st state
	if err := json.Unmarshal(raw, &st); err != nil {
		return fmt.Errorf("skillcurator: parse state: %w", err)
	}
	if st.Usage == nil {
		st.Usage = map[string]*Usage{}
	}
	c.state = st
	for _, p := range st.Pinned {
		c.pinned[p] = true
	}
	return nil
}

func (c *Curator) save() error {
	c.state.Pinned = nil
	for p := range c.pinned {
		c.state.Pinned = append(c.state.Pinned, p)
	}
	sort.Strings(c.state.Pinned)
	data, err := json.MarshalIndent(c.state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.cfg.StateFile), 0o750); err != nil {
		return err
	}
	tmp := c.cfg.StateFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // #nosec G306 -- curator-owned state
		return err
	}
	return os.Rename(tmp, c.cfg.StateFile)
}

// RecordUse records that a skill was invoked now. It lazily marks the skill
// active on first use. Never touches installed (non-agent) skills' files.
func (c *Curator) RecordUse(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	u, ok := c.state.Usage[name]
	if !ok {
		u = &Usage{}
		c.state.Usage[name] = u
	}
	u.LastUsed = time.Now()
	u.UseCount++
	_ = c.save()
}

// Pin pins a skill so auto-transitions never touch it.
func (c *Curator) Pin(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pinned[name] = true
	return c.save()
}

// Unpin removes a pin.
func (c *Curator) Unpin(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pinned, name)
	return c.save()
}

// Archive moves a skill's directory into a recoverable .archive/ subfolder.
// It refuses to archive pinned skills and refuses to delete anything.
func (c *Curator) Archive(name, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pinned[name] {
		return fmt.Errorf("skillcurator: %s is pinned; refuse to archive", name)
	}
	src := filepath.Join(c.cfg.SkillsDir, name)
	st, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("skillcurator: skill %s not found: %w", name, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("skillcurator: %s is not a skill directory", name)
	}
	archiveRoot := filepath.Join(c.cfg.SkillsDir, ".archive")
	if err := os.MkdirAll(archiveRoot, 0o750); err != nil {
		return err
	}
	dst := filepath.Join(archiveRoot, name)
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	// Record the archive in state so it stays discoverable/recoverable.
	if c.state.Usage == nil {
		c.state.Usage = map[string]*Usage{}
	}
	if _, ok := c.state.Usage[name]; !ok {
		c.state.Usage[name] = &Usage{}
	}
	c.state.Usage[name].LastUsed = time.Now()
	_ = reason
	return c.save()
}

// List enumerates agent-created skills under the skills dir with their status.
func (c *Curator) List() ([]Skill, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, err := os.ReadDir(c.cfg.SkillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue // .archive, .curator_state, etc.
		}
		s := Skill{
			Name:   name,
			Path:   filepath.Join(c.cfg.SkillsDir, name),
			Status: StatusActive,
		}
		if u, ok := c.state.Usage[name]; ok {
			s.LastUsed = u.LastUsed
			s.UseCount = u.UseCount
		}
		if c.pinned[name] {
			s.Status = StatusPinned
		}
		if meta := readFrontmatter(filepath.Join(s.Path, "SKILL.md")); meta != nil {
			s.Category = meta.category
			s.Version = meta.version
		}
		out = append(out, s)
	}
	// Include archived skills (recoverable) from .archive/.
	archiveDir := filepath.Join(c.cfg.SkillsDir, ".archive")
	if ae, aerr := os.ReadDir(archiveDir); aerr == nil {
		for _, e := range ae {
			if !e.IsDir() {
				continue
			}
			s := Skill{Name: e.Name(), Path: filepath.Join(archiveDir, e.Name()), Status: StatusArchived}
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// MaybeRun is the inactivity-triggered review: it runs only when the last run
// is older than IntervalHours, then archives unused Active skills.
func (c *Curator) MaybeRun(now time.Time) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.state.LastRunAt.IsZero() && now.Sub(c.state.LastRunAt) < time.Duration(c.cfg.IntervalHours)*time.Hour {
		return nil, nil // not due
	}
	c.state.LastRunAt = now
	// Unlock for the file walk; re-lock for mutations.
	archived, err := c.reviewLocked(now)
	if err != nil {
		return nil, err
	}
	if serr := c.save(); serr != nil {
		return archived, serr
	}
	return archived, nil
}

// reviewLocked runs the auto-transition pass. The caller holds the lock.
func (c *Curator) reviewLocked(now time.Time) ([]string, error) {
	threshold := now.AddDate(0, 0, -c.cfg.IdleDaysBeforeArchive)
	var archived []string
	entries, err := os.ReadDir(c.cfg.SkillsDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		name := e.Name()
		if c.pinned[name] {
			continue
		}
		u, ok := c.state.Usage[name]
		if !ok {
			continue // never-used agent skill: leave it (conservative)
		}
		if u.LastUsed.Before(threshold) {
			// Only auto-archive skills the agent actually created and that have
			// seen use in the past (use count > 0) but have since gone cold.
			if u.UseCount > 0 {
				if err := c.moveToArchiveLocked(name); err != nil {
					continue
				}
				archived = append(archived, name)
			}
		}
	}
	return archived, nil
}

func (c *Curator) moveToArchiveLocked(name string) error {
	src := filepath.Join(c.cfg.SkillsDir, name)
	archiveRoot := filepath.Join(c.cfg.SkillsDir, ".archive")
	if err := os.MkdirAll(archiveRoot, 0o750); err != nil {
		return err
	}
	dst := filepath.Join(archiveRoot, name)
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return os.Rename(src, dst)
	}
	return nil
}

// frontmatter holds the minimal SKILL.md YAML keys the curator reads.
type frontmatter struct {
	category string
	version  string
}

// readFrontmatter parses the leading --- frontmatter block of a SKILL.md.
func readFrontmatter(path string) *frontmatter {
	raw, err := os.ReadFile(path) // #nosec G304 -- skill file under curated dir
	if err != nil {
		return nil
	}
	text := string(raw)
	if !strings.HasPrefix(text, "---") {
		return nil
	}
	rest := text[3:]
	if idx := strings.Index(rest, "---"); idx < 0 {
		return nil
	} else {
		rest = rest[:idx]
	}
	fm := &frontmatter{}
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := cutKV(line, "category"); ok {
			fm.category = v
		}
		if v, ok := cutKV(line, "version"); ok {
			fm.version = v
		}
	}
	return fm
}

func cutKV(line, key string) (string, bool) {
	kv := strings.TrimSpace(line)
	if !strings.HasPrefix(kv, key+":") {
		return "", false
	}
	v := strings.TrimSpace(strings.TrimPrefix(kv, key+":"))
	v = strings.Trim(v, `"'`)
	return v, true
}

// ForceReview runs the auto-transition review immediately, ignoring the
// inactivity interval (used by explicit CLI invocations). Returns the names
// of archived skills.
func (c *Curator) ForceReview() ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.LastRunAt = time.Now()
	archived, err := c.reviewLocked(time.Now())
	if err != nil {
		return nil, err
	}
	if serr := c.save(); serr != nil {
		return archived, serr
	}
	return archived, nil
}
