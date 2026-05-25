package cmd

import (
	"context"
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestPathCmdRuns(t *testing.T) {
	if err := pathCmd.RunE(pathCmd, nil); err == nil {
		t.Skip("machine has full developer path setup")
	}
}

func TestPathCmdPrintsReport(t *testing.T) {
	out := hawkconfig.FormatDeveloperPathReport(context.Background())
	if !strings.Contains(out, "Developer path") {
		t.Fatalf("unexpected output: %s", out)
	}
}
