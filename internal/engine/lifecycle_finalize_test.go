package engine

import (
	"context"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// finalizeMockSkillStore records Distill calls so Finalize's skill
// distillation path can be verified without real storage.
type finalizeMockSkillStore struct {
	distilled []struct {
		goal    string
		steps   []string
		outcome string
	}
}

func (m *finalizeMockSkillStore) Distill(goal string, steps []string, outcome string) error {
	m.distilled = append(m.distilled, struct {
		goal    string
		steps   []string
		outcome string
	}{goal, steps, outcome})
	return nil
}

func (m *finalizeMockSkillStore) Retrieve(query string) []string { return nil }

// TestFinalizePopulatesToolsAndFiles_TriggersSkillDistill verifies the H1
// fix: Finalize must populate outcome.ToolsUsed/FilesChanged from the
// session messages so that isComplex() can fire and skill distillation
// actually runs in production. Previously these fields were never set, so
// distillation was dead code.
func TestFinalizePopulatesToolsAndFiles_TriggersSkillDistill(t *testing.T) {
	sk := &finalizeMockSkillStore{}
	svc := &LifecycleService{lifecycle: &SessionLifecycle{SkillStore: sk}}

	messages := []types.EyrieMessage{
		{Role: "user", Content: "Refactor the auth module"},
		{Role: "assistant", ToolUse: []types.ToolCall{
			{Name: "Read", Arguments: map[string]interface{}{"path": "auth.go"}},
			{Name: "Write", Arguments: map[string]interface{}{"path": "auth.go"}},
			{Name: "Bash", Arguments: map[string]interface{}{"command": "go test ./..."}},
			{Name: "Edit", Arguments: map[string]interface{}{"file_path": "middleware.go"}},
		}},
	}

	svc.Finalize(context.Background(), messages, true, time.Second, 0)

	if len(sk.distilled) == 0 {
		t.Fatal("expected skill distillation to fire: Finalize must populate ToolsUsed/FilesChanged so isComplex() is satisfied")
	}
	if sk.distilled[0].goal != "Refactor the auth module" {
		t.Errorf("goal = %q, want %q", sk.distilled[0].goal, "Refactor the auth module")
	}
	has := func(name string) bool {
		for _, s := range sk.distilled[0].steps {
			if s == name {
				return true
			}
		}
		return false
	}
	if !has("Read") || !has("Bash") {
		t.Errorf("expected tools in steps, got %v", sk.distilled[0].steps)
	}
}

// TestFinalizeSimpleTaskNoSkillDistill verifies a trivial session with no
// tools/files does not trigger distillation (regression guard).
func TestFinalizeSimpleTaskNoSkillDistill(t *testing.T) {
	sk := &finalizeMockSkillStore{}
	svc := &LifecycleService{lifecycle: &SessionLifecycle{SkillStore: sk}}

	messages := []types.EyrieMessage{
		{Role: "user", Content: "What is 2+2?"},
		{Role: "assistant", Content: "4"},
	}

	svc.Finalize(context.Background(), messages, true, time.Second, 0)

	if len(sk.distilled) != 0 {
		t.Errorf("expected no skill distillation for simple task, got %d", len(sk.distilled))
	}
}
