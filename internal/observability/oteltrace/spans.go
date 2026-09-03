package oteltrace

import (
	"context"
	"sync"

	"github.com/GrayCodeAI/graycode-cli/internal/engine/safety"
)

// StartAgentLoopSpan creates a span for the agent loop iteration.
func StartAgentLoopSpan(ctx context.Context, t *Tracer, provider, model string, messageCount int) (context.Context, *Span) {
	ctx, span := t.StartSpan(ctx, "agent_loop")
	span.SetTag("provider", provider)
	span.SetTag("model", model)
	span.SetTag("message_count", itoa(messageCount))
	return ctx, span
}

// StartToolSpan creates a span for a tool execution.
func StartToolSpan(ctx context.Context, t *Tracer, toolName, toolID string) (context.Context, *Span) {
	ctx, span := t.StartSpan(ctx, "tool."+toolName)
	span.SetTag("tool.name", toolName)
	span.SetTag("tool.id", toolID)
	return ctx, span
}

// StartCompactSpan creates a span for a compaction operation.
func StartCompactSpan(ctx context.Context, t *Tracer, strategy string, tokensBefore int) (context.Context, *Span) {
	ctx, span := t.StartSpan(ctx, "compact."+strategy)
	span.SetTag("compact.strategy", strategy)
	span.SetTag("compact.tokens_before", itoa(tokensBefore))
	return ctx, span
}

// StartAPICallSpan creates a span for an LLM API call.
func StartAPICallSpan(ctx context.Context, t *Tracer, provider, model string) (context.Context, *Span) {
	ctx, span := t.StartSpan(ctx, "api.chat")
	span.SetTag("api.provider", provider)
	span.SetTag("api.model", model)
	return ctx, span
}

// StartSessionSpan creates a span for a full session.
func StartSessionSpan(ctx context.Context, t *Tracer, sessionID string) (context.Context, *Span) {
	ctx, span := t.StartSpan(ctx, "session")
	span.SetTag("session.id", sessionID)
	return ctx, span
}

// EndSpanWithError finishes a span and marks it as errored if err is non-nil.
// The error message is redacted so secrets/PII that leak into an error string
// are not exported in span attributes (H15).
func EndSpanWithError(span *Span, err error) {
	if err != nil {
		span.SetTag("error", "true")
		span.SetTag("error.message", redactErrorMessage(err))
	}
	span.Finish()
}

var (
	redactorOnce sync.Once
	redactor     *safety.OutputRedactor
)

// redactErrorMessage applies the shared secret-redaction patterns to an
// error message before it is stored in a span tag.
func redactErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	redactorOnce.Do(func() {
		redactor = safety.NewOutputRedactor()
	})
	if redactor != nil {
		msg = redactor.Redact(msg)
	}
	return msg
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	if neg {
		result = "-" + result
	}
	return result
}
