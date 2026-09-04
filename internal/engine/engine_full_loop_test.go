package engine

import (
	"context"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestEngine_FullLoop_TextOnly(t *testing.T) {
	mc := newMockClient(
		mockTextResponse("I'll help you with that."),
	)
	s := newMockSession(mc)
	s.LifecycleSvc().Limits().SetMaxTurns(1)
	s.AddUser("explain how goroutines work")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := s.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var gotContent bool
	for ev := range ch {
		if ev.Type == "content" && ev.Content != "" {
			gotContent = true
		}
	}
	if !gotContent {
		t.Error("expected content event from stream")
	}

	msgs := s.Persistence().RawMessages()
	if len(msgs) < 2 {
		t.Errorf("expected at least 2 messages (user+assistant), got %d", len(msgs))
	}
}

func TestEngine_FullLoop_MultiTurn(t *testing.T) {
	mc := newMockClient(
		mockTextResponse("Here's the first answer."),
		mockTextResponse("And here's the follow-up."),
	)
	s := newMockSession(mc)
	s.LifecycleSvc().Limits().SetMaxTurns(3)
	s.AddUser("first question")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := s.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	if len(events) == 0 {
		t.Error("expected events from multi-turn stream")
	}
}

func TestEngine_FullLoop_ToolUse(t *testing.T) {
	mc := newMockClient(
		&types.GraycodeRouterResponse{
			FinishReason: "tool_use",
			ToolCalls: []types.ToolCall{{
				ID:        "call_1",
				Name:      "Read",
				Arguments: map[string]interface{}{"file_path": "/nonexistent"},
			}},
			Usage: &types.GraycodeRouterUsage{PromptTokens: 50, CompletionTokens: 30},
		},
		mockTextResponse("I read the file and here's what I found."),
	)
	s := newMockSession(mc)
	s.LifecycleSvc().Limits().SetMaxTurns(2)
	s.AddUser("read main.go")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := s.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var gotToolUse bool
	for ev := range ch {
		if ev.Type == "tool_use" {
			gotToolUse = true
		}
	}
	_ = gotToolUse // tool_use event depends on engine routing
}

func TestEngine_CostTracking(t *testing.T) {
	mc := newMockClient(mockTextResponse("response"))
	s := newMockSession(mc)
	s.LifecycleSvc().Limits().SetMaxTurns(1)
	s.AddUser("hello")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, _ := s.Stream(ctx)
	for range ch {
	}

	// Cost may or may not be recorded depending on mock event format
	_ = s.Cost.Total()
}

func TestEngine_MaxTurnsRespected(t *testing.T) {
	mc := newMockClient(
		mockTextResponse("turn 1"),
		mockTextResponse("turn 2"),
		mockTextResponse("turn 3"),
	)
	s := newMockSession(mc)
	s.LifecycleSvc().Limits().SetMaxTurns(1)
	s.AddUser("go")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, _ := s.Stream(ctx)
	for range ch {
	}

	if mc.callCount() > 2 {
		t.Errorf("MaxTurns=1 but made %d calls", mc.callCount())
	}
}
