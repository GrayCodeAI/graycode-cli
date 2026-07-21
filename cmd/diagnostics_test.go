package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
	"github.com/GrayCodeAI/hawk/internal/resilience/health"
)

type diagnosticsContextKey struct{}

type contextRecordingCredentialStore struct {
	gateway.MapStore
	contextValue any
}

func (s *contextRecordingCredentialStore) Get(ctx context.Context, account string) (string, error) {
	s.contextValue = ctx.Value(diagnosticsContextKey{})
	return s.MapStore.Get(ctx, account)
}

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
	if !strings.Contains(report, "Ecosystem (eyrie · yaad · tok)") {
		t.Error("report should include ecosystem panel")
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

func TestDoctorReportUsesResolvedProviderForChecks(t *testing.T) {
	isolateCredentialHome(t)
	hawkconfig.InvalidateConfigUICache()
	gateway.SetDefaultStore(&gateway.MapStore{})
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	report := doctorReport(hawkconfig.Settings{
		Model:    "claude-sonnet-4-20250514",
		Provider: "anthropic",
	})
	if !strings.Contains(report, "provider anthropic") || !strings.Contains(report, "anthropic_api_key") {
		t.Fatalf("doctor checks did not use resolved provider:\n%s", report)
	}
}

func TestProviderCredentialHealthCheckerResolvesAuto(t *testing.T) {
	isolateCredentialHome(t)
	t.Setenv("EYRIE_CONFIG_DIR", t.TempDir())
	hawkconfig.InvalidateConfigUICache()
	store := &contextRecordingCredentialStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.WithValue(context.Background(), diagnosticsContextKey{}, "checker-context")
	if err := store.Set(ctx, gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890"); err != nil {
		t.Fatal(err)
	}
	if err := hawkconfig.SetActiveProvider(ctx, "openrouter"); err != nil {
		t.Fatal(err)
	}
	if err := hawkconfig.SetActiveModel(ctx, "gpt-4o"); err != nil {
		t.Fatal(err)
	}
	store.contextValue = nil

	started := time.Now()
	result := providerCredentialHealthChecker(" auto ")(ctx)
	finished := time.Now()
	if result.Name != "openrouter_api_key" {
		t.Fatalf("name = %q, want openrouter_api_key", result.Name)
	}
	if result.Status != health.Healthy {
		t.Fatalf("status = %q, want healthy (%s)", result.Status, result.Message)
	}
	if result.LastChecked.Before(started) || result.LastChecked.After(finished) {
		t.Fatalf("last checked = %v, want between %v and %v", result.LastChecked, started, finished)
	}
	if result.Duration <= 0 {
		t.Fatalf("duration = %v, want populated timing", result.Duration)
	}
	if store.contextValue != "checker-context" {
		t.Fatalf("credential lookup context = %v, want checker-context", store.contextValue)
	}
}

func TestProviderCredentialHealthCheckerMissingIsUnhealthy(t *testing.T) {
	isolateCredentialHome(t)
	t.Setenv("EYRIE_CONFIG_DIR", t.TempDir())
	hawkconfig.InvalidateConfigUICache()
	gateway.SetDefaultStore(&gateway.MapStore{})
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	result := providerCredentialHealthChecker("openai")(context.Background())
	if result.Name != "openai_api_key" {
		t.Fatalf("name = %q, want openai_api_key", result.Name)
	}
	if result.Status != health.Unhealthy {
		t.Fatalf("status = %q, want unhealthy (%s)", result.Status, result.Message)
	}
	if result.LastChecked.IsZero() {
		t.Fatal("last checked timestamp was not populated")
	}
	if result.Duration < 0 {
		t.Fatalf("duration = %v, want non-negative timing", result.Duration)
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
