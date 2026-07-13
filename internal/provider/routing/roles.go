package routing

import (
	"context"
	"strings"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
)

// Role identifies the purpose of a model within a Hawk multi-model workflow.
type Role string

const (
	RolePlanner  Role = "planner"
	RoleCoder    Role = "coder"
	RoleReviewer Role = "reviewer"
	RoleCommit   Role = "commit"
)

type ModelRoles struct {
	Planner  string `json:"planner,omitempty"`
	Coder    string `json:"coder,omitempty"`
	Reviewer string `json:"reviewer,omitempty"`
	Commit   string `json:"commit,omitempty"`
}

// DefaultRoles keeps Hawk's workflow policy while asking Eyrie for the
// economical same-provider model used for commit/summarization work.
func DefaultRoles(primaryModel string) ModelRoles {
	primaryModel = strings.TrimSpace(primaryModel)
	commit := primaryModel
	if engine := eyrieModelEngine(); engine != nil {
		if provider := engine.ProviderForModel(context.Background(), primaryModel); provider != "" {
			commit = engine.PreferredModel(context.Background(), provider, eyrieengine.ModelClassEconomical, primaryModel)
		}
	}
	return ModelRoles{Planner: primaryModel, Coder: primaryModel, Reviewer: primaryModel, Commit: commit}
}

func (r ModelRoles) ModelForRole(role Role) string {
	var model string
	switch role {
	case RolePlanner:
		model = r.Planner
	case RoleCoder:
		model = r.Coder
	case RoleReviewer:
		model = r.Reviewer
	case RoleCommit:
		model = r.Commit
	}
	if model = strings.TrimSpace(model); model != "" {
		return model
	}
	if coder := strings.TrimSpace(r.Coder); coder != "" {
		return coder
	}
	if engine := eyrieModelEngine(); engine != nil {
		return engine.PrimaryModel(context.Background())
	}
	return ""
}

func CheapestForProvider(provider, fallback string) string {
	if engine := eyrieModelEngine(); engine != nil {
		return engine.PreferredModel(context.Background(), provider, eyrieengine.ModelClassEconomical, fallback)
	}
	return fallback
}
