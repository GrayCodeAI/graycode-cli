package eval

// Headless agent-loop driver used by the smoke benchmarks. It drives the real
// engine.Session.Stream loop with a stub ChatClient (no provider, no API key)
// and reports steps, tool calls, and token usage — the "scorecard" for the
// agent pipeline itself. Used by `hawk eval smoke`.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// smokeChatClient is a stub ChatClient that replays a canned event stream per
// StreamChatContinue call. It records the tool calls it was asked to emit so
// the driver can score whether the agent attempted them.
type smokeChatClient struct {
	events []types.EyrieStreamEvent
	calls  []string
	used   bool
}

func (m *smokeChatClient) Chat(_ context.Context, _ []types.EyrieMessage, _ types.ChatOptions) (*types.EyrieResponse, error) {
	return &types.EyrieResponse{Content: "stub", FinishReason: "end_turn"}, nil
}

func (m *smokeChatClient) StreamChatContinue(_ context.Context, _ []types.EyrieMessage, _ types.ChatOptions, _ types.ContinuationConfig) (*types.StreamResult, error) {
	// Emit the scripted stream once; on subsequent turns (tool-result loops)
	// fall back to a plain answer so the loop terminates instead of replaying
	// the tool call forever.
	events := m.events
	if m.used {
		events = []types.EyrieStreamEvent{
			{Type: "content", Content: "done"},
			{Type: "done", StopReason: "end_turn"},
		}
	}
	m.used = true
	ch := make(chan types.EyrieStreamEvent, len(events)+1)
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return &types.StreamResult{Events: ch}, nil
}

// streamHeadless runs the agent loop once and returns (steps, toolCalls,
// tokens). It never contacts a provider — the stub client replays the given
// events, and tools are wired to real registered tools (spawn/background
// through SpawnController) so the loop exercises the real execution path.
func streamHeadless(ctx context.Context, events []json.RawMessage) (int, int, int, error) {
	client := &smokeChatClient{}
	for _, raw := range events {
		var ev types.EyrieStreamEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			return 0, 0, 0, fmt.Errorf("bad event json: %w", err)
		}
		if ev.Type == "tool_call" && ev.ToolCall != nil {
			client.calls = append(client.calls, ev.ToolCall.Name)
		}
		client.events = append(client.events, ev)
	}

	// Tool service wires the production essential tool set so the loop
	// exercises the real execution path (permissions, sandbox, registry).
	s := engine.NewSessionWithClient(client, "smoke", "smoke-model", "smoke system prompt",
		tool.NewRegistry(tool.BashTool{}, tool.FileReadTool{}, tool.FileWriteTool{}, tool.FileEditTool{},
			tool.LSTool{}, tool.GrepTool{}, tool.WebFetchTool{}, tool.ToolSearchTool{},
			tool.AgentTool{}, tool.AskUserQuestionTool{}, tool.TodoWriteTool{},
			tool.MonitorTool{}, tool.MultiEditTool{}), false)
	// Auto-approve every tool so the loop executes them rather than stalling
	// on permission prompts.
	s.PermSvc().Memory().AlwaysAllow("*")
	s.WireAgentTool()
	s.Tools().EnsureBackgroundManager()

	s.AddUser("smoke task")

	// Quiet the engine logger so the scorecard report is the only stdout.
	s.SetLogger(logger.New(io.Discard, logger.Error))

	ch, err := s.Stream(ctx)
	if err != nil {
		return 0, 0, 0, err
	}

	steps, toolCalls, tokens := 0, 0, 0
	terminated := false
	for ev := range ch {
		steps++
		switch ev.Type {
		case "tool_use":
			toolCalls++
		case "usage":
			if ev.Usage != nil {
				tokens += ev.Usage.PromptTokens + ev.Usage.CompletionTokens
			}
		case "done":
			terminated = true
		}
	}
	if !terminated {
		return steps, toolCalls, tokens, fmt.Errorf("stream closed without a done event")
	}
	return steps, toolCalls, tokens, nil
}

// Verify that the smoke driver's stub client satisfies the engine contract.
var _ engine.ChatClient = (*smokeChatClient)(nil)
