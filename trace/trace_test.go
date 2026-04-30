package trace

import (
	"context"
	"testing"
	"time"
)

func TestNewTracer(t *testing.T) {
	tr := NewTracer()
	if !tr.IsEnabled() {
		t.Fatal("expected tracer to be enabled")
	}
}

func TestStartSpan(t *testing.T) {
	tr := NewTracer()
	ctx, span := tr.StartSpan(context.Background(), "test-operation")

	if span.Name != "test-operation" {
		t.Fatalf("expected name 'test-operation', got %q", span.Name)
	}
	if span.TraceID == "" {
		t.Fatal("expected trace ID")
	}

	// Check context
	_, ok := SpanFromContext(ctx)
	if !ok {
		t.Fatal("expected span in context")
	}
}

func TestSpanFinish(t *testing.T) {
	tr := NewTracer()
	_, span := tr.StartSpan(context.Background(), "test")

	time.Sleep(10 * time.Millisecond)
	span.Finish()

	if span.Duration() < 5*time.Millisecond {
		t.Fatalf("expected duration >= 5ms, got %v", span.Duration())
	}
}

func TestSpanTags(t *testing.T) {
	_, span := NewTracer().StartSpan(context.Background(), "test")
	span.SetTag("key", "value")

	if span.Tags["key"] != "value" {
		t.Fatalf("expected tag value 'value', got %q", span.Tags["key"])
	}
}

func TestSpanEvents(t *testing.T) {
	_, span := NewTracer().StartSpan(context.Background(), "test")
	span.AddEvent("event1", map[string]string{"key": "val"})

	if len(span.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(span.Events))
	}
	if span.Events[0].Name != "event1" {
		t.Fatalf("expected event name 'event1', got %q", span.Events[0].Name)
	}
}

func TestTracerSpans(t *testing.T) {
	tr := NewTracer()
	tr.StartSpan(context.Background(), "span1")
	tr.StartSpan(context.Background(), "span2")

	spans := tr.Spans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
}

func TestTracerClear(t *testing.T) {
	tr := NewTracer()
	tr.StartSpan(context.Background(), "test")
	tr.Clear()

	spans := tr.Spans()
	if len(spans) != 0 {
		t.Fatalf("expected 0 spans, got %d", len(spans))
	}
}

func TestTracerEnableDisable(t *testing.T) {
	tr := NewTracer()
	tr.Disable()
	if tr.IsEnabled() {
		t.Fatal("expected disabled")
	}
	tr.Enable()
	if !tr.IsEnabled() {
		t.Fatal("expected enabled")
	}
}
