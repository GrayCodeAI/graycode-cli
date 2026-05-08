package routing

import "testing"

func TestClassifyTask_Planning(t *testing.T) {
	cases := []string{
		"How should we approach this refactoring?",
		"Plan the implementation of the auth system",
		"Design a caching strategy for this service",
		"Let's architect the new microservice",
		"Break down this feature into tasks",
	}
	for _, msg := range cases {
		if got := ClassifyTask(msg); got != TaskPlanning {
			t.Errorf("ClassifyTask(%q) = %s, want planning", msg, got)
		}
	}
}

func TestClassifyTask_Coding(t *testing.T) {
	cases := []string{
		"Implement the login endpoint",
		"Fix the null pointer exception in handler.go",
		"Refactor the database layer to use connection pooling",
		"Write a function that parses CSV files",
		"Add error handling to the API client",
	}
	for _, msg := range cases {
		if got := ClassifyTask(msg); got != TaskCoding {
			t.Errorf("ClassifyTask(%q) = %s, want coding", msg, got)
		}
	}
}

func TestClassifyTask_Summary(t *testing.T) {
	cases := []string{
		"Summarize the changes we made today",
		"Give me a tldr of this file",
		"Recap what happened in the last session",
		"Write a commit message for these changes",
	}
	for _, msg := range cases {
		if got := ClassifyTask(msg); got != TaskSummary {
			t.Errorf("ClassifyTask(%q) = %s, want summary", msg, got)
		}
	}
}

func TestClassifyTask_Review(t *testing.T) {
	cases := []string{
		"Review this pull request",
		"Check this code for security issues",
		"Code review the auth module",
		"Audit the database queries",
	}
	for _, msg := range cases {
		if got := ClassifyTask(msg); got != TaskReview {
			t.Errorf("ClassifyTask(%q) = %s, want review", msg, got)
		}
	}
}

func TestClassifyTask_General(t *testing.T) {
	cases := []string{
		"What is the capital of France?",
		"Tell me about Go generics",
		"How does HTTP/2 work?",
	}
	for _, msg := range cases {
		if got := ClassifyTask(msg); got != TaskGeneral {
			t.Errorf("ClassifyTask(%q) = %s, want general", msg, got)
		}
	}
}

func TestCascadeRouter_Route(t *testing.T) {
	roles := ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-4-20250514",
	}
	cr := NewCascadeRouter(roles)

	tests := []struct {
		msg  string
		want string
	}{
		{"Plan the migration", "claude-opus-4-20250514"},
		{"Implement the endpoint", "claude-sonnet-4-20250514"},
		{"Summarize the session", "claude-haiku-4-20250514"},
		{"Review this diff", "claude-sonnet-4-20250514"},
		{"What is Go?", "claude-sonnet-4-20250514"}, // general falls back to coder
	}

	for _, tt := range tests {
		got := cr.Route(tt.msg, "")
		if got != tt.want {
			t.Errorf("Route(%q) = %s, want %s", tt.msg, got, tt.want)
		}
	}
}

func TestCascadeRouter_HintOverride(t *testing.T) {
	roles := ModelRoles{
		Planner:  "opus",
		Coder:    "sonnet",
		Reviewer: "sonnet",
		Commit:   "haiku",
	}
	cr := NewCascadeRouter(roles)

	// Even though "implement" would classify as coding, hint overrides
	got := cr.Route("implement something", TaskPlanning)
	if got != "opus" {
		t.Errorf("Route with hint=planning got %s, want opus", got)
	}

	got = cr.Route("plan something", TaskSummary)
	if got != "haiku" {
		t.Errorf("Route with hint=summary got %s, want haiku", got)
	}
}

func TestCascadeRouter_EmptyRolesFallback(t *testing.T) {
	roles := ModelRoles{
		Coder: "sonnet",
	}
	cr := NewCascadeRouter(roles)

	// All roles should fall back to coder when not explicitly set
	if got := cr.Route("Plan something", ""); got != "sonnet" {
		t.Errorf("planning with empty planner role got %s, want sonnet (fallback)", got)
	}
}
