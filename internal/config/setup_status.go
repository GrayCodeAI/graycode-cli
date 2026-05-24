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

// EvaluateSetup loads the OS credential store and reports whether /config is still required.
func EvaluateSetup(ctx context.Context) SetupState {
	if ctx == nil {
		ctx = context.Background()
	}
	PrepareCredentialDiscovery(ctx)
	return evaluateSetupFrom(hasConfiguredDeployment(ctx), HasSelectedModel())
}

// EvaluateSetupCached uses the in-memory credential snapshot (fast; for TUI hot paths).
func EvaluateSetupCached(ctx context.Context) SetupState {
	if ctx == nil {
		ctx = context.Background()
	}
	return evaluateSetupFrom(HasConfiguredDeploymentCached(ctx), HasSelectedModel())
}

func evaluateSetupFrom(hasCreds, hasModel bool) SetupState {
	st := SetupState{
		HasCredentials: hasCreds,
		HasModel:       hasModel,
		NeedsSetup:     !hasCreds || !hasModel,
	}
	switch {
	case !hasCreds:
		st.Hint = "First-time setup: run /config to paste an API key or use Ollama local"
	case !hasModel:
		st.Hint = "Almost ready: pick a model to start chatting"
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
	RefreshConfigCredSnapshot(ctx)
	if hasConfiguredDeploymentCached(ctx) {
		return true
	}
	return eyriecfg.HasAnyConfiguredDeployment(ctx)
}

// HasSelectedModel reports whether eyrie provider.json has a selected model.
func HasSelectedModel() bool {
	return strings.TrimSpace(ActiveModel(context.Background())) != ""
}

// NeedsFirstRunSetup is true when the user should complete /config (API key and/or model).
func NeedsFirstRunSetup(ctx context.Context) bool {
	return EvaluateSetupCached(ctx).NeedsSetup
}

// FirstRunSetupHint returns a short banner line for the welcome screen.
func FirstRunSetupHint(ctx context.Context) string {
	return EvaluateSetupCached(ctx).Hint
}
