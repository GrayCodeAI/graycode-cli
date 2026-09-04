package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/graphjournal"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
	shrike "github.com/GrayCodeAI/shrike"
)

type graphVerifyTool struct{}

func (graphVerifyTool) Name() string        { return "VerifyPlanExecution" }
func (graphVerifyTool) Description() string { return "test verification tool" }
func (graphVerifyTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}

func (graphVerifyTool) Execute(context.Context, json.RawMessage) (string, error) {
	return `{"allVerified":false,"totalSteps":3,"verified":2}`, nil
}

func TestToolExecutionAutomaticallyRecordsPolicyAndVerification(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())

	sess := NewSession("test", "test", "system", tool.NewRegistry(graphVerifyTool{}))
	sess.SetPersistID("graph-runtime-session")
	sess.PermSvc().SetAutonomy(AutonomyYOLO)
	ch := make(chan StreamEvent, 4)

	result := sess.executeSingleTool(
		context.Background(),
		types.ToolCall{ID: "verify-1", Name: "VerifyPlanExecution"},
		ch,
		1,
		"verify the plan",
	)
	if result.isErr {
		t.Fatalf("executeSingleTool() returned error: %s", result.output)
	}

	entries, err := graphjournal.Load("graph-runtime-session")
	if err != nil {
		t.Fatalf("graphjournal.Load() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want policy and verification", len(entries))
	}
	if entries[0].Policy == nil || !entries[0].Policy.Allowed {
		t.Fatal("automatic permission observation missing")
	}
	if entries[1].Verification == nil {
		t.Fatal("automatic verification observation missing")
	}
	if !entries[1].Verification.Failed || entries[1].Verification.FindingCount != 1 {
		t.Fatalf("verification summary = %#v, want one failed check", entries[1].Verification)
	}
}

func TestShrikeCompressionObservationIsPrivacySafe(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())
	sess := NewSession("test", "test", "system", tool.NewRegistry())
	sess.SetPersistID("shrike-runtime-session")
	sess.recordShrikeCompressionObservation(
		"private conversation with sk-secret",
		"context-compaction",
		shrike.Stats{OriginalTokens: 100, FinalTokens: 40, TokensSaved: 60, Model: "private-model"},
	)
	entries, err := graphjournal.Load("shrike-runtime-session")
	if err != nil {
		t.Fatalf("graphjournal.Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Runtime == nil {
		t.Fatalf("runtime entries = %#v, want one", entries)
	}
	payload, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, secret := range []string{"private conversation with sk-secret", "private-model"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("Shrike graph observation leaked %q", secret)
		}
	}
}

func TestShrikeRedactionObservationIsPrivacySafe(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())
	sess := NewSession("test", "test", "system", tool.NewRegistry())
	sess.SetPersistID("shrike-redaction-session")
	sess.recordShrikeRedactionObservation(
		"response containing sk-secret",
		2,
		map[string]int{"OpenAI API Key": 2},
	)
	entries, err := graphjournal.Load("shrike-redaction-session")
	if err != nil {
		t.Fatalf("graphjournal.Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Runtime == nil {
		t.Fatalf("runtime entries = %#v, want one", entries)
	}
	payload, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, secret := range []string{"response containing sk-secret", "OpenAI API Key"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("Shrike redaction graph leaked %q", secret)
		}
	}
}

