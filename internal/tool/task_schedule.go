package tool

import (
	"fmt"
	"sort"
	"strings"
)

const (
	maxScheduleTasks        = 1000
	maxScheduleDependencies = 10000
)

// TaskSchedule is a validated, deterministic view of blocking task edges.
type TaskSchedule struct {
	Waves [][]string
	Ready []*Task
}

// Schedule validates blocking dependencies and computes topological waves.
// Relationship-only edges remain visible but never delay execution.
func (s *TaskStore) Schedule() (TaskSchedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.tasks) > maxScheduleTasks {
		return TaskSchedule{}, fmt.Errorf("task schedule exceeds %d-task limit", maxScheduleTasks)
	}

	ids := make([]string, 0, len(s.tasks))
	for id := range s.tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	inDegree := make(map[string]int, len(ids))
	dependents := make(map[string][]string, len(ids))
	blockers := make(map[string][]string, len(ids))
	edgeCount := 0
	for _, id := range ids {
		task := s.tasks[id]
		if task == nil || strings.TrimSpace(id) == "" || task.ID != id {
			return TaskSchedule{}, fmt.Errorf("task schedule contains invalid task identity %q", id)
		}
		switch task.Status {
		case TaskStatusPending, TaskStatusInProgress, TaskStatusCompleted, TaskStatusFailed:
		default:
			return TaskSchedule{}, fmt.Errorf("task %q has invalid status %q", id, task.Status)
		}
		seen := make(map[string]struct{})
		for _, dependency := range task.Dependencies {
			if dependency.Type != "blocks" {
				continue
			}
			targetID := strings.TrimSpace(dependency.TargetID)
			if targetID == "" {
				return TaskSchedule{}, fmt.Errorf("task %q has an empty blocking dependency", id)
			}
			if targetID == id {
				return TaskSchedule{}, fmt.Errorf("task %q blocks itself", id)
			}
			if _, ok := s.tasks[targetID]; !ok {
				return TaskSchedule{}, fmt.Errorf("task %q depends on missing blocker %q", id, targetID)
			}
			if _, duplicate := seen[targetID]; duplicate {
				continue
			}
			seen[targetID] = struct{}{}
			edgeCount++
			if edgeCount > maxScheduleDependencies {
				return TaskSchedule{}, fmt.Errorf(
					"task schedule exceeds %d-dependency limit",
					maxScheduleDependencies,
				)
			}
			inDegree[id]++
			dependents[targetID] = append(dependents[targetID], id)
			blockers[id] = append(blockers[id], targetID)
		}
	}
	for _, id := range ids {
		sort.Strings(dependents[id])
		sort.Strings(blockers[id])
	}

	queue := make([]string, 0)
	for _, id := range ids {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	waves := make([][]string, 0)
	visited := 0
	for len(queue) > 0 {
		wave := append([]string(nil), queue...)
		waves = append(waves, wave)
		visited += len(wave)
		next := make([]string, 0)
		for _, id := range wave {
			for _, dependentID := range dependents[id] {
				inDegree[dependentID]--
				if inDegree[dependentID] == 0 {
					next = append(next, dependentID)
				}
			}
		}
		sort.Strings(next)
		queue = next
	}
	if visited != len(ids) {
		return TaskSchedule{}, fmt.Errorf("cycle detected in blocking task dependency graph")
	}

	ready := make([]*Task, 0)
	for _, id := range ids {
		task := s.tasks[id]
		if task.Status != TaskStatusPending {
			continue
		}
		isReady := true
		for _, blockerID := range blockers[id] {
			if s.tasks[blockerID].Status != TaskStatusCompleted {
				isReady = false
				break
			}
		}
		if isReady {
			ready = append(ready, cloneScheduledTask(task))
		}
	}
	return TaskSchedule{Waves: waves, Ready: ready}, nil
}

func cloneScheduledTask(task *Task) *Task {
	cloned := *task
	cloned.Dependencies = append([]TaskDependency(nil), task.Dependencies...)
	if task.Metadata != nil {
		cloned.Metadata = make(map[string]any, len(task.Metadata))
		for key, value := range task.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return &cloned
}
