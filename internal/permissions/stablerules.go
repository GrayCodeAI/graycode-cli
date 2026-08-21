package permissions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/permissions/stableid"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

// StableRuleStore persists exact, stable-id permission rules (ports fx's
// session-permission state) per project, alongside the glob-based rules.
//
// Unlike the glob rules (which are re-derived pattern matches), each exact
// rule carries a stable id and generation that survive workspace changes, so
// it can be listed and revoked by id without re-deriving the rule from the
// current workspace.
type StableRuleStore struct {
	path  string
	mu    sync.RWMutex
	state stableid.State
}

// DefaultStableRulesPath returns the default persisted state path for a
// project directory, next to the glob-rules permissions.json.
func DefaultStableRulesPath(projectDir string) string {
	return filepath.Join(storage.ProjectStateDir(projectDir), "stable-rules.json")
}

// NewStableRuleStore returns an empty store persisted at path.
func NewStableRuleStore(path string) *StableRuleStore {
	return &StableRuleStore{path: path, state: stableid.NewState()}
}

type stableRuleFile struct {
	NextGeneration uint64    `json:"next_generation"`
	Rules          []ruleDoc `json:"rules"`
}

type ruleDoc struct {
	ID              uint64 `json:"id"`
	Kind            int    `json:"kind"`
	Canonical       string `json:"canonical"`
	DisplayIdentity string `json:"display_identity"`
	Decision        int    `json:"decision"`
	Generation      uint64 `json:"generation"`
}

// Load reads the persisted state from disk. A missing file is the empty state.
// Malformed content produces an error without corrupting the in-memory state.
func (s *StableRuleStore) Load() error {
	if s == nil {
		return nil
	}
	data, err := os.ReadFile(s.path) // #nosec G304 -- path is the caller-supplied stable-rules.json path (see DefaultStableRulesPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.mu.Lock()
			s.state = stableid.NewState()
			s.mu.Unlock()
			return nil
		}
		return fmt.Errorf("read stable rules: %w", err)
	}
	var file stableRuleFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("unmarshal stable rules: %w", err)
	}
	state := stableid.State{NextGeneration: file.NextGeneration}
	if state.NextGeneration == 0 {
		state.NextGeneration = 1
	}
	for _, d := range file.Rules {
		key, ok := stableid.NewKey(stableid.Kind(d.Kind), d.Canonical)
		if !ok {
			return fmt.Errorf("stable rules: invalid key for rule %d", d.ID)
		}
		decision := stableid.Deny
		if d.Decision == int(stableid.Allow) {
			decision = stableid.Allow
		}
		state.Rules = append(state.Rules, stableid.Rule{
			ID:              d.ID,
			Key:             key,
			DisplayIdentity: d.DisplayIdentity,
			Decision:        decision,
			Generation:      d.Generation,
		})
	}
	if err := stableid.Validate(state); err != nil {
		return err
	}
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
	return nil
}

// Save persists the current state atomically.
func (s *StableRuleStore) Save() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	file := stableRuleFile{NextGeneration: s.state.NextGeneration}
	for _, r := range s.state.Rules {
		file.Rules = append(file.Rules, ruleDoc{
			ID: r.ID, Kind: int(r.Key.Kind), Canonical: r.Key.Canonical,
			DisplayIdentity: r.DisplayIdentity,
			Decision:        int(r.Decision), Generation: r.Generation,
		})
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal stable rules: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create stable rules directory: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // #nosec G306 -- rule store is user-owned policy data
		return fmt.Errorf("write stable rules temp file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename stable rules file: %w", err)
	}
	return nil
}

// Remember upserts an exact rule and returns its stable id. ok=false when the
// identity is invalid, the state is stale, or the store is full (fx
// invalid/stale/full outcomes).
func (s *StableRuleStore) Remember(kind stableid.Kind, canonical, displayIdentity string, decision stableid.Decision) (uint64, bool) {
	if s == nil {
		return 0, false
	}
	key, ok := stableid.NewKey(kind, canonical)
	if !ok {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Upsert an existing exact rule by matching its current generation.
	var expected *uint64
	if r, ok := stableid.RuleForKey(s.state, key); ok {
		gen := r.Generation
		expected = &gen
	}
	next, status := stableid.ApplySet(s.state, stableid.SetEvent{
		Key: key, DisplayIdentity: displayIdentity, Decision: decision, ExpectedGeneration: expected,
	})
	if status != stableid.Applied {
		return 0, false
	}
	s.state = next
	rule, _ := stableid.RuleForKey(s.state, key)
	return rule.ID, true
}

// Revoke removes the rule with the given stable id. Returns false when no such
// rule exists (fx stale outcome).
func (s *StableRuleStore) Revoke(id uint64) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rule, ok := stableid.RuleForID(s.state, id)
	if !ok {
		return false
	}
	next, status := stableid.ApplyRevoke(s.state, stableid.RevokeEvent{
		ID: id, ExpectedGeneration: rule.Generation,
	})
	if status != stableid.Applied {
		return false
	}
	s.state = next
	return true
}

// List returns all exact rules ordered by stable id.
func (s *StableRuleStore) List() []stableid.RuleSnap {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return stableid.Sorted(s.state)
}

// Resolve reports the decision for an exact key, or ok=false when unresolved.
func (s *StableRuleStore) Resolve(kind stableid.Kind, canonical string) (stableid.Decision, bool) {
	if s == nil {
		return stableid.Deny, false
	}
	key, ok := stableid.NewKey(kind, canonical)
	if !ok {
		return stableid.Deny, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return stableid.Decide(s.state, key)
}
