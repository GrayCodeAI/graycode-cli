package tool

import (
	"reflect"
	"strings"
	"testing"
)

func TestTaskScheduleDeterministicWavesAndReadiness(t *testing.T) {
	store := &TaskStore{tasks: make(map[string]*Task)}
	first := store.Create("first", "first", "", nil)
	second := store.Create("second", "second", "", nil)
	third := store.Create("third", "third", "", nil)
	store.Update(third.ID, func(task *Task) {
		task.Dependencies = []TaskDependency{
			{TargetID: second.ID, Type: "blocks"},
			{TargetID: first.ID, Type: "blocks"},
		}
	})

	schedule, err := store.Schedule()
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	wantWaves := [][]string{{first.ID, second.ID}, {third.ID}}
	if !reflect.DeepEqual(schedule.Waves, wantWaves) {
		t.Fatalf("waves = %#v, want %#v", schedule.Waves, wantWaves)
	}
	gotReady := []string{schedule.Ready[0].ID, schedule.Ready[1].ID}
	if !reflect.DeepEqual(gotReady, wantWaves[0]) {
		t.Fatalf("ready = %#v, want %#v", gotReady, wantWaves[0])
	}

	store.Update(first.ID, func(task *Task) { task.Status = TaskStatusCompleted })
	store.Update(second.ID, func(task *Task) { task.Status = TaskStatusCompleted })
	schedule, err = store.Schedule()
	if err != nil {
		t.Fatalf("Schedule() after completion error = %v", err)
	}
	if len(schedule.Ready) != 1 || schedule.Ready[0].ID != third.ID {
		t.Fatalf("ready after completion = %#v", schedule.Ready)
	}
}

func TestTaskScheduleRejectsInvalidBlockingGraphs(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*TaskStore)
		want  string
	}{
		{
			name: "missing blocker",
			setup: func(store *TaskStore) {
				task := store.Create("task", "task", "", nil)
				store.Update(task.ID, func(current *Task) {
					current.Dependencies = []TaskDependency{{TargetID: "missing", Type: "blocks"}}
				})
			},
			want: "missing blocker",
		},
		{
			name: "self blocker",
			setup: func(store *TaskStore) {
				task := store.Create("task", "task", "", nil)
				store.Update(task.ID, func(current *Task) {
					current.Dependencies = []TaskDependency{{TargetID: task.ID, Type: "blocks"}}
				})
			},
			want: "blocks itself",
		},
		{
			name: "cycle",
			setup: func(store *TaskStore) {
				first := store.Create("first", "first", "", nil)
				second := store.Create("second", "second", "", nil)
				store.Update(first.ID, func(current *Task) {
					current.Dependencies = []TaskDependency{{TargetID: second.ID, Type: "blocks"}}
				})
				store.Update(second.ID, func(current *Task) {
					current.Dependencies = []TaskDependency{{TargetID: first.ID, Type: "blocks"}}
				})
			},
			want: "cycle detected",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &TaskStore{tasks: make(map[string]*Task)}
			test.setup(store)
			_, err := store.Schedule()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Schedule() error = %v, want %q", err, test.want)
			}
			if ready := store.GetReadyWork(); len(ready) != 0 {
				t.Fatalf("GetReadyWork() = %#v, want fail-closed empty set", ready)
			}
		})
	}
}

func TestTaskScheduleIgnoresRelationshipOnlyEdges(t *testing.T) {
	store := &TaskStore{tasks: make(map[string]*Task)}
	first := store.Create("first", "first", "", nil)
	second := store.Create("second", "second", "", nil)
	store.Update(second.ID, func(task *Task) {
		task.Dependencies = []TaskDependency{
			{TargetID: first.ID, Type: "related"},
			{TargetID: "external-parent", Type: "parent-child"},
		}
	})
	schedule, err := store.Schedule()
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if len(schedule.Ready) != 2 {
		t.Fatalf("ready count = %d, want 2", len(schedule.Ready))
	}
}
