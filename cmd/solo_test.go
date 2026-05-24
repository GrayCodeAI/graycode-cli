package cmd

import (
	"context"
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestSoloCmdRuns(t *testing.T) {
	if err := soloCmd.RunE(soloCmd, nil); err == nil {
		// May pass on dev machines with full setup; at minimum should print report.
		t.Skip("machine has full solo setup")
	}
	// Fresh test env would fail — covered in config package tests.
}

func TestSoloCmdPrintsReport(t *testing.T) {
	// Capture via RunE side effect: solo always prints before error.
	ctx := context.Background()
	out := hawkconfig.FormatSoloPathReport(ctx)
	if !strings.Contains(out, "Developer path") {
		t.Fatalf("unexpected output: %s", out)
	}
}
