package config

import (
	"context"
	"strings"
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
	return evaluateSetupFrom(HasConfiguredDeployment(ctx), HasSelectedModel())
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
		// Splash uses footer guidance only until credentials exist.
		st.Hint = ""
	case !hasModel:
		st.Hint = "Almost ready: /config → finish setup"
	}
	return st
}

// HasConfiguredDeployment reports whether at least one graycode-router deployment has credentials.
func HasConfiguredDeployment(ctx context.Context) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	engine, err := newGraycodeRouterEngine()
	return err == nil && engine.EffectiveSelection(ctx, SelectionOptions{}).HasConfiguredDeployment
}

// HasSelectedModel reports whether graycode-router provider.json has a selected model.
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
