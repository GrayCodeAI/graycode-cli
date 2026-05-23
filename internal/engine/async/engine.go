package async

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

// Engine wraps an engine.Session with queue-based async operation.
// Submissions are processed in order; events are broadcast to all subscribers.
type Engine struct {
	session *engine.Session
	subQ    *SubmissionQueue
	evtQ    *EventQueue
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
}

// New creates an async engine wrapping the given session.
func New(sess *engine.Session) *Engine {
	return &Engine{
		session: sess,
		subQ:    NewSubmissionQueue(),
		evtQ:    NewEventQueue(5000),
	}
}

// Start begins processing submissions in the background.
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	ctx, e.cancel = context.WithCancel(ctx)
	e.mu.Unlock()

	go e.loop(ctx)
}

// Stop gracefully stops the engine.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		e.cancel()
	}
	e.running = false
}

// Submit enqueues a user turn for processing.
func (e *Engine) Submit(message string) *Submission {
	s := &Submission{
		ID:        uuid.New().String()[:8],
		Op:        OpUserTurn,
		Payload:   message,
		Timestamp: time.Now(),
	}
	e.subQ.Submit(s)
	return s
}

// Cancel enqueues a cancellation for the current turn.
func (e *Engine) Cancel() {
	e.subQ.Submit(&Submission{
		ID:   uuid.New().String()[:8],
		Op:   OpCancel,
	})
}

// Events returns a channel for consuming events.
func (e *Engine) Events() <-chan *Event {
	return e.evtQ.Subscribe()
}

// Replay returns all events since the engine started.
func (e *Engine) Replay() []*Event {
	return e.evtQ.Replay()
}

// Session returns the underlying engine session.
func (e *Engine) Session() *engine.Session {
	return e.session
}

func (e *Engine) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			sub := e.subQ.Next()
			if sub == nil {
				continue
			}
			e.process(ctx, sub)
		}
	}
}

func (e *Engine) process(ctx context.Context, sub *Submission) {
	switch sub.Op {
	case OpUserTurn:
		e.processTurn(ctx, sub.Payload, sub.ReplyTo)
	case OpCancel:
		e.cancel()
	case OpResume:
		// Resume will be implemented with session replay support.
	default:
		e.evtQ.Push(&Event{
			ID:    uuid.New().String()[:8],
			Type:  EventError,
			Content: fmt.Sprintf("unknown op: %s", sub.Op),
		})
	}
}

func (e *Engine) processTurn(ctx context.Context, message string, _ chan<- *Event) {
	e.session.AddUser(message)

	stream, err := e.session.Stream(ctx)
	if err != nil {
		e.evtQ.Push(&Event{
			ID:      uuid.New().String()[:8],
			Type:    EventError,
			Content: err.Error(),
		})
		return
	}

	for evt := range stream {
		e.evtQ.Push(toAsyncEvent(evt))
	}

	// Signal done.
	e.evtQ.Push(&Event{
		ID:   uuid.New().String()[:8],
		Type: EventDone,
	})
}

func toAsyncEvent(evt engine.StreamEvent) *Event {
	e := &Event{
		ID:        uuid.New().String()[:8],
		Timestamp: time.Now(),
	}
	switch evt.Type {
	case "content":
		e.Type = EventToken
		e.Content = evt.Content
	case "tool_use":
		e.Type = EventToolCall
		e.ToolName = evt.ToolName
		e.ToolID = evt.ToolID
	case "tool_result":
		e.Type = EventToolResult
		e.ToolName = evt.ToolName
		e.Content = evt.Content
	case "error":
		e.Type = EventError
		e.Content = evt.Content
	case "usage":
		e.Type = EventUsage
		if evt.Usage != nil {
			e.Usage = &Usage{
				PromptTokens:     evt.Usage.PromptTokens,
				CompletionTokens: evt.Usage.CompletionTokens,
				TotalTokens:      evt.Usage.PromptTokens + evt.Usage.CompletionTokens,
			}
		}
	case "thinking":
		e.Type = EventThinking
		e.Content = evt.Content
	case "done":
		e.Type = EventDone
	}
	return e
}
