package routing

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/provider/gateway"
)

// Role identifies the purpose of a model within a Graycode multi-model workflow.
type Role string

const (
	RolePlanner  Role = "planner"
	RoleExplorer Role = "explorer"
	RoleCoder    Role = "coder"
	RoleReviewer Role = "reviewer"
	RoleCommit   Role = "commit"
)

type ModelRoles struct {
	Planner  string `json:"planner,omitempty"`
	Explorer string `json:"explorer,omitempty"`
	Coder    string `json:"coder,omitempty"`
	Reviewer string `json:"reviewer,omitempty"`
	Commit   string `json:"commit,omitempty"`
}

// DefaultRoles keeps Graycode's workflow policy while asking GraycodeRouter for the
// economical same-provider model used for commit/summarization work.
func DefaultRoles(primaryModel string) ModelRoles {
	primaryModel = strings.TrimSpace(primaryModel)
	ctx := context.Background()
	commit := primaryModel
	if provider := gateway.ProviderForModel(ctx, primaryModel); provider != "" {
		commit = gateway.PreferredModel(ctx, provider, gateway.ModelClassEconomical, primaryModel)
	}
	return ModelRoles{Planner: primaryModel, Explorer: commit, Coder: primaryModel, Reviewer: primaryModel, Commit: commit}
}

func (r ModelRoles) ModelForRole(role Role) string {
	var model string
	switch role {
	case RolePlanner:
		model = r.Planner
	case RoleExplorer:
		model = r.Explorer
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
	return gateway.PrimaryModel(context.Background())
}

func CheapestForProvider(provider, fallback string) string {
	return gateway.PreferredModel(context.Background(), provider, gateway.ModelClassEconomical, fallback)
}
