package async

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// fakeClient is a scripted ChatClient for exercising the engine loop.
type fakeClient struct {
	mu            sync.Mutex
	script        []types.EyrieStreamEvent
	streamStarted chan struct{}
	streamCtx     context.Context
}

func newFakeClient(events ...types.EyrieStreamEvent) *fakeClient {
	return &fakeClient{
		script:        events,
		streamStarted: make(chan struct{}),
	}
}

func (f *fakeClient) Chat(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	return &types.EyrieResponse{Content: "mock", FinishReason: "end_turn"}, nil
}

func (f *fakeClient) StreamChatContinue(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions, cfg types.ContinuationConfig) (*types.StreamResult, error) {
	f.mu.Lock()
	f.streamCtx = ctx
	f.mu.Unlock()
	select {
	case <-f.streamStarted:
	default:
		close(f.streamStarted)
	}

	ch := make(chan types.EyrieStreamEvent, 16)
	go func() {
		defer close(ch)
		for _, evt := range f.script {
			select {
			case ch <- evt:
			case <-ctx.Done():
				return
			}
		}
	}()
	return &types.StreamResult{Events: ch}, nil
}

// cancelTurns is a client whose FIRST stream blocks until the context is
// canceled (emitting one content event first); subsequent streams complete
// normally with a done event.
type cancelTurns struct {
	streamStarted chan struct{}
	mu            sync.Mutex
	call          int
}

func (c *cancelTurns) Chat(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions) (*types.EyrieResponse, error) {
	return &types.EyrieResponse{Content: "mock", FinishReason: "end_turn"}, nil
}

func (c *cancelTurns) StreamChatContinue(ctx context.Context, messages []types.EyrieMessage, opts types.ChatOptions, cfg types.ContinuationConfig) (*types.StreamResult, error) {
	select {
	case <-c.streamStarted:
	default:
		close(c.streamStarted)
	}
	c.mu.Lock()
	c.call++
	first := c.call == 1
	c.mu.Unlock()

	ch := make(chan types.EyrieStreamEvent, 4)
	ch <- types.EyrieStreamEvent{Type: "content", Content: "thinking..."}
	if first {
		go func() {
			<-ctx.Done()
			close(ch)
		}()
	} else {
		close(ch)
	}
	return &types.StreamResult{Events: ch}, nil
}

func newTestSession(t *testing.T, client engine.ChatClient) *engine.Session {
	t.Helper()
	sess := engine.NewSession("test-provider", "test-model", "system", tool.NewRegistry())
	sess.SetTestClient(client)
	return sess
}

// drainEvents reads events until the terminal EventDone (or timeout).
func drainEvents(t *testing.T, ch <-chan *Event) []*Event {
	t.Helper()
	var out []*Event
	timeout := time.After(10 * time.Second)
	for {
		select {
		case evt := <-ch:
			out = append(out, evt)
			if evt.Type == EventDone {
				return out
			}
		case <-timeout:
			t.Fatalf("timed out waiting for EventDone; got %d events", len(out))
		}
	}
}

func TestEngineTurnEmitsTerminalDone(t *testing.T) {
	client := newFakeClient(
		types.EyrieStreamEvent{Type: "content", Content: "hello"},
		types.EyrieStreamEvent{Type: "usage", Usage: &types.EyrieUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}},
		types.EyrieStreamEvent{Type: "done", StopReason: "end_turn"},
	)
	e := New(newTestSession(t, client))
	e.Start(context.Background())
	defer e.Stop()

	sub := e.Submit("hi")
	evts := drainEvents(t, e.Events())

	// The session consumes the client's "done" and emits its own; the engine
	// then appends a terminal EventDone: content, usage, done.
	if got, want := len(evts), 3; got != want {
		t.Fatalf("expected %d events (content, usage, done), got %d: %+v", want, got, evts)
	}
	if evts[0].Type != EventToken || evts[0].Content != "hello" {
		t.Errorf("first event = %+v, want token event", evts[0])
	}
	if evts[1].Type != EventUsage || evts[1].Usage == nil {
		t.Errorf("second event = %+v, want usage event", evts[1])
	} else if evts[1].Usage.TotalTokens != 15 {
		t.Errorf("usage = %+v, want total 15", evts[1].Usage)
	}
	if evts[len(evts)-1].Type != EventDone {
		t.Errorf("last event = %+v, want EventDone", evts[len(evts)-1])
	}
	if sub.ID == "" {
		t.Error("submission ID must be set")
	}
}

func TestEngineStopThenStartProcessesAgain(t *testing.T) {
	client := newFakeClient(types.EyrieStreamEvent{Type: "done", StopReason: "end_turn"})
	e := New(newTestSession(t, client))

	e.Start(context.Background())
	e.Submit("first")
	drainEvents(t, e.Events())
	e.Stop()

	// After Stop, no new events are processed.
	ch := e.Events()
	e.Submit("stale")
	select {
	case evt := <-ch:
		t.Fatalf("got event %+v after Stop", evt)
	case <-time.After(300 * time.Millisecond):
	}

	// Restart must spawn a fresh loop and process new submissions.
	e.Start(context.Background())
	defer e.Stop()
	e.Submit("second")
	evts := drainEvents(t, e.Events())
	if evts[len(evts)-1].Type != EventDone {
		t.Errorf("restarted engine did not process submission")
	}
}

