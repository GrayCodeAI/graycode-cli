package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	mission "github.com/GrayCodeAI/hawk/internal/multiagent"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

var missionFromTasks bool

func init() {
	missionCmd.Flags().BoolVar(&missionFromTasks, "from-tasks", false, "Execute the validated task graph instead of LLM planning")
}

func missionPrompt(args []string) (string, error) {
	if missionFromTasks {
		if len(args) == 0 {
			return "Execute validated task graph", nil
		}
		return strings.Join(args, " "), nil
	}
	if len(args) != 1 {
		return "", fmt.Errorf("mission requires exactly one prompt unless --from-tasks is set")
	}
	return args[0], nil
}

func missionFeaturesFromTasks(store *tool.TaskStore, missionID string) ([]mission.Feature, [][]string, error) {
	schedule, err := store.GetSchedule()
	if err != nil {
		return nil, nil, err
	}
	if len(schedule.Waves) == 0 {
		return nil, nil, fmt.Errorf("task graph is empty")
	}

	features := make([]mission.Feature, 0, len(schedule.Waves))
	for _, wave := range schedule.Waves {
		for _, id := range wave {
			task, ok := store.Get(id)
			if !ok {
				return nil, nil, fmt.Errorf("scheduled task %q not found", id)
			}
			status := mission.FeaturePending
			if task.Status == tool.TaskStatusCompleted {
				status = mission.FeatureCompleted
			}
			expected := strings.TrimSpace(task.Description)
			if expected == "" {
				expected = task.Subject
			}
			features = append(features, mission.Feature{
				ID:               task.ID,
				Description:      task.Subject,
				ExpectedBehavior: expected,
				Branch:           fmt.Sprintf("hawk-mission/%s/%s", missionID, task.ID),
				Status:           status,
			})
		}
	}
	return features, schedule.Waves, nil
}

func graphTrackingWorker(store *tool.TaskStore, worker mission.WorkerFunc) mission.WorkerFunc {
	return func(ctx context.Context, feature *mission.Feature, missionDir string, cfg mission.Config) (*mission.Handoff, error) {
		store.Update(feature.ID, func(task *tool.Task) {
			task.Status = tool.TaskStatusInProgress
			if task.Owner == "" {
				task.Owner = "mission"
			}
			if task.Metadata == nil {
				task.Metadata = map[string]any{}
			}
			task.Metadata["graph_dispatch_started_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		})

		handoff, err := worker(ctx, feature, missionDir, cfg)
		store.Update(feature.ID, func(task *tool.Task) {
			if task.Metadata == nil {
				task.Metadata = map[string]any{}
			}
			task.Metadata["graph_dispatch_finished_at"] = time.Now().UTC().Format(time.RFC3339Nano)
			if err != nil {
				task.Status = tool.TaskStatusPending
				task.Metadata["graph_dispatch_error"] = err.Error()
				return
			}
			task.Status = tool.TaskStatusCompleted
			delete(task.Metadata, "graph_dispatch_error")
			if handoff != nil {
				task.Metadata["graph_dispatch_summary"] = handoff.Summary
				task.Metadata["graph_dispatch_tests_passed"] = handoff.TestsPassed
				if handoff.CommitID != "" {
					task.Metadata["graph_dispatch_commit"] = handoff.CommitID
				}
			}
		})
		return handoff, err
	}
}
