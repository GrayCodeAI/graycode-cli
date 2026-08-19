package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillInvocationPolicy(t *testing.T) {
	// Default zero-value policy -> model and user invocable
	def := SkillInvocationPolicy{}
	if !def.IsModelInvocable() {
		t.Error("default zero-value policy should be model invocable")
	}
	if !def.IsUserInvocable() {
		t.Error("default zero-value policy should be user invocable")
	}

	// Explicit false for model
	f := false
	tr := true
	p1 := SkillInvocationPolicy{ModelInvocable: &f, UserInvocable: &tr}
	if p1.IsModelInvocable() {
		t.Error("p1 should not be model invocable")
	}
	if !p1.IsUserInvocable() {
		t.Error("p1 should be user invocable")
	}

	// NewInvocationPolicy constructor
	p2 := NewInvocationPolicy(true, false)
	if !p2.IsModelInvocable() {
		t.Error("p2 should be model invocable")
	}
	if p2.IsUserInvocable() {
		t.Error("p2 should not be user invocable")
	}
}

func TestSkillRegistry_RegisterAndDispose(t *testing.T) {
	reg := NewSkillRegistry()
	runtime := NewRuntimeSkillProvider("test-runtime")
	runtime.AddSkill(SkillEntry{
		Name:        "custom-lint",
		Description: "Run custom linter",
		Content:     "Instructions for custom lint",
	})

	ctx := context.Background()
	disposer := reg.Register(runtime)

	// Verify skill is discoverable
	entry, err := reg.Get(ctx, "", "custom-lint")
	if err != nil {
		t.Fatalf("Get custom-lint failed: %v", err)
	}
	if entry.Name != "custom-lint" || entry.Provider != "test-runtime" {
		t.Fatalf("unexpected entry: %#v", entry)
	}

	// Call disposer
	disposer()

	// Verify skill is no longer in registry
	_, err = reg.Get(ctx, "", "custom-lint")
	if err == nil {
		t.Fatal("expected error after disposing provider, got nil")
	}
}

func TestSkillRegistry_ShadowingNearestWins(t *testing.T) {
	reg := &GlobalSkillRegistry{}

	globalProvider := NewRuntimeSkillProvider("global-layer")
	globalProvider.AddSkill(SkillEntry{
		Name:        "test-runner",
		Description: "Global test runner",
		Content:     "global instructions",
	})

	projectProvider := NewRuntimeSkillProvider("project-layer")
	projectProvider.AddSkill(SkillEntry{
		Name:        "test-runner",
		Description: "Project-specific test runner",
		Content:     "project instructions",
	})

	// Register global first, then project (project has higher priority / shadows global)
	_ = reg.Register(globalProvider)
	_ = reg.Register(projectProvider)

	ctx := context.Background()
	entry, err := reg.Get(ctx, "", "test-runner")
	if err != nil {
		t.Fatalf("Get test-runner failed: %v", err)
	}

	// Project layer should shadow global layer
	if entry.Description != "Project-specific test runner" {
		t.Fatalf("entry.Description = %q, want project-layer description", entry.Description)
	}
	if entry.Provider != "project-layer" {
		t.Fatalf("entry.Provider = %q, want project-layer", entry.Provider)
	}

	// List should only contain the winning shadowed entry once
	list, err := reg.List(ctx, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 deduplicated entry, got %d", len(list))
	}
	if list[0].Provider != "project-layer" {
		t.Fatalf("list[0].Provider = %q, want project-layer", list[0].Provider)
	}
}

func TestSkillDigest_StabilityAcrossUnchanged(t *testing.T) {
	skills1 := []SkillEntry{
		{Name: "beta", Description: "Beta description"},
		{Name: "alpha", Description: "Alpha description"},
	}
	skills2 := []SkillEntry{
		{Name: "alpha", Description: "Alpha description"},
		{Name: "beta", Description: "Beta description"},
	}

	d1 := ComputeSkillDigest(skills1)
	d2 := ComputeSkillDigest(skills2)

	if d1 == "" || d1 == "empty" {
		t.Fatalf("invalid digest: %s", d1)
	}
	if d1 != d2 {
		t.Fatalf("expected deterministic digest regardless of input slice order: %s vs %s", d1, d2)
	}
}

func TestSkillDigest_ReplacementOnChange(t *testing.T) {
	skills1 := []SkillEntry{
		{Name: "alpha", Description: "Alpha description"},
	}
	skills2 := []SkillEntry{
		{Name: "alpha", Description: "Updated description"},
	}

	d1 := ComputeSkillDigest(skills1)
	d2 := ComputeSkillDigest(skills2)

	if d1 == d2 {
		t.Fatal("digest should change when skill description changes")
	}
}

func TestSkillDigest_TombstoneOnEmpty(t *testing.T) {
	d := ComputeSkillDigest(nil)
	if d != "empty" {
		t.Fatalf("digest for nil skills = %q, want 'empty'", d)
	}

	msg := RenderSkillCatalogMessage(nil, d)
	if !strings.Contains(msg, "retired") {
		t.Fatalf("tombstone message %q should mention retirement", msg)
	}
}

func TestFilesystemSkillProvider_CwdScopedDiscovery(t *testing.T) {
	tmpDir := t.TempDir()
	agentSkillsDir := filepath.Join(tmpDir, ".agents", "skills", "code-review")
	if err := os.MkdirAll(agentSkillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillContent := `---
name: code-review
description: Comprehensive Go code reviewer
model_invocable: true
user_invocable: false
---

Review Go code thoroughly for bugs and performance.
`
	if err := os.WriteFile(filepath.Join(agentSkillsDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	provider := NewFilesystemSkillProvider()
	ctx := context.Background()
	skills, err := provider.List(ctx, tmpDir)
	if err != nil {
		t.Fatalf("provider.List failed: %v", err)
	}

	var found *SkillEntry
	for _, s := range skills {
		if s.Name == "code-review" {
			found = &s
			break
		}
	}

	if found == nil {
		t.Fatal("code-review skill not discovered in .agents/skills")
	}
	if found.Description != "Comprehensive Go code reviewer" {
		t.Fatalf("found.Description = %q", found.Description)
	}
	if !found.Invocation.IsModelInvocable() {
		t.Error("expected ModelInvocable=true")
	}
	if found.Invocation.IsUserInvocable() {
		t.Error("expected UserInvocable=false")
	}
}
