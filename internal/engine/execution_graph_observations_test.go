package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/graphjournal"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
	"github.com/GrayCodeAI/tok"
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
	t.Setenv("HAWK_STATE_DIR", t.TempDir())

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

func TestTokCompressionObservationIsPrivacySafe(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	sess := NewSession("test", "test", "system", tool.NewRegistry())
	sess.SetPersistID("tok-runtime-session")
	sess.recordTokCompressionObservation(
		"private conversation with sk-secret",
		"context-compaction",
		tok.Stats{OriginalTokens: 100, FinalTokens: 40, TokensSaved: 60, Model: "private-model"},
	)
	entries, err := graphjournal.Load("tok-runtime-session")
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
			t.Fatalf("Tok graph observation leaked %q", secret)
		}
	}
}

func TestTokRedactionObservationIsPrivacySafe(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	sess := NewSession("test", "test", "system", tool.NewRegistry())
	sess.SetPersistID("tok-redaction-session")
	sess.recordTokRedactionObservation(
		"response containing sk-secret",
		2,
		map[string]int{"OpenAI API Key": 2},
	)
	entries, err := graphjournal.Load("tok-redaction-session")
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
			t.Fatalf("Tok redaction graph leaked %q", secret)
		}
	}
}

func TestTokUsageBudgetObservationTracksAndProjectsAuthoritativeUsage(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	sess := NewSession("test", "test", "system", tool.NewRegistry())
	sess.SetPersistID("tok-usage-session")
	if err := sess.SetMaxBudgetUSD(1); err != nil {
		t.Fatal(err)
	}

	sess.recordTokUsageBudgetObservation(
		120,
		0.25,
		"private-provider",
		"private/model",
	)

	tracker := sess.currentTokUsageTracker()
	if tracker == nil {
		t.Fatal("Tok usage tracker was not initialized")
	}
	usage := tracker.GetUsage()
	if usage.SessionTokens != 120 || usage.DailyCostUSD != 0.25 {
		t.Fatalf("Tok usage = %+v, want 120 tokens and $0.25", usage)
	}

	entries, err := graphjournal.Load("tok-usage-session")
	if err != nil {
		t.Fatalf("graphjournal.Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Runtime == nil {
		t.Fatalf("runtime entries = %#v, want one", entries)
	}
	if len(entries[0].Runtime.Nodes) != 2 ||
		len(entries[0].Runtime.Edges) != 1 ||
		len(entries[0].Runtime.Events) != 2 {
		t.Fatalf("Tok usage topology = %#v", entries[0].Runtime)
	}
	payload, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-provider", "private/model"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("Tok usage graph leaked %q", secret)
		}
	}
}

func TestTokUsageBudgetStopsAtConfiguredLimit(t *testing.T) {
	sess := NewSession("test", "test", "system", tool.NewRegistry())
	tracker := sess.ensureTokUsageTracker()
	limits := tracker.GetLimits()
	limits.SessionTokens = 100
	tracker.SetLimits(limits)
	tracker.Record(100, 0, "provider", "model")

	allowed, reason := sess.tokUsageCanProceed()
	if allowed || !strings.Contains(reason, "session token limit") {
		t.Fatalf("budget decision = %v/%q, want session token denial", allowed, reason)
	}
}

func TestEyrieOperationObservationIsPrivacySafe(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	sess := NewSession("test", "test", "system", tool.NewRegistry())
	sess.SetPersistID("eyrie-runtime-session")
	sess.recordEyrieOperationObservation(
		"private-provider",
		"private/model",
		"stop",
		"private generated content",
		2,
		&types.EyrieUsage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
	)
	entries, err := graphjournal.Load("eyrie-runtime-session")
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
			t.Fatalf("Eyrie graph observation leaked %q", secret)
		}
	}
}

var _ tool.Tool = graphVerifyTool{}
