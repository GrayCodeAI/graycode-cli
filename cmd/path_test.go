package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestPathCmdRuns(t *testing.T) {
	useInMemoryCredentials(t)
	if err := pathCmd.RunE(pathCmd, nil); err == nil {
		t.Skip("machine has full developer path setup") // TODO: https://github.com/GrayCodeAI/hawk/issues/30
	}
}

func TestPathCmdPrintsReport(t *testing.T) {
	useInMemoryCredentials(t)
	out := hawkconfig.FormatDeveloperPathReport(context.Background())
	if !strings.Contains(out, "Developer path") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func useInMemoryCredentials(t *testing.T) {
	t.Helper()
	credentials.SetDefaultStore(&credentials.MapStore{})
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
}
