package model

import "testing"

func TestDefaultRoles(t *testing.T) {
	roles := DefaultRoles("claude-sonnet-4-20250514")

	if roles.Planner != "claude-sonnet-4-20250514" {
		t.Errorf("Planner = %q, want primary model", roles.Planner)
	}
	if roles.Coder != "claude-sonnet-4-20250514" {
		t.Errorf("Coder = %q, want primary model", roles.Coder)
	}
	if roles.Reviewer != "claude-sonnet-4-20250514" {
		t.Errorf("Reviewer = %q, want primary model", roles.Reviewer)
	}
	// Commit should be a cheap model (not the primary expensive one).
	if roles.Commit == "" {
		t.Error("Commit should not be empty")
	}
	if roles.Commit == roles.Coder {
		// Only acceptable if no cheap model was found in catalog
		t.Log("Commit fell back to primary model (no cheap model in catalog)")
	}
}

func TestDefaultRolesWithUnknownPrimary(t *testing.T) {
	// Even with an unknown primary, Planner/Coder/Reviewer should use it.
	roles := DefaultRoles("my-custom-model")

	if roles.Planner != "my-custom-model" {
		t.Errorf("Planner = %q, want my-custom-model", roles.Planner)
	}
	if roles.Coder != "my-custom-model" {
		t.Errorf("Coder = %q, want my-custom-model", roles.Coder)
	}
	// Commit picks a cheap model from catalog, or falls back to primary.
	if roles.Commit == "" {
		t.Error("Commit should not be empty")
	}
}

func TestModelForRole(t *testing.T) {
	roles := ModelRoles{
		Planner:  "planner-model",
		Coder:    "coder-model",
		Reviewer: "reviewer-model",
		Commit:   "commit-model",
	}

	tests := []struct {
		role     Role
		expected string
	}{
		{RolePlanner, "planner-model"},
		{RoleCoder, "coder-model"},
		{RoleReviewer, "reviewer-model"},
		{RoleCommit, "commit-model"},
	}
	for _, tt := range tests {
		got := roles.ModelForRole(tt.role)
		if got != tt.expected {
			t.Errorf("ModelForRole(%q) = %q, want %q", tt.role, got, tt.expected)
		}
	}
}

func TestModelForRoleFallback(t *testing.T) {
	// When a role field is empty, it should fall back to the Coder model.
	roles := ModelRoles{
		Coder: "primary-model",
	}

	if got := roles.ModelForRole(RolePlanner); got != "primary-model" {
		t.Errorf("empty Planner should fall back to Coder, got %q", got)
	}
	if got := roles.ModelForRole(RoleReviewer); got != "primary-model" {
		t.Errorf("empty Reviewer should fall back to Coder, got %q", got)
	}
	if got := roles.ModelForRole(RoleCommit); got != "primary-model" {
		t.Errorf("empty Commit should fall back to Coder, got %q", got)
	}
}

func TestModelForRoleUltimateFallback(t *testing.T) {
	// When even Coder is empty, fall back to the hardcoded default.
	roles := ModelRoles{}

	got := roles.ModelForRole(RolePlanner)
	if got != "claude-sonnet-4-20250514" {
		t.Errorf("fully empty roles should fall back to hardcoded default, got %q", got)
	}
}

func TestModelForRoleUnknownRole(t *testing.T) {
	roles := ModelRoles{
		Coder: "primary",
	}
	// An undefined role should fall back to Coder.
	got := roles.ModelForRole(Role("unknown"))
	if got != "primary" {
		t.Errorf("unknown role should fall back to Coder, got %q", got)
	}
}
