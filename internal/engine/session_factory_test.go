package engine

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/eyrie/runtime"
)

func TestNewHawkSession_UsesResolvedSelectionModel(t *testing.T) {
	selection := runtime.SelectionState{
		Provider:          "openrouter",
		Model:             "openrouter/auto",
		DeploymentRouting: false,
	}

	sess := NewHawkSession(context.Background(), selection, "openrouter", "", "system", nil)
	if got := sess.Provider(); got != "openrouter" {
		t.Fatalf("provider = %q, want openrouter", got)
	}
	if got := sess.Model(); got != "openrouter/auto" {
		t.Fatalf("model = %q, want openrouter/auto", got)
	}
}

func TestNewHawkSession_FallsBackToCallerModelWhenSelectionEmpty(t *testing.T) {
	selection := runtime.SelectionState{
		Provider:          "openrouter",
		DeploymentRouting: false,
	}

	sess := NewHawkSession(context.Background(), selection, "openrouter", "openrouter/fallback", "system", nil)
	if got := sess.Model(); got != "openrouter/fallback" {
		t.Fatalf("model = %q, want openrouter/fallback", got)
	}
}
