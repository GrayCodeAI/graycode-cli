package eyrieclient_test

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/eyrieclient"
)

func TestPreflight_ReturnsChecks(t *testing.T) {
	r := eyrieclient.Preflight(context.Background())
	if len(r.Checks) == 0 {
		t.Fatal("expected checks")
	}
	out := eyrieclient.FormatPreflightReport(r)
	if !strings.Contains(out, "Preflight:") {
		t.Fatal(out)
	}
}
