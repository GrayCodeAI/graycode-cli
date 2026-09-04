package cmd

import (
	"testing"

	mission "github.com/GrayCodeAI/graycode-cli/internal/multiagent"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
)

func TestMissionFeaturesFromTasks(t *testing.T) {
	store := &tool.TaskStore{}
	store.Reset()
	root := store.Create("Root", "Root behavior", "", nil)
	child := store.Create("Child", "Child behavior", "", nil)
	store.Update(child.ID, func(task *tool.Task) {
		task.Dependencies = []tool.TaskDependency{{TargetID: root.ID, Type: "blocks"}}
	})

	features, waves, err := missionFeaturesFromTasks(store, "mission123")
	if err != nil {
		t.Fatalf("missionFeaturesFromTasks() error = %v", err)
	}
	if len(features) != 2 {
		t.Fatalf("features = %d, want 2", len(features))
	}
	if len(waves) != 2 || len(waves[0]) != 1 || waves[0][0] != root.ID || waves[1][0] != child.ID {
		t.Fatalf("waves = %#v", waves)
	}
	if features[0].Branch != "graycode-mission/mission123/"+root.ID {
		t.Fatalf("branch = %q", features[0].Branch)
	}
	if features[0].Status != mission.FeaturePending {
		t.Fatalf("status = %s, want pending", features[0].Status)
	}
}
