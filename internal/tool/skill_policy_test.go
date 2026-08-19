package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/plugin"
)

func TestSkillTool_PolicyAndRendering(t *testing.T) {
	runtime := plugin.NewRuntimeSkillProvider("test-policy")
	runtime.AddSkill(plugin.SkillEntry{
		Name:         "allowed-skill",
		Description:  "Skill available to model",
		Content:      "Follow these steps...",
		ResourceBase: "/path/to/resources",
		Invocation:   plugin.NewInvocationPolicy(true, true),
	})
	runtime.AddSkill(plugin.SkillEntry{
		Name:        "human-only-skill",
		Description: "Skill for user only",
		Content:     "Secret manual instructions...",
		Invocation:  plugin.NewInvocationPolicy(false, true),
	})

	disposer := plugin.DefaultRegistry.Register(runtime)
	defer disposer()

	ctx := context.Background()
	skillTool := SkillTool{}

	// 1. List skills should only include model-invocable skill
	listOut, err := skillTool.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("list skills failed: %v", err)
	}
	if !strings.Contains(listOut, "allowed-skill") {
		t.Fatalf("list output %q should contain allowed-skill", listOut)
	}
	if strings.Contains(listOut, "human-only-skill") {
		t.Fatalf("list output %q should NOT contain human-only-skill", listOut)
	}

	// 2. Fetch allowed-skill should return canonical rendering
	inputAllowed, _ := json.Marshal(map[string]string{"skill": "allowed-skill"})
	renderOut, err := skillTool.Execute(ctx, inputAllowed)
	if err != nil {
		t.Fatalf("execute allowed-skill failed: %v", err)
	}
	if !strings.Contains(renderOut, "# Skill: allowed-skill") {
		t.Fatalf("render output missing header: %s", renderOut)
	}
	if !strings.Contains(renderOut, "Provider: test-policy") {
		t.Fatalf("render output missing provider: %s", renderOut)
	}
	if !strings.Contains(renderOut, "Resource Base: /path/to/resources") {
		t.Fatalf("render output missing resource base: %s", renderOut)
	}
	if !strings.Contains(renderOut, "Follow these steps...") {
		t.Fatalf("render output missing content: %s", renderOut)
	}

	// 3. Fetch human-only skill should return distinct policy restricted error
	inputRestricted, _ := json.Marshal(map[string]string{"skill": "human-only-skill"})
	_, err = skillTool.Execute(ctx, inputRestricted)
	if err == nil {
		t.Fatal("expected error requesting human-only skill, got nil")
	}
	if !strings.Contains(err.Error(), "policy restricted") {
		t.Fatalf("error %q should mention 'policy restricted'", err.Error())
	}
}
