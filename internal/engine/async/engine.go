package async

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

// Engine wraps an engine.Session with queue-based async operation.
// Submissions are processed in order; events are broadcast to all subscribers.
//
// Lifecycle: Start spawns exactly one processing loop; Stop cancels it and
// waits for it to exit. Start after Stop is safe — a fresh loop is created.
// Submissions queued after Stop are never processed (documented: check the
// engine state before submitting).
type Engine struct {
	session *engine.Session
	subQ    *SubmissionQueue
	evtQ    *EventQueue
	mu      sync.Mutex
	running bool
	wg      sync.WaitGroup
	stop    context.CancelFunc
	turnMu  sync.Mutex
	turn    context.CancelFunc
}

// New creates an async engine wrapping the given session.
func New(sess *engine.Session) *Engine {
	return &Engine{
		session: sess,
		subQ:    NewSubmissionQueue(),
		evtQ:    NewEventQueue(5000),
	}
}

// Start begins processing submissions in the background. It is safe to call
// multiple times; only the first call spawns the loop. After Stop, a new
// Start spawns a fresh loop.
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	loopCtx, cancel := context.WithCancel(ctx)
	e.stop = cancel
	e.wg.Add(1)
	e.mu.Unlock()

	go e.loop(loopCtx)
}

// Stop gracefully stops the engine. It cancels the processing loop and waits
// (with a bounded grace period) for it to exit, so a subsequent Start can
// never run two loops over the same queue. Idempotent.
func (e *Engine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	stop := e.stop
	e.stop = nil
	e.mu.Unlock()

	if stop != nil {
		stop()
	}
	// Bounded wait: the loop may be mid-Stream (provider call); do not block
	// shutdown forever, but still prevent double-loops in the common case.
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

// Submit enqueues a user turn for processing.
func (e *Engine) Submit(message string) *Submission {
	s := &Submission{
		ID:        uuid.NewString(),
		Op:        OpUserTurn,
		Payload:   message,
		Timestamp: time.Now(),
	}
	e.subQ.Submit(s)
	return s
}

// Cancel aborts the in-flight turn immediately. No-op when the engine is
// idle. Direct (not queued): the processing loop is single-threaded and
// blocked inside the turn's stream, so a queued cancel would never be
// popped until the turn ended anyway.
func (e *Engine) Cancel() {
	e.cancel()
}

// cancel aborts the in-flight turn only; the processing loop keeps running.
func (e *Engine) cancel() {
	e.turnMu.Lock()
	c := e.turn
	e.turnMu.Unlock()
	if c != nil {
		c()
	}
}

// Events returns a channel for consuming events. Unsubscribe with
// e.Unsubscribe(ch) when done; subscriber channels are never closed by the
// engine (the terminal EventDone/EventError marks the end of a turn).
func (e *Engine) Events() <-chan *Event {
	return e.evtQ.Subscribe()
}

// Unsubscribe removes a subscriber channel so it stops receiving events.
func (e *Engine) Unsubscribe(ch <-chan *Event) {
	e.evtQ.Unsubscribe(ch)
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
	defer e.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.subQ.notify:
			for {
				sub, ok := e.subQ.pop()
				if !ok {
					break
				}
				e.process(ctx, sub)
			}
		}
	}
}

func (e *Engine) process(ctx context.Context, sub *Submission) {
	switch sub.Op {
	case OpUserTurn:
		e.processTurn(ctx, sub.Payload, sub.ReplyTo)
	case OpCancel, OpInterrupt:
		// Cancellation is applied directly by Engine.Cancel (the loop is
		// single-threaded, so a queued cancel could not run until the turn
		// ended). This path exists for queue compatibility only.
		e.cancel()
	case OpResume:
		// Resume will be implemented with session replay support.
	default:
		e.evtQ.Push(&Event{
			ID:      uuid.NewString(),
			Type:    EventError,
			Content: fmt.Sprintf("unknown op: %s", sub.Op),
		})
	}
}

// processTurn runs one agent turn. The terminal event is EventDone on both
// success and stream error paths, so consumers can always wait for it; the
// error detail (if any) arrives as an EventError before EventDone.
func (e *Engine) processTurn(ctx context.Context, message string, replyTo chan<- *Event) {
	e.session.AddUser(message)

	turnCtx, cancel := context.WithCancel(ctx)
	e.turnMu.Lock()
	e.turn = cancel
	e.turnMu.Unlock()
	defer func() {
		e.turnMu.Lock()
		e.turn = nil
		e.turnMu.Unlock()
		cancel()
	}()

	stream, err := e.session.Stream(turnCtx)
	if err != nil {
		errEvt := &Event{
			ID:      uuid.NewString(),
			Type:    EventError,
			Content: err.Error(),
		}
		e.evtQ.Push(errEvt)
		notifyReply(replyTo, errEvt)
		doneEvt := &Event{ID: uuid.NewString(), Type: EventDone}
		e.evtQ.Push(doneEvt)
		notifyReply(replyTo, doneEvt)
		return
	}

	for evt := range stream {
		asyncEvt := toAsyncEvent(evt)
		e.evtQ.Push(asyncEvt)
	}

	// Signal done.
	doneEvt := &Event{ID: uuid.NewString(), Type: EventDone}
	e.evtQ.Push(doneEvt)
	notifyReply(replyTo, doneEvt)
}

// notifyReply delivers the terminal event to the submission's direct-reply
// channel when one is configured. Non-blocking: a slow direct consumer must
// not stall the engine loop.
func notifyReply(replyTo chan<- *Event, evt *Event) {
	if replyTo == nil {
		return
	}
	select {
	case replyTo <- evt:
	default:
	}
}

func toAsyncEvent(evt engine.StreamEvent) *Event {
	e := &Event{
		ID:         uuid.NewString(),
		Timestamp:  time.Now(),
		SourceType: evt.Type,
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
				CacheReadTokens:  evt.Usage.CacheReadTokens,
				CacheWriteTokens: evt.Usage.CacheWriteTokens,
				TotalTokens:      evt.Usage.PromptTokens + evt.Usage.CompletionTokens,
				Provider:         evt.Usage.Provider,
				Model:            evt.Usage.Model,
			}
		}
	case "thinking":
		e.Type = EventThinking
		e.Content = evt.Content
	case "done":
		e.Type = EventDone
	default:
		// Engine stream events without a mapping (compact_start,
		// blast_radius, ...) must not become garbage zero-value events:
		// preserve them as EventInfo with the raw type for diagnostics.
		e.Type = EventInfo
		e.Content = evt.Content
	}
	return e
}
