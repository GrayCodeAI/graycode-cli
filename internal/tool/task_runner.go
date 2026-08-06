package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// taskStoreSnapshot is the on-disk representation of a TaskStore.
type taskStoreSnapshot struct {
	Version int     `json:"version"`
	Next    int     `json:"next"`
	Tasks   []*Task `json:"tasks"`
}

// persistDir is the directory a TaskStore writes its snapshot to, or "" when
// persistence is disabled. Stored on the store so mutations can auto-save.
type persistState struct {
	dir string
}

// EnablePersistence turns on disk persistence for the store. The directory is
// created if missing. While enabled, every mutation writes an atomic snapshot
// to <dir>/tasks.json so a TaskRunner can resume across process restarts.
// Persistence is opt-in: the global store used by interactive sessions stays
// in-memory unless a caller explicitly enables it.
func (s *TaskStore) EnablePersistence(dir string) error {
	if dir == "" {
		return fmt.Errorf("task: persistence directory is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("task: create persistence dir: %w", err)
	}
	s.mu.Lock()
	s.persist = &persistState{dir: dir}
	s.mu.Unlock()
	return s.Save("")
}

// DisablePersistence turns off disk persistence, leaving the in-memory state
// untouched.
func (s *TaskStore) DisablePersistence() {
	s.mu.Lock()
	s.persist = nil
	s.mu.Unlock()
}

// PersistDir returns the active persistence directory, or "" if disabled.
func (s *TaskStore) PersistDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.persist == nil {
		return ""
	}
	return s.persist.dir
}

// Save writes an atomic snapshot of the store to path. Use Save() with a
// configured persistence dir, or Save(path) directly.
func (s *TaskStore) Save(path string) error {
	if path == "" {
		s.mu.RLock()
		p := s.persist
		s.mu.RUnlock()
		if p == nil {
			return nil
		}
		path = filepath.Join(p.dir, "tasks.json")
	}
	// Snapshot under lock so the file always reflects one consistent state.
	s.mu.RLock()
	snap := taskStoreSnapshot{
		Version: 1,
		Next:    s.next,
		Tasks:   make([]*Task, 0, len(s.tasks)),
	}
	for _, t := range s.tasks {
		snap.Tasks = append(snap.Tasks, cloneScheduledTask(t))
	}
	s.mu.RUnlock()
	sort.Slice(snap.Tasks, func(i, j int) bool { return snap.Tasks[i].ID < snap.Tasks[j].ID })

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("task: encode snapshot: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("task: ensure snapshot dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("task: write snapshot: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("task: commit snapshot: %w", err)
	}
	return nil
}

// Load replaces the store contents from an on-disk snapshot, preserving the
// id counter. It is the resume entry point for a persisted TaskRunner.
func (s *TaskStore) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("task: read snapshot: %w", err)
	}
	var snap taskStoreSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("task: decode snapshot: %w", err)
	}
	if snap.Version != 1 {
		return fmt.Errorf("task: unsupported snapshot version %d", snap.Version)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = make(map[string]*Task, len(snap.Tasks))
	for _, t := range snap.Tasks {
		if t == nil || t.ID == "" {
			return fmt.Errorf("task: snapshot contains an invalid task")
		}
		s.tasks[t.ID] = t
	}
	s.next = snap.Next
	if s.next == 0 {
		// Backfill the counter from the largest numeric id so new tasks never
		// collide with loaded ones.
		for id := range s.tasks {
			var n int
			if _, err := fmt.Sscanf(id, "task_%d", &n); err == nil && n > s.next {
				s.next = n
			}
		}
	}
	return nil
}

// persistOnMutation writes the snapshot after a state change when persistence
// is enabled. Callers invoke it while holding no store lock; it is a no-op
// when disabled.
func (s *TaskStore) persistOnMutation() {
	if s.PersistDir() == "" {
		return
	}
	_ = s.Save("")
}

// MarkFailed records a failure and applies the retry budget. When the task
// still has attempts left it is requeued to pending (with the backoff tick in
// metadata) so the runner can pick it up again; once the budget is exhausted
// the task is parked in failed for replanning. Returns whether the task was
// requeued.
func (s *TaskStore) MarkFailed(id, errMsg string) (bool, error) {
	s.mu.Lock()
	t, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return false, fmt.Errorf("task %q not found", id)
	}
	t.Attempts++
	t.LastError = errMsg
	t.UpdatedAt = time.Now()
	requeued := false
	switch t.Status {
	case TaskStatusCompleted:
		t.Status = TaskStatusCompleted
	case TaskStatusInProgress:
		if t.Attempts < t.EffectiveMaxAttempts() {
			t.Status = TaskStatusPending
			if t.Metadata == nil {
				t.Metadata = make(map[string]any)
			}
			t.Metadata["retryBackoffTick"] = t.Attempts
			requeued = true
		} else {
			t.Status = TaskStatusFailed
		}
	case TaskStatusPending:
		// A not-yet-started task that failed during planning: treat as failed.
		t.Status = TaskStatusFailed
	default:
		t.Status = TaskStatusFailed
	}
	persist := s.persist
	s.mu.Unlock()
	if persist != nil {
		_ = s.Save("")
	}
	return requeued, nil
}

// Requeue resets a failed task to pending and clears its last error so the
// runner may attempt it again (replan path). The attempt counter is reset so
// a replan starts with a fresh retry budget.
func (s *TaskStore) Requeue(id string) (bool, error) {
	s.mu.Lock()
	t, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return false, fmt.Errorf("task %q not found", id)
	}
	t.Status = TaskStatusPending
	t.Attempts = 0
	t.LastError = ""
	t.UpdatedAt = time.Now()
	persist := s.persist
	s.mu.Unlock()
	if persist != nil {
		_ = s.Save("")
	}
	return true, nil
}

// Checkpoint merges resumable progress onto a task without changing its
// status. A replan or resume reads the checkpoint to avoid starting from zero.
func (s *TaskStore) Checkpoint(id string, data map[string]any) (bool, error) {
	s.mu.Lock()
	t, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return false, fmt.Errorf("task %q not found", id)
	}
	if t.Checkpoint == nil {
		t.Checkpoint = make(map[string]any)
	}
	for k, v := range data {
		t.Checkpoint[k] = v
	}
	t.UpdatedAt = time.Now()
	persist := s.persist
	s.mu.Unlock()
	if persist != nil {
		_ = s.Save("")
	}
	return true, nil
}

// FailedTasks returns tasks parked in the failed state, most recently updated
// first, for replanning input.
func (s *TaskStore) FailedTasks() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.Status == TaskStatusFailed {
			out = append(out, cloneScheduledTask(t))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}
