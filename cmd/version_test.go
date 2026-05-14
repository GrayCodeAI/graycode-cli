package cmd

import "testing"

func TestSetVersion(t *testing.T) {
	t.Parallel()
	SetVersion("1.2.3")
	if version != "1.2.3" {
		t.Errorf("version = %q, want %q", version, "1.2.3")
	}
}

func TestSetBuildDate(t *testing.T) {
	t.Parallel()
	SetBuildDate("2026-01-01")
	if buildDate != "2026-01-01" {
		t.Errorf("buildDate = %q, want %q", buildDate, "2026-01-01")
	}
}

func TestVersionString(t *testing.T) {
	t.Parallel()
	Version = "test-ver"
	Commit = "abc123"
	Date = "2026-05-15"
	s := VersionString()
	if s == "" {
		t.Error("VersionString() should not be empty")
	}
}

func TestShortVersion(t *testing.T) {
	t.Parallel()
	Version = "0.4.0"
	got := ShortVersion()
	if got != "0.4.0" {
		t.Errorf("ShortVersion() = %q, want %q", got, "0.4.0")
	}
}
