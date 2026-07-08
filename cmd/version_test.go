package cmd

import "testing"

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
