// Package harness implements a continual, evidence-backed refinement store
// adopted from Prime Agent's Continual Harness: supplemental agent state
// (prompts, memories, skill descriptions, subagent specs) is stored as
// versioned entries that the agent can refine through small, evidence-backed
// updates, with a recorded refinement history and snapshot-based rollback.
//
// It never rewrites the immutable base system prompt — only this supplemental
// state. Entries are keyed by kind; each update bumps the entry version and
// appends a refinement event (trigger -> changes -> evidence -> outcome) so
// every change is auditable and reversible.
package harness

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

// Kind is the category of supplemental state.
type Kind string

const (
	KindPrompt   Kind = "prompt"
	KindMemory   Kind = "memory"
	KindSkill    Kind = "skill"
	KindSubagent Kind = "subagent"
)

func (k Kind) Valid() bool {
	switch k {
	case KindPrompt, KindMemory, KindSkill, KindSubagent:
		return true
	}
	return false
}

// Scope is where an entry applies.
type Scope string

const (
	ScopeLocal  Scope = "local"
	ScopeGlobal Scope = "global"
)

// Entry is one versioned supplemental state item.
type Entry struct {
	ID        string    `json:"id"`
	Kind      Kind      `json:"kind"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Path      string    `json:"path,omitempty"`
	Scope     Scope     `json:"scope,omitempty"`
	Version   int       `json:"version"`
	Evidence  string    `json:"evidence,omitempty"`
	Source    string    `json:"source,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Refinement is one recorded update event.
type Refinement struct {
	ID        string    `json:"id"`
	Trigger   string    `json:"trigger"`
	Changes   []string  `json:"changes"`
	Evidence  string    `json:"evidence"`
	Outcome   string    `json:"outcome,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Snapshot is a point-in-time copy of all entries, used for rollback.
type Snapshot struct {
	Schema  int     `json:"schema"`
	Entries []Entry `json:"entries"`
}

// Store is a thread-safe continual harness.
type Store struct {
	mu          sync.Mutex
	entries     map[Kind]map[string]*Entry
	refinements []Refinement
	dir         string
}

// New creates a Store persisted under dir ("" disables persistence).
func New(dir string) (*Store, error) {
	s := &Store{
		entries: map[Kind]map[string]*Entry{
			KindPrompt: {}, KindMemory: {}, KindSkill: {}, KindSubagent: {},
		},
		dir: dir,
	}
	if dir != "" {
		if err := s.load(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) path() string {
	if s.dir == "" {
		return ""
	}
	return filepath.Join(s.dir, "harness.json")
}

func (s *Store) load() error {
	raw, err := os.ReadFile(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("harness: load: %w", err)
	}
	var st struct {
		Schema  int          `json:"schema"`
		Entries []Entry      `json:"entries"`
		History []Refinement `json:"history"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return fmt.Errorf("harness: parse: %w", err)
	}
	for _, e := range st.Entries {
		if s.entries[e.Kind] == nil {
			s.entries[e.Kind] = map[string]*Entry{}
		}
		cp := e
		s.entries[e.Kind][e.ID] = &cp
	}
	s.refinements = st.History
	return nil
}

func (s *Store) save() error {
	if s.dir == "" {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return err
	}
	var all []Entry
	for _, m := range s.entries {
		for _, e := range m {
			all = append(all, *e)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Kind != all[j].Kind {
			return all[i].Kind < all[j].Kind
		}
		return all[i].Title < all[j].Title
	})
	data, err := json.MarshalIndent(struct {
		Schema  int          `json:"schema"`
		Entries []Entry      `json:"entries"`
		History []Refinement `json:"history"`
	}{Schema: 1, Entries: all, History: s.refinements}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path())
}

