package config

import (
	"context"
	"strings"
	"testing"
)

func TestFormatEcosystemPanel(t *testing.T) {
	t.Parallel()
	out := FormatEcosystemPanel(context.Background(), "anthropic", "claude-sonnet-4-20250514")
	for _, want := range []string{
		"Ecosystem (graycode-router · harrier · shrike):",
		"graycode-router:",
		"harrier:",
		"shrike:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("panel missing %q:\n%s", want, out)
		}
	}
}
