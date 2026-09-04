package cmd

import (
	"strings"
	"testing"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
)

func TestEcosystemCmdRuns(t *testing.T) {
	settings := graycodeconfig.Settings{}
	model, provider := effectiveModelAndProvider(settings)
	if provider == "" {
		provider = "auto"
	}
	out := graycodeconfig.FormatEcosystemPanel(t.Context(), provider, model)
	if !strings.Contains(out, "Ecosystem (graycode-router · harrier · shrike)") {
		t.Fatalf("unexpected panel: %q", out)
	}
	if err := ecosystemCmd.RunE(ecosystemCmd, nil); err != nil {
		t.Fatalf("ecosystem command: %v", err)
	}
}
