package eval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSmokeSuiteTasks runs the smoke tasks through the real Runner (stub
// provider, no API key) and asserts both terminate cleanly. This is the CI
// gate for the agent-loop smoke scorecard.
func TestSmokeSuiteTasks(t *testing.T) {
	suite := SmokeSuite()
	if len(suite.Tasks) != 2 {
		t.Fatalf("SmokeSuite has %d tasks, want 2", len(suite.Tasks))
	}

	runner := NewRunner("smoke", "")
	runner.NoCache = true
	runner.Filters = nil

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := runner.Run(ctx, suite)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Passed != 2 {
		for _, r := range result.Results {
			if !r.Passed {
				t.Logf("FAIL %s: %s", r.TaskID, r.Error)
			}
		}
		t.Fatalf("passed = %d/%d, want 2/2", result.Passed, result.TotalTasks)
	}
}

// TestStreamHeadless_ReadTool ensures the driver counts a tool call and
// terminates when the stub asks for Read.
func TestStreamHeadless_ReadTool(t *testing.T) {
	events := []json.RawMessage{
		json.RawMessage(`{"type":"tool_call","tool_call":{"name":"Read","arguments":{"path":"go.mod"}}}`),
		json.RawMessage(`{"type":"content","content":"done"}`),
		json.RawMessage(`{"type":"done","stop_reason":"end_turn"}`),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	steps, calls, _, err := streamHeadless(ctx, events)
	if err != nil {
		t.Fatalf("streamHeadless: %v", err)
	}
	if steps == 0 {
		t.Fatal("no steps emitted")
	}
	if calls != 1 {
		t.Fatalf("tool calls = %d, want 1", calls)
	}
}

// TestSmokeTask_ValidateFn ensures the read-file task's ValidateFn passes
// when the scorecard records a tool call.
func TestSmokeTask_ValidateFn(t *testing.T) {
	dir := t.TempDir()
	card := SmokeScorecard{Steps: 3, ToolCalls: 1, Passed: true}
	if err := os.WriteFile(filepath.Join(dir, "scorecard.json"), mustJSON(card), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, msg := taskSmokeReadFile().ValidateFn(dir)
	if !ok {
		t.Fatalf("ValidateFn failed: %s", msg)
	}
}

// TestSmokeTask_ValidateFn_NoTools ensures the no-tools task rejects a
// scorecard that did not terminate cleanly.
func TestSmokeTask_ValidateFn_NoTools(t *testing.T) {
	dir := t.TempDir()
	card := SmokeScorecard{Steps: 5, Passed: false, Error: "stream closed without a done event"}
	if err := os.WriteFile(filepath.Join(dir, "scorecard.json"), mustJSON(card), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, _ := taskSmokeNoTools().ValidateFn(dir)
	if ok {
		t.Fatal("ValidateFn should fail for a non-terminating run")
	}
}
