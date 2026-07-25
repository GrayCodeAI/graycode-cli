package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSetVersion(t *testing.T) {
	preserveCLICompilerVersionState(t)
	SetVersion("1.2.3")
	if version != "1.2.3" {
		t.Errorf("version = %q, want %q", version, "1.2.3")
	}
}

func TestSetBuildDate(t *testing.T) {
	preserveCLICompilerVersionState(t)
	SetBuildDate("2026-01-01")
	if buildDate != "2026-01-01" {
		t.Errorf("buildDate = %q, want %q", buildDate, "2026-01-01")
	}
}

func TestVersionJSON(t *testing.T) {
	preserveCLICompilerVersionState(t)
	SetVersion("1.2.3")
	SetBuildDate("2026-07-24")

	old := versionJSON
	t.Cleanup(func() { versionJSON = old })
	versionJSON = true

	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	versionCmd.SetErr(&buf)

	versionCmd.Run(versionCmd, nil)

	var info versionInfo
	if err := json.Unmarshal(buf.Bytes(), &info); err != nil {
		t.Fatalf("decoding JSON output %q: %v", buf.String(), err)
	}
	if info.Version != "1.2.3" {
		t.Errorf("version = %q, want 1.2.3", info.Version)
	}
	if info.BuildDate != "2026-07-24" {
		t.Errorf("build_date = %q, want 2026-07-24", info.BuildDate)
	}
}

func TestVersionJSON_OmitsEmptyBuildDate(t *testing.T) {
	preserveCLICompilerVersionState(t)
	SetVersion("1.2.3")
	SetBuildDate("")

	old := versionJSON
	t.Cleanup(func() { versionJSON = old })
	versionJSON = true

	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	versionCmd.SetErr(&buf)

	versionCmd.Run(versionCmd, nil)

	var info versionInfo
	if err := json.Unmarshal(buf.Bytes(), &info); err != nil {
		t.Fatalf("decoding JSON output %q: %v", buf.String(), err)
	}
	if info.BuildDate != "" {
		t.Errorf("build_date should be omitted when empty, got %q", info.BuildDate)
	}
	// Ensure the key is actually absent (omitempty), not just empty.
	if strings.Contains(buf.String(), "build_date") {
		t.Errorf("empty build_date should be omitted from JSON, got %q", buf.String())
	}
}

func TestVersionText_NotJSON(t *testing.T) {
	preserveCLICompilerVersionState(t)
	SetVersion("1.2.3")

	old := versionJSON
	t.Cleanup(func() { versionJSON = old })
	versionJSON = false

	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	versionCmd.SetErr(&buf)

	versionCmd.Run(versionCmd, nil)

	out := buf.String()
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Error("text mode should not start with '{'")
	}
	if !strings.Contains(out, "1.2.3") {
		t.Errorf("expected version in output, got %q", out)
	}
}
