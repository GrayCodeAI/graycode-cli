package cmd

import (
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestDoctorReport(t *testing.T) {
	t.Parallel()
	settings := hawkconfig.Settings{}
	report := doctorReport(settings)
	if report == "" {
		t.Error("doctorReport should produce non-empty output")
	}
	if !strings.Contains(report, "Version") {
		t.Error("report should mention version")
	}
}

func TestDoctorReportProviderModelOrder(t *testing.T) {
	t.Parallel()
	settings := hawkconfig.Settings{
		Model:    "claude-sonnet-4-20250514",
		Provider: "anthropic",
	}
	model, provider := effectiveModelAndProvider(settings)
	report := doctorReport(settings)
	if model != "" && !strings.Contains(report, "Model: "+model) {
		t.Errorf("report should show model %q, got:\n%s", model, report)
	}
	if provider != "" && !strings.Contains(report, "Provider: "+provider) {
		t.Errorf("report should show provider %q, got:\n%s", provider, report)
	}
}

func TestSettingsSummary(t *testing.T) {
	t.Parallel()
	settings := hawkconfig.Settings{
		Model:    "claude-sonnet-4-20250514",
		Provider: "anthropic",
	}
	summary := settingsSummary(settings)
	if summary == "" {
		t.Error("settingsSummary should produce output")
	}
}

func TestMcpConfigSummary(t *testing.T) {
	t.Parallel()
	settings := hawkconfig.Settings{}
	summary := mcpConfigSummary(settings)
	if summary == "" {
		t.Error("mcpConfigSummary should produce output")
	}
}

func TestBuiltInToolsSummary(t *testing.T) {
	t.Parallel()
	summary := builtInToolsSummary()
	if summary == "" {
		t.Error("builtInToolsSummary should produce output")
	}
	if !strings.Contains(summary, "Bash") {
		t.Error("should list Bash tool")
	}
	if !strings.Contains(summary, "Read") {
		t.Error("should list Read tool")
	}
}

func TestSessionsSummary(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	summary := sessionsSummary()
	if summary == "" {
		t.Error("sessionsSummary should produce output even with no sessions")
	}
}
