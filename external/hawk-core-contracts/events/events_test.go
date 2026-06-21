package events_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk-core-contracts/events"
)

func TestToolEventJSONRoundTrip(t *testing.T) {
	ts := time.Date(2026, time.June, 21, 10, 30, 0, 0, time.UTC)
	in := events.ToolEvent{
		ToolName:  "exec_command",
		ToolInput: map[string]interface{}{"cmd": "pwd"},
		CWD:       "/workspace",
		Timestamp: ts,
		SessionID: "sess_123",
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var out events.ToolEvent
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if out.ToolName != in.ToolName {
		t.Fatalf("ToolName = %q, want %q", out.ToolName, in.ToolName)
	}
	if out.CWD != in.CWD {
		t.Fatalf("CWD = %q, want %q", out.CWD, in.CWD)
	}
	if !out.Timestamp.Equal(ts) {
		t.Fatalf("Timestamp = %v, want %v", out.Timestamp, ts)
	}
}

func TestTraceEventJSONIncludesUsage(t *testing.T) {
	event := events.TraceEvent{
		ID:        "trace_123",
		Name:      "model_completion",
		StartTime: time.Date(2026, time.June, 21, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, time.June, 21, 10, 0, 1, 0, time.UTC),
		Usage: &events.UsageInfo{
			PromptTokens:     10,
			CompletionTokens: 15,
			TotalTokens:      25,
			CostUSD:          0.01,
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	usage, ok := got["usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("usage = %#v, want object", got["usage"])
	}
	if usage["total_tokens"] != float64(25) {
		t.Fatalf("usage[total_tokens] = %#v, want 25", usage["total_tokens"])
	}
}
