// Package trace provides distributed tracing support.
// This is a lightweight stub for future OpenTelemetry integration.
package trace

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Span represents a trace span.
type Span struct {
	Name      string            `json:"name"`
	TraceID   string            `json:"trace_id"`
	SpanID    string            `json:"span_id"`
	ParentID  string            `json:"parent_id,omitempty"`
	StartTime time.Time         `json:"start_time"`
	EndTime   time.Time         `json:"end_time,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
	Events    []SpanEvent       `json:"events,omitempty"`
}

// SpanEvent represents an event within a span.
type SpanEvent struct {
	Name      string            `json:"name"`
	Timestamp time.Time         `json:"timestamp"`
	Tags      map[string]string `json:"tags,omitempty"`
}

// Tracer is a simple tracer.
type Tracer struct {
	mu     sync.RWMutex
	spans  []*Span
	enable bool
}

// NewTracer creates a new tracer.
func NewTracer() *Tracer {
	return &Tracer{enable: true}
}

// StartSpan starts a new span.
func (t *Tracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	span := &Span{
		Name:      name,
		TraceID:   generateID(),
		SpanID:    generateID(),
		StartTime: time.Now(),
		Tags:      make(map[string]string),
	}

	t.mu.Lock()
	t.spans = append(t.spans, span)
	t.mu.Unlock()

	return context.WithValue(ctx, spanKey, span), span
}

// Finish finishes a span.
func (s *Span) Finish() {
	s.EndTime = time.Now()
}

// AddEvent adds an event to the span.
func (s *Span) AddEvent(name string, tags map[string]string) {
	s.Events = append(s.Events, SpanEvent{
		Name:      name,
		Timestamp: time.Now(),
		Tags:      tags,
	})
}

// SetTag sets a tag on the span.
func (s *Span) SetTag(key, value string) {
	s.Tags[key] = value
}

// Duration returns the span duration.
func (s *Span) Duration() time.Duration {
	if s.EndTime.IsZero() {
		return time.Since(s.StartTime)
	}
	return s.EndTime.Sub(s.StartTime)
}

// Spans returns all recorded spans.
func (t *Tracer) Spans() []*Span {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]*Span, len(t.spans))
	copy(out, t.spans)
	return out
}

// Clear clears all spans.
func (t *Tracer) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.spans = nil
}

// Enable enables tracing.
func (t *Tracer) Enable() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.enable = true
}

// Disable disables tracing.
func (t *Tracer) Disable() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.enable = false
}

// IsEnabled returns whether tracing is enabled.
func (t *Tracer) IsEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.enable
}

type spanKeyType struct{}

var spanKey = spanKeyType{}

// SpanFromContext retrieves a span from context.
func SpanFromContext(ctx context.Context) (*Span, bool) {
	s, ok := ctx.Value(spanKey).(*Span)
	return s, ok
}

var idCounter int64

func generateID() string {
	idCounter++
	return fmt.Sprintf("trace-%d-%d", time.Now().UnixNano(), idCounter)
}