func TestShrikeUsageBudgetObservationTracksAndProjectsAuthoritativeUsage(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())
	sess := NewSession("test", "test", "system", tool.NewRegistry())
	sess.SetPersistID("shrike-usage-session")
	if err := sess.SetMaxBudgetUSD(1); err != nil {
		t.Fatal(err)
	}

	sess.recordShrikeUsageBudgetObservation(
		120,
		0.25,
		"private-provider",
		"private/model",
	)

	tracker := sess.currentShrikeUsageTracker()
	if tracker == nil {
		t.Fatal("Shrike usage tracker was not initialized")
	}
	usage := tracker.GetUsage()
	if usage.SessionTokens != 120 || usage.DailyCostUSD != 0.25 {
		t.Fatalf("Shrike usage = %+v, want 120 tokens and $0.25", usage)
	}

	entries, err := graphjournal.Load("shrike-usage-session")
	if err != nil {
		t.Fatalf("graphjournal.Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Runtime == nil {
		t.Fatalf("runtime entries = %#v, want one", entries)
	}
	if len(entries[0].Runtime.Nodes) != 2 ||
		len(entries[0].Runtime.Edges) != 1 ||
		len(entries[0].Runtime.Events) != 2 {
		t.Fatalf("Shrike usage topology = %#v", entries[0].Runtime)
	}
	payload, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-provider", "private/model"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("Shrike usage graph leaked %q", secret)
		}
	}
}

func TestShrikeUsageBudgetStopsAtConfiguredLimit(t *testing.T) {
	sess := NewSession("test", "test", "system", tool.NewRegistry())
	tracker := sess.ensureShrikeUsageTracker()
	limits := tracker.GetLimits()
	limits.SessionTokens = 100
	tracker.SetLimits(limits)
	tracker.Record(100, 0, "provider", "model")

	allowed, reason := sess.shrikeUsageCanProceed()
	if allowed || !strings.Contains(reason, "session token limit") {
		t.Fatalf("budget decision = %v/%q, want session token denial", allowed, reason)
	}
}

func TestApplyShrikeUsageSettingsOverridesAndDisables(t *testing.T) {
	sess := NewSession("test", "test", "system", tool.NewRegistry())
	// Defaults: token ceilings off (provider rate limits own throughput).
	defaults := sess.ensureShrikeUsageTracker().GetLimits()
	if defaults.HourlyTokens != 0 || defaults.DailyTokens != 0 || defaults.SessionTokens != 0 {
		t.Fatalf("expected disabled token ceilings by default, got %#v", defaults)
	}

	sess.ApplyShrikeUsageSettings(250_000, -1, 0)
	limits := sess.ensureShrikeUsageTracker().GetLimits()
	if limits.HourlyTokens != 250_000 {
		t.Fatalf("HourlyTokens = %d, want 250000", limits.HourlyTokens)
	}
	if limits.DailyTokens != 0 {
		t.Fatalf("DailyTokens = %d, want 0 (disabled)", limits.DailyTokens)
	}
	if limits.SessionTokens != 0 {
		t.Fatalf("SessionTokens = %d, want 0 (still disabled)", limits.SessionTokens)
	}
}

func TestDrainAlertsSurfacesHourlyWarning(t *testing.T) {
	tracker := shrike.NewUsageTracker()
	tracker.SetLimits(shrike.UsageLimits{
		HourlyTokens:  100,
		DailyTokens:   10_000,
		SessionTokens: 10_000,
		CostUSD:       10,
	})
	tracker.Record(55, 0, "provider", "model")
	alerts := tracker.DrainAlerts()
	if len(alerts) == 0 {
		t.Fatal("expected threshold alert after crossing 50%")
	}
	if len(tracker.DrainAlerts()) != 0 {
		t.Fatal("expected DrainAlerts to clear pending alerts")
	}
}

func TestGraycodeRouterOperationObservationIsPrivacySafe(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())
	sess := NewSession("test", "test", "system", tool.NewRegistry())
	sess.SetPersistID("graycode-router-runtime-session")
	sess.recordGraycodeRouterOperationObservation(
		"private-provider",
		"private/model",
		"stop",
		"private generated content",
		2,
		&types.GraycodeRouterUsage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
	)
	entries, err := graphjournal.Load("graycode-router-runtime-session")
	if err != nil {
		t.Fatalf("graphjournal.Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Runtime == nil {
		t.Fatalf("runtime entries = %#v, want one", entries)
	}
	payload, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, secret := range []string{"private-provider", "private/model", "private generated content"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("GraycodeRouter graph observation leaked %q", secret)
		}
	}
}

var _ tool.Tool = graphVerifyTool{}
