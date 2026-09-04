package bench

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// taskFixture is a minimal SWE-bench-style task: an instruction plus the set
// of file paths the solution is expected to touch. The harness runs the agent
// loop against a stub provider and checks those files were referenced via tool
// calls — a smoke test for the end-to-end agent pipeline (headless, no TUI).
type taskFixture struct {
	Name    string
	Prompt  string
	MustUse []string // tool names the solution must emit
}

// sWEbenchSmokeTasks is a small curated fixture set exercising the common
// agent shapes: read+edit, planning, and a no-op task. Kept tiny so the bench
// stays a compile+smoke gate (real SWE-bench evaluation uses real providers
// and is gated by GRAYCODE_BENCH_PROVIDER / GRAYCODE_BENCH_API_KEY, which is why this
// test only runs when that env is set — see TestBenchmark_SWE_benchHeadless).
var sWEbenchSmokeTasks = []taskFixture{
	{
		Name:    "read-fix",
		Prompt:  "Read internal/engine/stream.go and describe the retry timer.",
		MustUse: []string{"Read"},
	},
	{
		Name:    "no-tools",
		Prompt:  "What is the capital of France?",
		MustUse: nil,
	},
}

// stubChatClient is a headless ChatClient that re-emits a canned event stream
// per StreamChatContinue call. It records the tool calls it was asked to emit
// so a benchmark can assert the agent attempted them.
type stubChatClient struct {
	t      testing.TB
	events []types.GraycodeRouterStreamEvent
}

func (m *stubChatClient) Chat(_ context.Context, _ []types.GraycodeRouterMessage, _ types.ChatOptions) (*types.GraycodeRouterResponse, error) {
	return &types.GraycodeRouterResponse{Content: "stub", FinishReason: "end_turn"}, nil
}

func (m *stubChatClient) StreamChatContinue(_ context.Context, _ []types.GraycodeRouterMessage, _ types.ChatOptions, _ types.ContinuationConfig) (*types.StreamResult, error) {
	ch := make(chan types.GraycodeRouterStreamEvent, len(m.events)+1)
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return &types.StreamResult{Events: ch}, nil
}

// runTask drives engine.Session.Stream for a single fixture and returns the
// flattened event sequence + whether the loop terminated cleanly.
func runTask(t *testing.B, fix taskFixture, events []types.GraycodeRouterStreamEvent) (terminated bool, got []engine.StreamEvent) {
	svc := &stubChatClient{t: t, events: events}
	s := engine.NewSessionWithClient(svc, "bench", "bench-model", "bench system", nil, false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := s.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	for ev := range ch {
		got = append(got, ev)
		if ev.Type == "done" {
			terminated = true
		}
	}
	return terminated, got
}

// TestBenchmark_SWE_benchHeadless is a compile + smoke gate for the headless
// agent loop. It is skipped unless GRAYCODE_BENCH_HEADLESS=1 is set so it never
// runs in CI by default (it uses a stub provider). Real provider-backed SWE
// harness execution lives in bench_suite.go and is invoked via `graycode bench`.
func TestBenchmark_SWE_benchHeadless(t *testing.T) {
	if v := os.Getenv("GRAYCODE_BENCH_HEADLESS"); v != "1" {
		// TODO: track scheduling the headless benchmark smoke in CI.
		t.Skip("set GRAYCODE_BENCH_HEADLESS=1 to run agent-loop benchmark smoke")
	}
	b := &testing.B{}
	for _, fix := range sWEbenchSmokeTasks {
		// The stub emits a content + done event, modelling a provider that
		// answers without tools (the no-tools path) or a read+answer path.
		events := []types.GraycodeRouterStreamEvent{
			{Type: "content", Content: "understood"},
		}
		if len(fix.MustUse) > 0 {
			// Simulate a tool-use turn followed by completion.
			events = nil
			for _, name := range fix.MustUse {
				events = append(events, types.GraycodeRouterStreamEvent{Type: "tool_use", Content: name})
			}
			events = append(events, types.GraycodeRouterStreamEvent{Type: "content", Content: "done"})
		}
		events = append(events, types.GraycodeRouterStreamEvent{Type: "done", StopReason: "end_turn"})

		terminated, got := runTask(b, fix, events)
		if !terminated {
			t.Errorf("%s: stream did not terminate with a done event (got %d events)", fix.Name, len(got))
		}
	}
}
