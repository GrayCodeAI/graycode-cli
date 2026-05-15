package tool

import "testing"

func TestTaskStore_Create(t *testing.T) {
	t.Parallel()
	store := &TaskStore{tasks: make(map[string]*Task)}
	task := store.Create("Fix bug", "Fix the login bug", "Fix the login page crash on empty email", nil)

	if task == nil {
		t.Fatal("Create returned nil")
	}
	if task.ID == "" {
		t.Error("task ID should not be empty")
	}
	if task.Subject != "Fix bug" {
		t.Errorf("Subject = %q, want 'Fix bug'", task.Subject)
	}
	if task.Status != TaskStatusPending {
		t.Errorf("Status = %q, want pending", task.Status)
	}
}

func TestTaskStore_Get(t *testing.T) {
	t.Parallel()
	store := &TaskStore{tasks: make(map[string]*Task)}
	task := store.Create("Test task", "desc", "form", nil)

	got, ok := store.Get(task.ID)
	if !ok {
		t.Fatal("Get should find created task")
	}
	if got.Subject != "Test task" {
		t.Errorf("Subject = %q, want 'Test task'", got.Subject)
	}

	_, ok = store.Get("nonexistent")
	if ok {
		t.Error("Get should return false for missing task")
	}
}

func TestTaskStore_List(t *testing.T) {
	t.Parallel()
	store := &TaskStore{tasks: make(map[string]*Task)}
	store.Create("Task 1", "d1", "f1", nil)
	store.Create("Task 2", "d2", "f2", nil)
	store.Create("Task 3", "d3", "f3", nil)

	tasks := store.List()
	if len(tasks) != 3 {
		t.Errorf("List() = %d tasks, want 3", len(tasks))
	}
}

func TestTaskStore_Update(t *testing.T) {
	t.Parallel()
	store := &TaskStore{tasks: make(map[string]*Task)}
	task := store.Create("Original", "desc", "form", nil)

	ok := store.Update(task.ID, func(t *Task) {
		t.Status = TaskStatusCompleted
		t.Subject = "Updated"
	})
	if !ok {
		t.Fatal("Update should return true for existing task")
	}

	got, _ := store.Get(task.ID)
	if got.Status != TaskStatusCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if got.Subject != "Updated" {
		t.Errorf("Subject = %q, want 'Updated'", got.Subject)
	}

	ok = store.Update("nonexistent", func(t *Task) {})
	if ok {
		t.Error("Update should return false for missing task")
	}
}

func TestTaskStore_CreateWithParent(t *testing.T) {
	t.Parallel()
	store := &TaskStore{tasks: make(map[string]*Task)}
	parent := store.Create("Parent", "p-desc", "p-form", nil)
	child := store.CreateWithParent("Child", "c-desc", "c-form", nil, parent.ID)

	if child.ParentID != parent.ID {
		t.Errorf("ParentID = %q, want %q", child.ParentID, parent.ID)
	}
}
