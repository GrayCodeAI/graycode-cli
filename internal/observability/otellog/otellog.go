// Package otellog is a Go-native port of DSH's OTLP log-record export seam
// (`session/session-telemetry-otel`, dsh-v0.1.0-rc.7). It owns the logical
// telemetry record vocabulary (ledger vs ops channel, three-level severity,
// minimal identity attributes, JSON-safe body) and a backend that composes the
// official OpenTelemetry SDK as-is — a LoggerProvider with a batch processor
// and an OTLP/HTTP log exporter — while this package owns the sharing policy
// (mode), config validation, and an outer shutdown deadline.
//
// The capture side (DSH's SessionTelemetryCoordinator, wired to a live session
// bus) is intentionally out of scope for this port: graycode has no Cordis bus.
// Backends receive records from the host via Emit / EmitFeedback.
package otellog

import (
	"context"
	"time"
)

// Channel is the record channel: ledger (session-log mirror) or ops
// (operational signal with no log home). Backends keep the two under separate
// instrumentation scopes (DSH `SessionTelemetryRecord.channel`).
type Channel string

const (
	ChannelLedger Channel = "ledger"
	ChannelOps    Channel = "ops"
)

// Severity is the pre-mapped three-level alerting vocabulary (DSH
// `SessionTelemetrySeverity`).
type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// Record is one logical telemetry record handed to a backend — the capture
// contract's whole outbound vocabulary (DSH `SessionTelemetryRecord`). Body is
// the complete JSON-safe payload and is never mutated after handoff.
type Record struct {
	// Channel names the instrumentation scope: ledger or ops.
	Channel Channel
	// Time is the source event's append time for ledger records, the
	// emission time for ops records (Unix epoch precision).
	Time time.Time
	// Severity is the pre-mapped alerting severity.
	Severity Severity
	// Attributes are minimal identity attributes; values are string|number.
	Attributes map[string]any
	// Body is the complete JSON-serializable payload.
	Body any
}

// Sink is the minimum backend contract (DSH `SessionTelemetrySink`). Emit
// MUST be a non-blocking enqueue — callers invoke it synchronously from hot
// paths, so anything slower than a queue push would tax the loop. Errors
// thrown by the backend are contained by the caller; they never reach the
// loop.
type Sink interface {
	// Emit hands one record to the backend's pipeline.
	Emit(record Record)
	// Flush is an optional hint that a turn ended; a backend may forward it
	// to its SDK's flush. Callers invoke it fire-and-forget.
	Flush(ctx context.Context) error
	// Shutdown drains and quiesces the pipeline, bounded by the configured
	// deadline.
	Shutdown(ctx context.Context) error
}

// Mode is the session-sharing policy (DSH `SessionTelemetryMode`).
type Mode string

const (
	// ModeFull enables direct emission and feedback capture.
	ModeFull Mode = "FULL"
	// ModeFeedbackOnly drops direct emission; only canonical feedback
	// records are captured.
	ModeFeedbackOnly Mode = "FEEDBACK_ONLY"
	// ModeDisabled constructs no SDK state and drops every record.
	ModeDisabled Mode = "DISABLED"
)

// DefaultMode is the default session-sharing policy: local-only.
const DefaultMode = ModeDisabled

// SharingStatus is the backend-independent sharing vocabulary (DSH
// `SessionTelemetrySharingStatus`).
type SharingStatus string

const (
	SharingFull         SharingStatus = "full"
	SharingFeedbackOnly SharingStatus = "feedback-only"
	SharingDisabled     SharingStatus = "disabled"
)

// DefaultShutdownTimeout is the outer allowance for the SDK's complete
// shutdown sequence (DSH `DEFAULT_SHUTDOWN_TIMEOUT_MILLIS` = 3000).
const DefaultShutdownTimeout = 3 * time.Second

// Config mirrors DSH's backend config: one sharing policy, the full OTLP
// logs endpoint, transport headers, and the DSH-owned shutdown bound.
// Uploading modes validate the endpoint and shutdown deadline at load;
// DISABLED reads neither.
type Config struct {
	// Mode is the sharing policy; defaults to local-only DISABLED behavior.
	Mode Mode
	// URL is the full OTLP logs endpoint
	// (e.g. https://collector.example.com/v1/logs). Required outside
	// DISABLED; validated at load.
	URL string
	// Headers are passed verbatim to the OTLP/HTTP log exporter.
	Headers map[string]string
	// ShutdownTimeout bounds the provider's complete shutdown path; default
	// DefaultShutdownTimeout. Must be positive.
	ShutdownTimeout time.Duration
	// ServiceName/ServiceVersion travel in the export Resource
	// (service.name/service.version).
	ServiceName    string
	ServiceVersion string
	// MaxExportBatchSize, when positive, sets the batch processor's
	// maxExportBatchSize (DSH validates the SDK's positive-integer
	// invariant at load).
	MaxExportBatchSize int
}