func TestEngineCancelAbortsTurnAndKeepsRunning(t *testing.T) {
	c := &cancelTurns{streamStarted: make(chan struct{})}
	e := New(newTestSession(t, c))
	e.Start(context.Background())
	defer e.Stop()

	ch := e.Events()
	e.Submit("long turn")

	// Wait until the turn is streaming, then cancel it.
	select {
	case <-c.streamStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("stream never started")
	}
	e.Cancel()

	// The canceled turn must still terminate with EventDone.
	first := drainEvents(t, ch)
	if first[len(first)-1].Type != EventDone {
		t.Errorf("canceled turn did not emit EventDone")
	}

	// The engine must still process the next turn.
	e.Submit("after cancel")
	second := drainEvents(t, ch)
	if second[len(second)-1].Type != EventDone {
		t.Errorf("turn after cancel did not complete")
	}
}

func TestEngineStreamErrorEmitsErrorThenDone(t *testing.T) {
	client := newFakeClient(
		types.EyrieStreamEvent{Type: "error", Error: "provider exploded"},
	)
	e := New(newTestSession(t, client))
	e.Start(context.Background())
	defer e.Stop()

	e.Submit("hi")
	evts := drainEvents(t, e.Events())

	var sawErr, sawDone bool
	for _, evt := range evts {
		switch evt.Type {
		case EventError:
			sawErr = true
		case EventDone:
			sawDone = true
		}
	}
	if !sawErr {
		t.Errorf("expected EventError, got %+v", evts)
	}
	if !sawDone {
		t.Errorf("expected terminal EventDone after stream error, got %+v", evts)
	}
}

func TestEngineReplyToReceivesTerminalEvent(t *testing.T) {
	client := newFakeClient(types.EyrieStreamEvent{Type: "done", StopReason: "end_turn"})
	e := New(newTestSession(t, client))
	e.Start(context.Background())
	defer e.Stop()

	reply := make(chan *Event, 1)
	e.subQ.Submit(&Submission{
		ID:        "r1",
		Op:        OpUserTurn,
		Payload:   "direct",
		Timestamp: time.Now(),
		ReplyTo:   reply,
	})

	select {
	case evt := <-reply:
		if evt.Type != EventDone {
			t.Errorf("reply = %+v, want EventDone", evt)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for direct reply")
	}
}

func TestToAsyncEventMapsAllEngineTypes(t *testing.T) {
	usage := &engine.StreamUsage{
		PromptTokens:     10,
		CompletionTokens: 5,
		CacheReadTokens:  3,
		CacheWriteTokens: 2,
		Provider:         "p",
		Model:            "m",
	}

	cases := []struct {
		in  engine.StreamEvent
		typ EventType
	}{
		{engine.StreamEvent{Type: "content", Content: "x"}, EventToken},
		{engine.StreamEvent{Type: "tool_use", ToolName: "read", ToolID: "t1"}, EventToolCall},
		{engine.StreamEvent{Type: "tool_result", ToolName: "read", Content: "out"}, EventToolResult},
		{engine.StreamEvent{Type: "error", Content: "boom"}, EventError},
		{engine.StreamEvent{Type: "usage", Usage: usage}, EventUsage},
		{engine.StreamEvent{Type: "thinking", Content: "hmm"}, EventThinking},
		{engine.StreamEvent{Type: "done"}, EventDone},
		{engine.StreamEvent{Type: "compact_start", Content: "compacting"}, EventInfo},
	}

	for _, tc := range cases {
		got := toAsyncEvent(tc.in)
		if got.Type != tc.typ {
			t.Errorf("toAsyncEvent(%q).Type = %q, want %q", tc.in.Type, got.Type, tc.typ)
		}
		if got.ID == "" {
			t.Errorf("toAsyncEvent(%q) has empty ID", tc.in.Type)
		}
		if tc.in.Type == "compact_start" && got.SourceType != "compact_start" {
			t.Errorf("unmapped event must preserve SourceType, got %q", got.SourceType)
		}
	}
}

func TestEventQueueUnsubscribe(t *testing.T) {
	eq := NewEventQueue(100)
	ch := eq.Subscribe()
	eq.Unsubscribe(ch)

	eq.Push(&Event{ID: "e1", Type: EventDone})
	select {
	case evt := <-ch:
		t.Fatalf("unsubscribed channel received %+v", evt)
	default:
	}
	// Unsubscribing a channel that is already gone must be a no-op.
	eq.Unsubscribe(ch)
}

func TestEventQueueReplayIsBounded(t *testing.T) {
	eq := NewEventQueue(3)
	for i := 0; i < 10; i++ {
		eq.Push(&Event{ID: string(rune('a' + i))})
	}
	got := eq.Replay()
	if len(got) != 3 {
		t.Fatalf("replay length = %d, want 3 (bounded)", len(got))
	}
	if got[0].ID != "h" {
		t.Errorf("oldest retained event = %q, want newest events only", got[0].ID)
	}
}

func TestSubmissionQueueDrainsAllAfterSingleNotify(t *testing.T) {
	sq := NewSubmissionQueue()
	for i := 0; i < 5; i++ {
		sq.Submit(&Submission{ID: string(rune('a' + i))})
	}
	if sq.Len() != 5 {
		t.Fatalf("Len = %d, want 5", sq.Len())
	}
	var got []string
	for {
		s, ok := sq.pop()
		if !ok {
			break
		}
		got = append(got, s.ID)
	}
	if len(got) != 5 {
		t.Fatalf("popped %d, want 5", len(got))
	}
	// The leftover notify token must not poison the next submit/pop cycle.
	sq.Submit(&Submission{ID: "zz"})
	s, ok := sq.pop()
	if !ok || s.ID != "zz" {
		t.Errorf("pop after second submit = %+v, %v; want zz", s, ok)
	}
}
