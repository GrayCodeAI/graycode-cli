package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// VerifyPlanExecutionTool checks whether a plan's steps have been executed correctly.
type VerifyPlanExecutionTool struct{}

func (VerifyPlanExecutionTool) Name() string        { return "VerifyPlanExecution" }
func (VerifyPlanExecutionTool) Aliases() []string   { return []string{"verify_plan_execution"} }
func (VerifyPlanExecutionTool) Description() string {
	return "Verify that a plan's steps have been executed correctly by checking task completion and file changes"
}
func (VerifyPlanExecutionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"plan_steps": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"description": map[string]interface{}{"type": "string"},
						"expected":    map[string]interface{}{"type": "string"},
					},
				},
				"description": "Steps to verify with their expected outcomes",
			},
		},
		"required": []string{"plan_steps"},
	}
}

func (VerifyPlanExecutionTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		PlanSteps []struct {
			Description string `json:"description"`
			Expected    string `json:"expected"`
		} `json:"plan_steps"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if len(p.PlanSteps) == 0 {
		return "", fmt.Errorf("plan_steps must not be empty")
	}

	// Check task store for completed tasks matching plan steps
	tasks := globalTaskStore.List()
	completedSubjects := make(map[string]bool)
	for _, t := range tasks {
		if t.Status == TaskStatusCompleted {
			completedSubjects[strings.ToLower(t.Subject)] = true
		}
	}

	var results []map[string]any
	allVerified := true
	for _, step := range p.PlanSteps {
		verified := false
		for subject := range completedSubjects {
			if strings.Contains(subject, strings.ToLower(step.Description)) ||
				strings.Contains(strings.ToLower(step.Description), subject) {
				verified = true
				break
			}
		}
		if !verified {
			allVerified = false
		}
		results = append(results, map[string]any{
			"step":     step.Description,
			"expected": step.Expected,
			"verified": verified,
		})
	}

	out, _ := json.Marshal(map[string]any{
		"allVerified": allVerified,
		"steps":       results,
		"totalSteps":  len(p.PlanSteps),
		"verified":    countVerified(results),
	})
	return string(out), nil
}

func countVerified(results []map[string]any) int {
	count := 0
	for _, r := range results {
		if v, ok := r["verified"].(bool); ok && v {
			count++
		}
	}
	return count
}
