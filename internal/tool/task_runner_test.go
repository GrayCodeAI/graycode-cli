package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := &TaskStore{tasks: make(map[string]*Task)}
	if err := store.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	a := store.Create("a", "do a", "doing a", nil)
	b := store.CreateWithParent("b", "do b", "doing b", nil, a.ID)
	store.Update(a.ID, func(ta *Task) { ta.Status = TaskStatusCompleted })
	store.Checkpoint(b.ID, map[string]any{"phase": "parse"})

	loaded := &TaskStore{tasks: make(map[string]*Task)}
	if err := loaded.Load(filepath.Join(dir, "tasks.json")); err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Get(a.ID)
	if !ok {
		t.Fatalf("task %q missing after load", a.ID)
	}
	if got.Status != TaskStatusCompleted {
		t.Fatalf("expected completed status after load, got %q", got.Status)
	}
	gotB, ok := loaded.Get(b.ID)
	if !ok {
		t.Fatalf("task %q missing after load", b.ID)
	}
	if gotB.Checkpoint["phase"] != "parse" {
		t.Fatalf("checkpoint not preserved: %+v", gotB.Checkpoint)
	}
	if gotB.ParentID != a.ID {
		t.Fatalf("parent link not preserved: %q", gotB.ParentID)
	}
	// New task ids must not collide with loaded ones.
	c := loaded.Create("c", "do c", "doing c", nil)
	if c.ID == a.ID || c.ID == b.ID {
		t.Fatalf("id counter not preserved: %q", c.ID)
	}
}

func TestEnablePersistencePersistsMutations(t *testing.T) {
	dir := t.TempDir()
	store := &TaskStore{tasks: make(map[string]*Task)}
	if err := store.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	store.Create("a", "do a", "doing a", nil)

	data, err := os.ReadFile(filepath.Join(dir, "tasks.json"))
	if err != nil {
		t.Fatalf("expected snapshot after create: %v", err)
	}
	var snap taskStoreSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks) != 1 {
		t.Fatalf("expected 1 persisted task, got %d", len(snap.Tasks))
	}
}

func TestMarkFailedRequeueAndExhaust(t *testing.T) {
	store := &TaskStore{tasks: make(map[string]*Task)}
	task := store.Create("a", "do a", "doing a", nil)
	store.Update(task.ID, func(ta *Task) {
		ta.Status = TaskStatusInProgress
		ta.MaxAttempts = 2
	})

	requeued, err := store.MarkFailed(task.ID, "boom")
	if err != nil {
		t.Fatal(err)
	}
	if !requeued {
		t.Fatal("attempt 1 of 2 should requeue")
	}
	got, _ := store.Get(task.ID)
	if got.Status != TaskStatusPending || got.Attempts != 1 {
		t.Fatalf("expected pending with 1 attempt, got %+v", got)
	}
	if got.LastError != "boom" {
		t.Fatalf("expected lastError recorded, got %q", got.LastError)
	}

	store.Update(task.ID, func(ta *Task) { ta.Status = TaskStatusInProgress })
	requeued, err = store.MarkFailed(task.ID, "boom again")
	if err != nil {
		t.Fatal(err)
	}
	if requeued {
		t.Fatal("attempt 2 of 2 should exhaust the budget")
	}
	got, _ = store.Get(task.ID)
	if got.Status != TaskStatusFailed || got.Attempts != 2 {
		t.Fatalf("expected failed with 2 attempts, got %+v", got)
	}
}

func TestRequeueResetsBudget(t *testing.T) {
	store := &TaskStore{tasks: make(map[string]*Task)}
	task := store.Create("a", "do a", "doing a", nil)
	store.Update(task.ID, func(ta *Task) {
		ta.Status = TaskStatusInProgress
		ta.MaxAttempts = 1
	})
	store.MarkFailed(task.ID, "boom")

	ok, err := store.Requeue(task.ID)
	if err != nil || !ok {
		t.Fatalf("requeue failed: %v", err)
	}
	got, _ := store.Get(task.ID)
	if got.Status != TaskStatusPending || got.Attempts != 0 || got.LastError != "" {
		t.Fatalf("requeue should reset budget: %+v", got)
	}
}

func TestFailedTasksListsForReplan(t *testing.T) {
	store := &TaskStore{tasks: make(map[string]*Task)}
	a := store.Create("a", "do a", "doing a", nil)
	store.Update(a.ID, func(ta *Task) {
		ta.Status = TaskStatusInProgress
		ta.MaxAttempts = 1
	})
	store.MarkFailed(a.ID, "boom")

	failed := store.FailedTasks()
	if len(failed) != 1 || failed[0].ID != a.ID {
		t.Fatalf("expected 1 failed task, got %+v", failed)
	}
	if failed[0].LastError != "boom" {
		t.Fatalf("expected lastError on failed task, got %q", failed[0].LastError)
	}
}

func TestLoadSnapshotInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &TaskStore{tasks: make(map[string]*Task)}
	if err := store.Load(path); err == nil {
		t.Fatal("expected error loading malformed snapshot")
	}
}

func TestDisablePersistenceStopsSaves(t *testing.T) {
	dir := t.TempDir()
	store := &TaskStore{tasks: make(map[string]*Task)}
	if err := store.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	store.Create("a", "do a", "doing a", nil)
	store.DisablePersistence()
	store.Create("b", "do b", "doing b", nil)

	loaded := &TaskStore{tasks: make(map[string]*Task)}
	if err := loaded.Load(filepath.Join(dir, "tasks.json")); err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Get("task_2"); ok {
		t.Fatal("expected second task not persisted after disable")
	}
}
