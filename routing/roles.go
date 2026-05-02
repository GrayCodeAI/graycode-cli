package routing

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

// DefaultRoles returns a ModelRoles where every role uses primaryModel except
// Commit, which defaults to the cheapest available model from the catalog.
func DefaultRoles(primaryModel string) ModelRoles {
	commit := CheapestForProvider(providerOf(primaryModel), primaryModel)
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
		return primaryModel()
	}
	return m
}

// CheapestForProvider queries eyrie's catalog at runtime and returns the
// cheapest model for the given provider. No hardcoded model names.
func CheapestForProvider(provider, fallback string) string {
	models := ByProvider(provider)
	if len(models) == 0 {
		return fallback
	}
	cheapest := models[0]
	for _, m := range models[1:] {
		if m.InputPrice > 0 && m.InputPrice < cheapest.InputPrice {
			cheapest = m
		}
	}
	if cheapest.Name != "" {
		return cheapest.Name
	}
	return fallback
}

// providerOf extracts the provider from a model name by looking it up in the catalog.
func providerOf(modelName string) string {
	info, ok := Find(modelName)
	if ok {
		return info.Provider
	}
	return ""
}

// primaryModel returns a reasonable fallback by querying what's available.
func primaryModel() string {
	providers := AllProviders()
	for _, p := range providers {
		if m := DefaultModel(p); m != "" {
			return m
		}
	}
	return ""
}
