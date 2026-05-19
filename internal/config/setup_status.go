package config

import (
	"context"
	"strings"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
)

// SetupState is a single evaluation of first-run /config requirements.
type SetupState struct {
	HasCredentials bool
	HasModel       bool
	NeedsSetup     bool
	Hint           string
}

// EvaluateSetup loads keychain + env once and reports whether /config is still required.
func EvaluateSetup(ctx context.Context) SetupState {
	if ctx == nil {
		ctx = context.Background()
	}
	PrepareCredentialDiscovery(ctx)
	hasCreds := hasConfiguredDeployment(ctx)
	hasModel := HasSelectedModel()
	st := SetupState{
		HasCredentials: hasCreds,
		HasModel:       hasModel,
		NeedsSetup:     !hasCreds || !hasModel,
	}
	switch {
	case !hasCreds:
		st.Hint = "Setup: open /config → API keys → paste your key (stored in keychain)"
	case !hasModel:
		st.Hint = "Setup: open /config → pick a model after your API key"
	}
	return st
}

// HasConfiguredDeployment reports whether at least one eyrie deployment has credentials.
func HasConfiguredDeployment(ctx context.Context) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	PrepareCredentialDiscovery(ctx)
	return hasConfiguredDeployment(ctx)
}

func hasConfiguredDeployment(ctx context.Context) bool {
	rows, err := ListDeploymentRows(ctx)
	if err == nil {
		for _, row := range rows {
			if row.Configured {
				return true
			}
		}
	}
	return eyriecfg.HasAnyConfiguredDeployment(ctx)
}

// HasSelectedModel reports whether global settings include a non-empty model id.
func HasSelectedModel() bool {
	return strings.TrimSpace(LoadSettings().Model) != ""
}

// NeedsFirstRunSetup is true when the user should complete /config (API key and/or model).
func NeedsFirstRunSetup(ctx context.Context) bool {
	return EvaluateSetup(ctx).NeedsSetup
}

// FirstRunSetupHint returns a short banner line for the welcome screen.
func FirstRunSetupHint(ctx context.Context) string {
	return EvaluateSetup(ctx).Hint
}
