package model

import "strings"

// Role identifies the purpose of a model within a multi-model workflow.
type Role string

const (
	RolePlanner  Role = "planner"
	RoleCoder    Role = "coder"
	RoleReviewer Role = "reviewer"
	RoleCommit   Role = "commit"
)

// ModelRoles maps each role to a specific model name.
// Empty fields fall back to the primary (coder) model.
type ModelRoles struct {
	Planner  string `json:"planner,omitempty"`
	Coder    string `json:"coder,omitempty"`
	Reviewer string `json:"reviewer,omitempty"`
	Commit   string `json:"commit,omitempty"`
}

// cheapCommitModels lists inexpensive models, in priority order, to use for
// the commit role when no explicit model is configured.
var cheapCommitModels = []string{
	"claude-3-5-haiku-20241022",
	"gpt-4o-mini",
	"gemini-2.5-flash",
}

// DefaultRoles returns a ModelRoles where every role uses primaryModel except
// Commit, which defaults to the cheapest available model.
func DefaultRoles(primaryModel string) ModelRoles {
	commit := cheapestAvailable(primaryModel)
	return ModelRoles{
		Planner:  primaryModel,
		Coder:    primaryModel,
		Reviewer: primaryModel,
		Commit:   commit,
	}
}

// ModelForRole returns the model name assigned to role, falling back to the
// Coder model (primary) if the role-specific field is empty.
func (r ModelRoles) ModelForRole(role Role) string {
	var m string
	switch role {
	case RolePlanner:
		m = r.Planner
	case RoleCoder:
		m = r.Coder
	case RoleReviewer:
		m = r.Reviewer
	case RoleCommit:
		m = r.Commit
	}
	if strings.TrimSpace(m) == "" {
		if strings.TrimSpace(r.Coder) != "" {
			return r.Coder
		}
		return "claude-sonnet-4-20250514" // ultimate fallback
	}
	return m
}

// cheapestAvailable picks the first cheap model present in the catalog,
// falling back to primaryModel if none are found.
func cheapestAvailable(primaryModel string) string {
	for _, name := range cheapCommitModels {
		if _, ok := Find(name); ok {
			return name
		}
	}
	return primaryModel
}
