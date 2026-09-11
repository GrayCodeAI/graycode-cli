package cmd

import (
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestEcosystemCmdRuns(t *testing.T) {
	settings := hawkconfig.Settings{}
	model, provider := effectiveModelAndProvider(settings)
	if provider == "" {
		provider = "auto"
	}
	out := hawkconfig.FormatEcosystemPanel(t.Context(), provider, model)
	if !strings.Contains(out, "Ecosystem (eyrie · harrier · shrike)") {
		t.Fatalf("unexpected panel: %q", out)
	}
	if err := ecosystemCmd.RunE(ecosystemCmd, nil); err != nil {
		t.Fatalf("ecosystem command: %v", err)
	}
}
