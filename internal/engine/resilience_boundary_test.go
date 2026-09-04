package engine

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
	"github.com/GrayCodeAI/graycode-router/llm"
)

type resilienceBoundaryClient struct {
	mu            sync.Mutex
	streams       [][]types.GraycodeRouterStreamEvent
	chatResponses []*types.GraycodeRouterResponse
	streamCalls   int
	chatCalls     int
	messages      [][]types.GraycodeRouterMessage
}

type managedResilienceBoundaryClient struct {
	*resilienceBoundaryClient
}

func (*managedResilienceBoundaryClient) ManagesResilience() bool { return true }

func (c *resilienceBoundaryClient) Chat(context.Context, []types.GraycodeRouterMessage, types.ChatOptions) (*types.GraycodeRouterResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.chatCalls++
	if len(c.chatResponses) == 0 {
		return &types.GraycodeRouterResponse{}, nil
	}
	response := c.chatResponses[0]
	c.chatResponses = c.chatResponses[1:]
	return response, nil
}

func (c *resilienceBoundaryClient) StreamChatContinue(_ context.Context, messages []types.GraycodeRouterMessage, _ types.ChatOptions, _ types.ContinuationConfig) (*types.StreamResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.streamCalls++
	c.messages = append(c.messages, cloneBoundaryMessages(messages))

	events := []types.GraycodeRouterStreamEvent{{Type: "done", StopReason: "end_turn"}}
	if len(c.streams) > 0 {
		events = c.streams[0]
		c.streams = c.streams[1:]
	}
	ch := make(chan types.GraycodeRouterStreamEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return llm.NewStreamResult(ch, "", nil), nil
}

func (c *resilienceBoundaryClient) counts() (stream, chat int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.streamCalls, c.chatCalls
}

func (c *resilienceBoundaryClient) messagesForCall(call int) []types.GraycodeRouterMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneBoundaryMessages(c.messages[call])
}

func cloneBoundaryMessages(messages []types.GraycodeRouterMessage) []types.GraycodeRouterMessage {
	out := make([]types.GraycodeRouterMessage, len(messages))
	copy(out, messages)
	for i := range out {
		out[i].ToolUse = append([]types.ToolCall(nil), messages[i].ToolUse...)
		out[i].ToolResults = append([]types.ToolResult(nil), messages[i].ToolResults...)
	}
	return out
}

func runResilienceBoundarySession(t *testing.T, client ChatClient, configure func(*Session)) []StreamEvent {
	t.Helper()
	session := NewSessionWithClient(client, "test", "test-model", "test system", nil, false)
	session.LifecycleSvc().Limits().SetMaxTurns(5)
	if configure != nil {
		configure(session)
	}
	session.AddUser("test resilience boundary")

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	stream, err := session.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var events []StreamEvent
	for event := range stream {
		events = append(events, event)
	}
	return events
}

func TestAgentLoopResilienceBoundary_StreamRetry(t *testing.T) {
	transient := []types.GraycodeRouterStreamEvent{{Type: "error", Error: "temporary connection reset by peer"}}
	success := []types.GraycodeRouterStreamEvent{
		{Type: "content", Content: "recovered"},
		{Type: "done", StopReason: "end_turn"},
	}

	t.Run("engine facade gets one stream attempt", func(t *testing.T) {
		base := &resilienceBoundaryClient{streams: [][]types.GraycodeRouterStreamEvent{transient, success}}
		events := runResilienceBoundarySession(t, &managedResilienceBoundaryClient{base}, nil)
		streamCalls, chatCalls := base.counts()
		if streamCalls != 1 || chatCalls != 0 {
			t.Fatalf("calls = stream %d, chat %d; want exactly one stream call", streamCalls, chatCalls)
		}
		if !hasBoundaryEvent(events, "error", "temporary connection reset") {
			t.Fatalf("managed stream error was not surfaced: %#v", events)
		}
	})

	t.Run("legacy client retains stream retry", func(t *testing.T) {
		client := &resilienceBoundaryClient{streams: [][]types.GraycodeRouterStreamEvent{transient, success}}
		events := runResilienceBoundarySession(t, client, nil)
		streamCalls, chatCalls := client.counts()
		if streamCalls != 2 || chatCalls != 0 {
			t.Fatalf("calls = stream %d, chat %d; want two stream calls", streamCalls, chatCalls)
		}
		if !hasBoundaryEvent(events, "retry", "transient stream error") || !hasBoundaryEvent(events, "content", "recovered") {
			t.Fatalf("legacy retry behavior missing: %#v", events)
		}
	})
}

func TestAgentLoopResilienceBoundary_ThinkingOnlyRecovery(t *testing.T) {
	thinkingOnly := []types.GraycodeRouterStreamEvent{
		{Type: "thinking", Thinking: "internal reasoning"},
		{Type: "done", StopReason: "end_turn"},
	}
	fallback := &types.GraycodeRouterResponse{Content: "compatibility reply", FinishReason: "end_turn"}

	t.Run("engine facade does not trigger Graycode protocol recovery", func(t *testing.T) {
		base := &resilienceBoundaryClient{
			streams:       [][]types.GraycodeRouterStreamEvent{thinkingOnly},
			chatResponses: []*types.GraycodeRouterResponse{fallback},
		}
		events := runResilienceBoundarySession(t, &managedResilienceBoundaryClient{base}, nil)
		streamCalls, chatCalls := base.counts()
		if streamCalls != 1 || chatCalls != 0 {
			t.Fatalf("calls = stream %d, chat %d; want one stream and no protocol fallback", streamCalls, chatCalls)
		}
		if hasBoundaryEvent(events, "content", "compatibility reply") {
			t.Fatalf("managed client unexpectedly used Graycode thinking fallback: %#v", events)
		}
	})

	t.Run("legacy client retains thinking-only fallback", func(t *testing.T) {
		client := &resilienceBoundaryClient{
			streams:       [][]types.GraycodeRouterStreamEvent{thinkingOnly},
			chatResponses: []*types.GraycodeRouterResponse{fallback},
		}
		events := runResilienceBoundarySession(t, client, nil)
		streamCalls, chatCalls := client.counts()
		if streamCalls != 1 || chatCalls != 1 {
			t.Fatalf("calls = stream %d, chat %d; want stream plus compatibility Chat", streamCalls, chatCalls)
		}
		if !hasBoundaryEvent(events, "content", "compatibility reply") {
			t.Fatalf("legacy thinking fallback missing: %#v", events)
		}
	})
}

