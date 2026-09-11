package conformance

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/observability/oteltrace"
)

// hawkSpanProducers wires the real span starters from internal/observability/
// oteltrace into the conformance harness. Keeping the starters as the producers
// means the test fails if a starter stops setting a required attribute.
func hawkSpanProducers() []SpanProducer {
	return []SpanProducer{
		func(ctx context.Context, t *oteltrace.Tracer) *oteltrace.Span {
			_, span := oteltrace.StartAgentLoopSpan(ctx, t, "anthropic", "claude-sonnet-4", 3)
			span.Finish()
			return span
		},
		func(ctx context.Context, t *oteltrace.Tracer) *oteltrace.Span {
			_, span := oteltrace.StartToolSpan(ctx, t, "Read", "tool-1")
			span.Finish()
			return span
		},
		func(ctx context.Context, t *oteltrace.Tracer) *oteltrace.Span {
			_, span := oteltrace.StartCompactSpan(ctx, t, "summary", 1200)
			span.Finish()
			return span
		},
		func(ctx context.Context, t *oteltrace.Tracer) *oteltrace.Span {
			_, span := oteltrace.StartAPICallSpan(ctx, t, "openai", "gpt-4o")
			span.Finish()
			return span
		},
		func(ctx context.Context, t *oteltrace.Tracer) *oteltrace.Span {
			_, span := oteltrace.StartSessionSpan(ctx, t, "sess-123")
			span.Finish()
			return span
		},
	}
}

func TestHawkSpansConformToSchema(t *testing.T) {
	report := Run(HawkSchema, hawkSpanProducers())
	if !report.Passed() {
		for _, f := range report.Findings {
			t.Errorf("conformance finding: %s: %s", f.SpanName, f.Message)
		}
		t.Fatalf("hawk spans must conform; %d violations", report.Violated)
	}
}

// TestSchemaCatchesDrift verifies the schema itself is strict: a span missing a
// required attribute or carrying a sensitive key must fail validation. This
// guards the conformance suite against becoming a no-op.
func TestSchemaCatchesDrift(t *testing.T) {
	schema := HawkSchema

	// Missing required attribute must fail.
	if findings := schema.Validate("agent_loop", map[string]string{"provider": "anthropic"}); len(findings) == 0 {
		t.Fatal("agent_loop without model/message_count must be a violation")
	}

	// Unknown span must fail.
	if findings := schema.Validate("does_not_exist", map[string]string{}); len(findings) == 0 {
		t.Fatal("unknown span must be a violation")
	}

	// Sensitive content-bearing attribute must fail.
	if findings := schema.Validate("agent_loop", map[string]string{
		"provider": "anthropic", "model": "m", "message_count": "1", "prompt": "secret",
	}); len(findings) == 0 {
		t.Fatal("span carrying raw prompt content must be a violation")
	}

	// Valid spans pass.
	if findings := schema.Validate("tool.Read", map[string]string{"tool.name": "Read", "tool.id": "1"}); len(findings) != 0 {
		t.Fatalf("valid tool span should pass, got %+v", findings)
	}
}

// TestRunIsPassive verifies the harness never panics even when a producer panics.
func TestRunIsPassive(t *testing.T) {
	producers := []SpanProducer{
		func(context.Context, *oteltrace.Tracer) *oteltrace.Span {
			panic("boom")
		},
	}
	report := Run(HawkSchema, producers)
	if !report.Passed() {
		if !strings.Contains(report.Findings[0].Message, "panicked") {
			t.Fatalf("expected a panicked finding, got %+v", report.Findings)
		}
	}
}