// Create adds a new entry and records a refinement event.
func (s *Store) Create(kind Kind, title, content, evidence, source string) (*Entry, error) {
	if !kind.Valid() {
		return nil, fmt.Errorf("harness: invalid kind %q", kind)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := slug(kind, title)
	e := &Entry{
		ID: id, Kind: kind, Title: title, Content: content,
		Scope: ScopeLocal, Version: 1, Evidence: evidence, Source: source,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.entries[kind][id] = e
	s.recordLocked("create", kind, id, evidence, "")
	return s.saveThen(e)
}

// Refine updates an existing entry (or creates it if absent), bumping the
// version and recording an evidence-backed refinement event.
func (s *Store) Refine(kind Kind, title, content, evidence string) (*Entry, error) {
	if !kind.Valid() {
		return nil, fmt.Errorf("harness: invalid kind %q", kind)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := slug(kind, title)
	now := time.Now()
	if e, ok := s.entries[kind][id]; ok {
		e.Content = content
		e.Version++
		e.Evidence = evidence
		e.UpdatedAt = now
		s.recordLocked("update", kind, id, evidence, "")
		return s.saveThen(e)
	}
	e := &Entry{
		ID: id, Kind: kind, Title: title, Content: content,
		Scope: ScopeLocal, Version: 1, Evidence: evidence, CreatedAt: now, UpdatedAt: now,
	}
	s.entries[kind][id] = e
	s.recordLocked("create", kind, id, evidence, "")
	return s.saveThen(e)
}

// Delete removes an entry, recording the event.
func (s *Store) Delete(kind Kind, title, evidence string) error {
	if !kind.Valid() {
		return fmt.Errorf("harness: invalid kind %q", kind)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := slug(kind, title)
	if _, ok := s.entries[kind][id]; !ok {
		return fmt.Errorf("harness: %s %q not found", kind, title)
	}
	delete(s.entries[kind], id)
	s.recordLocked("delete", kind, id, evidence, "")
	return s.save()
}

// Get returns a copy of an entry.
func (s *Store) Get(kind Kind, title string) (*Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[kind][slug(kind, title)]
	if !ok {
		return nil, false
	}
	cp := *e
	return &cp, true
}

// List returns all entries of a kind, sorted by title.
func (s *Store) List(kind Kind) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Entry
	for _, e := range s.entries[kind] {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

// History returns the recorded refinement events, oldest first.
func (s *Store) History() []Refinement {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Refinement{}, s.refinements...)
}

// Snapshot returns a point-in-time copy of all entries (for rollback).
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []Entry
	for _, m := range s.entries {
		for _, e := range m {
			all = append(all, *e)
		}
	}
	return Snapshot{Schema: 1, Entries: all}
}

// Restore replaces all entries with a snapshot, recording a rollback event.
func (s *Store) Restore(snap Snapshot, evidence string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.entries {
		s.entries[k] = map[string]*Entry{}
	}
	for i := range snap.Entries {
		e := snap.Entries[i]
		if s.entries[e.Kind] == nil {
			s.entries[e.Kind] = map[string]*Entry{}
		}
		cp := e
		s.entries[e.Kind][e.ID] = &cp
	}
	s.recordLocked("rollback", "", "", evidence, fmt.Sprintf("%d entries", len(snap.Entries)))
	return s.save()
}

func (s *Store) recordLocked(action string, kind Kind, id, evidence, outcome string) {
	s.refinements = append(s.refinements, Refinement{
		ID:        fmt.Sprintf("r-%d", len(s.refinements)+1),
		Trigger:   action + " " + string(kind) + " " + id,
		Changes:   []string{action + ":" + string(kind) + ":" + id},
		Evidence:  evidence,
		Outcome:   outcome,
		CreatedAt: time.Now(),
	})
}

func (s *Store) saveThen(e *Entry) (*Entry, error) {
	if err := s.save(); err != nil {
		return nil, err
	}
	cp := *e
	return &cp, nil
}

// slug builds a stable entry id from kind + title.
func slug(kind Kind, title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	t = strings.ReplaceAll(t, " ", "-")
	t = strings.ReplaceAll(t, "/", "-")
	return string(kind) + ":" + t
}