func TestAgentLoopResilienceBoundary_MaxTokensContinuation(t *testing.T) {
	partial := []types.GraycodeRouterStreamEvent{
		{Type: "content", Content: "partial"},
		{Type: "done", StopReason: "max_tokens"},
	}
	finished := []types.GraycodeRouterStreamEvent{
		{Type: "content", Content: " finished"},
		{Type: "done", StopReason: "end_turn"},
	}

	t.Run("engine facade does not get a synthetic continuation turn", func(t *testing.T) {
		base := &resilienceBoundaryClient{streams: [][]types.GraycodeRouterStreamEvent{partial, finished}}
		session := NewSessionWithClient(&managedResilienceBoundaryClient{base}, "test", "test-model", "test system", nil, false)
		session.LifecycleSvc().Limits().SetMaxTurns(5)
		session.AddUser("continue test")
		drainBoundarySession(t, session)

		streamCalls, chatCalls := base.counts()
		if streamCalls != 1 || chatCalls != 0 {
			t.Fatalf("calls = stream %d, chat %d; want exactly one facade stream", streamCalls, chatCalls)
		}
		messages := session.Persistence().RawMessages()
		if len(messages) != 2 || messages[1].Role != "assistant" || messages[1].Content != "partial" {
			t.Fatalf("managed conversation = %#v, want user plus partial assistant", messages)
		}
		for _, message := range messages {
			if strings.Contains(message.Content, "Continue from where you left off") {
				t.Fatalf("synthetic continuation leaked into managed conversation: %#v", messages)
			}
		}
	})

	t.Run("legacy client retains synthetic continuation", func(t *testing.T) {
		client := &resilienceBoundaryClient{streams: [][]types.GraycodeRouterStreamEvent{partial, finished}}
		session := NewSessionWithClient(client, "test", "test-model", "test system", nil, false)
		session.LifecycleSvc().Limits().SetMaxTurns(5)
		session.AddUser("continue test")
		drainBoundarySession(t, session)

		streamCalls, chatCalls := client.counts()
		if streamCalls != 2 || chatCalls != 0 {
			t.Fatalf("calls = stream %d, chat %d; want two compatibility streams", streamCalls, chatCalls)
		}
		messages := session.Persistence().RawMessages()
		if len(messages) != 4 || messages[2].Content != "Continue from where you left off." || messages[3].Content != " finished" {
			t.Fatalf("legacy continuation conversation = %#v", messages)
		}
	})
}

func TestAgentLoopResilienceBoundary_KeepsGraycodeToolAuthorizationAndMutation(t *testing.T) {
	base := &resilienceBoundaryClient{streams: [][]types.GraycodeRouterStreamEvent{
		{
			{Type: "tool_call", ToolCall: &types.ToolCall{ID: "write-1", Name: "Write", Arguments: map[string]interface{}{"file_path": "blocked.txt", "content": "no"}}},
			{Type: "done", StopReason: "tool_use"},
		},
		{
			{Type: "content", Content: "handled denial"},
			{Type: "done", StopReason: "end_turn"},
		},
	}}
	session := NewSessionWithClient(&managedResilienceBoundaryClient{base}, "test", "test-model", "test system", nil, false)
	session.LifecycleSvc().Limits().SetMaxTurns(5)
	session.PermSvc().SetDryRun(true)
	session.AddUser("attempt a write")
	events := drainBoundarySession(t, session)

	streamCalls, chatCalls := base.counts()
	if streamCalls != 2 || chatCalls != 0 {
		t.Fatalf("calls = stream %d, chat %d; want one facade call per Graycode agent turn", streamCalls, chatCalls)
	}
	if !hasBoundaryEvent(events, "tool_result", "dry-run") {
		t.Fatalf("Graycode tool denial was not emitted: %#v", events)
	}
	secondRequest := base.messagesForCall(1)
	if len(secondRequest) < 3 || len(secondRequest[1].ToolUse) != 1 || len(secondRequest[2].ToolResults) != 1 {
		t.Fatalf("Graycode did not persist tool call/result conversation shape: %#v", secondRequest)
	}
	if !secondRequest[2].ToolResults[0].IsError || !strings.Contains(secondRequest[2].ToolResults[0].Content, "dry-run") {
		t.Fatalf("tool authorization result = %#v, want persisted denial", secondRequest[2].ToolResults[0])
	}
}

func drainBoundarySession(t *testing.T, session *Session) []StreamEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	stream, err := session.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var events []StreamEvent
	for event := range stream {
		events = append(events, event)
	}
	return events
}

func hasBoundaryEvent(events []StreamEvent, eventType, content string) bool {
	for _, event := range events {
		if event.Type == eventType && strings.Contains(event.Content, content) {
			return true
		}
	}
	return false
}
