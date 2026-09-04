// Package trace provides distributed tracing support.
// This is a lightweight stub for future OpenTelemetry integration.
package oteltrace

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	oteltraceapi "go.opentelemetry.io/otel/trace"
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
	// otelSpan is the underlying OTel span, set when telemetry is enabled.
	// It is nil when OTel is not initialized (in-memory tracing only).
	otelSpan oteltraceapi.Span `json:"-"`
}

// SpanEvent represents an event within a span.
type SpanEvent struct {
	Name      string            `json:"name"`
	Timestamp time.Time         `json:"timestamp"`
	Tags      map[string]string `json:"tags,omitempty"`
}

// maxRecordedSpans bounds the in-memory span buffer (M9): long-lived tracers
// (daemon lifetime) must not accumulate spans without limit. When the buffer
// is full, new spans are still created and returned (so callers and child
// spans keep working) but they are not retained.
const maxRecordedSpans = 10000

// Tracer is a simple tracer that also delegates to the global OpenTelemetry
// tracer provider when one is installed (via InitTelemetry → InitOTelSDK).
// This lets existing engine code that calls Tracer.StartSpan produce real
// distributed traces without any callsite changes.
type Tracer struct {
	mu     sync.RWMutex
	spans  []*Span
	enable bool
}

// NewTracer creates a new tracer.
func NewTracer() *Tracer {
	return &Tracer{enable: true}
}

// StartSpan starts a new span. When the global OTel tracer provider is
// configured (telemetry enabled), the span is also created as a real OTel
// span and ended when Finish() is called, so traces are exported to the
// configured OTLP backend. The in-memory span is always returned for
// backwards-compatible in-session inspection.
func (t *Tracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	span := &Span{
		Name:      name,
		TraceID:   generateID(),
		SpanID:    generateID(),
		StartTime: time.Now(),
		Tags:      make(map[string]string),
	}

	// If the global OTel tracer provider is set, create a real span.
	// otel.Tracer uses the global provider set by InitOTelSDK.
	ctx, otelSpan := otel.Tracer("graycode").Start(ctx, name)
	span.otelSpan = otelSpan

	t.mu.Lock()
	// Disable() must stop recording (M9): previously only the flag flipped
	// while StartSpan kept appending regardless.
	if t.enable && len(t.spans) < maxRecordedSpans {
		t.spans = append(t.spans, span)
	}
	t.mu.Unlock()

	return context.WithValue(ctx, spanKey, span), span
}

// Finish finishes a span, recording the end time and ending the underlying
// OTel span if one was created.
func (s *Span) Finish() {
	s.EndTime = time.Now()
	if s.otelSpan != nil {
		s.otelSpan.End()
	}
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
	id := atomic.AddInt64(&idCounter, 1)
	return fmt.Sprintf("trace-%d-%d", time.Now().UnixNano(), id)
}
