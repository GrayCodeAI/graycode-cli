package routing

import (
	"strings"

	eycatalog "github.com/GrayCodeAI/eyrie/catalog"
)

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

// DefaultRoles returns a ModelRoles where every role uses primaryModel except
// Commit, which defaults to the cheapest available model from the catalog.
func DefaultRoles(primaryModel string) ModelRoles {
	return fromEyrieRoles(eycatalog.DefaultModelRolesV1(eyrieCatalogV1(), primaryModel))
}

// ModelForRole returns the model name assigned to role, falling back to the
// Coder model (primary) if the role-specific field is empty.
func (r ModelRoles) ModelForRole(role Role) string {
	return eycatalog.ModelForRoleV1(eyrieCatalogV1(), toEyrieRoles(r), eycatalog.ModelRole(role))
}

// CheapestForProvider queries eyrie's catalog at runtime and returns the
// cheapest model for the given provider. No hardcoded model names.
func CheapestForProvider(provider, fallback string) string {
	return eycatalog.CheapestModelForProviderV1(eyrieCatalogV1(), provider, fallback)
}

func toEyrieRoles(r ModelRoles) eycatalog.ModelRoleAssignments {
	return eycatalog.ModelRoleAssignments{
		Planner:  strings.TrimSpace(r.Planner),
		Coder:    strings.TrimSpace(r.Coder),
		Reviewer: strings.TrimSpace(r.Reviewer),
		Commit:   strings.TrimSpace(r.Commit),
	}
}

func fromEyrieRoles(r eycatalog.ModelRoleAssignments) ModelRoles {
	return ModelRoles{
		Planner:  strings.TrimSpace(r.Planner),
		Coder:    strings.TrimSpace(r.Coder),
		Reviewer: strings.TrimSpace(r.Reviewer),
		Commit:   strings.TrimSpace(r.Commit),
	}
}
