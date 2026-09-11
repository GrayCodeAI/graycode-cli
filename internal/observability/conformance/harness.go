package conformance

import (
	"context"

	"github.com/GrayCodeAI/hawk/internal/observability/oteltrace"
)

// SpanProducer starts and finishes one span on the given tracer, returning the
// finished span. It is the seam that connects a real span starter to the
// conformance harness.
type SpanProducer func(ctx context.Context, t *oteltrace.Tracer) *oteltrace.Span

// Report summarizes the conformance result for a batch of spans.
type Report struct {
	Total    int       `json:"total"`
	Violated int       `json:"violated"`
	Findings []Finding `json:"findings,omitempty"`
}

// Passed reports whether every span conformed.
func (r Report) Passed() bool { return r.Violated == 0 }

// Run executes the given producers against a fresh tracer, validates every
// recorded span against the schema, and returns a report. It is passive: a
// producer that panics is caught and reported as a finding rather than crashing
// the harness, so CI can surface drift without breaking the run.
func Run(schema Schema, producers []SpanProducer) Report {
	t := oteltrace.NewTracer()
	defer t.Clear()

	var report Report
	for _, produce := range producers {
		span := safeProduce(produce, t)
		if span == nil {
			report.Total++
			report.Violated++
			report.Findings = append(report.Findings, Finding{SpanName: "<producer>", Message: "span producer panicked"})
			continue
		}
		report.Total++
		findings := schema.Validate(span.Name, span.Tags)
		if len(findings) > 0 {
			report.Violated++
			report.Findings = append(report.Findings, findings...)
		}
	}
	return report
}

// safeProduce invokes a producer, recovering any panic so a misbehaving span
// starter cannot break the conformance run. It returns nil on panic.
func safeProduce(produce SpanProducer, t *oteltrace.Tracer) (span *oteltrace.Span) {
	defer func() {
		if r := recover(); r != nil {
			span = nil
		}
	}()
	return produce(context.Background(), t)
}
