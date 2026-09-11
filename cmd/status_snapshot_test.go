package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/sandbox"
)

func TestFormatStatusSnapshot(t *testing.T) {
	snapshot := buildStatusSnapshot()
	formatted := formatStatusSnapshot(snapshot)
	for _, expected := range []string{"Hawk status", "Schema: 1", "Secrets redacted: true"} {
		if !strings.Contains(formatted, expected) {
			t.Errorf("status output missing %q: %s", expected, formatted)
		}
	}
}

func TestStatusSnapshotReportsNativeSandboxBackend(t *testing.T) {
	want := sandbox.SelectSandbox(sandbox.IsolationDefault, ".").Backend
	got := buildStatusSnapshot().Permission.SandboxBackend
	if got != want {
		t.Fatalf("sandbox backend = %q, want selector result %q", got, want)
	}
	if want == "" {
		t.Skip("no sandbox backend on this platform")
	}
}
