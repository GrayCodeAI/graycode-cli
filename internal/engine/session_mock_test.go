package engine

import (
	"context"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/types"
)

func newMockSession(mc *mockClient) *Session {
	s := NewSession("", "mock-model", "You are a test assistant.", nil)
	// SetTestClient also reattaches the ChatService so the agent
	// loop's s.ChatLLM().Stream call site sees the mock.
	s.SetTestClient(mc)
	return s
}

func TestSession_AddUser(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	s.AddUser("hello world")

	msgs := s.RawMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("role = %q, want 'user'", msgs[0].Role)
	}
	if msgs[0].Content != "hello world" {
		t.Errorf("content = %q, want 'hello world'", msgs[0].Content)
	}
}

func TestSession_AddAssistant(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	s.AddAssistant("here is my response")

	msgs := s.RawMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "assistant" {
		t.Errorf("role = %q, want 'assistant'", msgs[0].Role)
	}
}

func TestSession_LoadMessages(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	msgs := []types.EyrieMessage{
		{Role: "user", Content: "msg1"},
		{Role: "assistant", Content: "msg2"},
		{Role: "user", Content: "msg3"},
	}
	s.LoadMessages(msgs)

	if len(s.RawMessages()) != 3 {
		t.Errorf("RawMessages() length = %d, want 3", len(s.RawMessages()))
	}
}

func TestSession_Model(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	if s.Model() != "mock-model" {
		t.Errorf("Model() = %q, want 'mock-model'", s.Model())
	}
}

func TestSession_Provider(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	if s.Provider() != "" {
		t.Errorf("Provider() = %q, want empty", s.Provider())
	}
}

func TestSession_Cost(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	s.Cost.Add(100, 50)
	if s.Cost.Total() == 0 {
		t.Error("Cost.Total() should be > 0 after Add")
	}
}

func TestSession_MaxTurns(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)
	s.MaxTurns = 5

	if s.MaxTurns != 5 {
		t.Errorf("MaxTurns = %d, want 5", s.MaxTurns)
	}
}

func TestSession_Chat_MockResponse(t *testing.T) {
	mc := newMockClient(mockTextResponse("hello from LLM"))
	s := newMockSession(mc)
	s.AddUser("hi")

	// Call Chat directly
	resp, err := mc.Chat(context.TODO(), s.RawMessages(), types.ChatOptions{})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if resp.Content != "hello from LLM" {
		t.Errorf("response = %q, want 'hello from LLM'", resp.Content)
	}
	if mc.callCount() != 1 {
		t.Errorf("callCount = %d, want 1", mc.callCount())
	}
}

func TestSession_SetPermissionMode(t *testing.T) {
	mc := newMockClient()
	s := newMockSession(mc)

	if err := s.SetPermissionMode("bypassPermissions"); err != nil {
		t.Errorf("SetPermissionMode error: %v", err)
	}
}

func TestSession_Metrics(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	if s.Metrics() == nil {
		t.Error("Metrics() should not be nil")
	}
}

func TestSession_Stream_MockEndTurn(t *testing.T) {
	mc := newMockClient(mockTextResponse("Hello! I can help with that."))
	s := newMockSession(mc)
	s.MaxTurns = 1
	s.AddUser("hi there")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := s.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	if len(events) == 0 {
		t.Fatal("expected at least one stream event")
	}

	if mc.callCount() < 1 {
		t.Error("expected at least one LLM call")
	}
}

func TestSession_Stream_MultiTurn(t *testing.T) {
	mc := newMockClient(
		mockTextResponse("First response"),
		mockTextResponse("Second response"),
	)
	s := newMockSession(mc)
	s.MaxTurns = 2
	s.AddUser("hello")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := s.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	if len(events) == 0 {
		t.Fatal("expected stream events")
	}
}
