package otellog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/GrayCodeAI/hawk/internal/identity"
)

// maxTimerDelayMillis is a runtime protocol limit for timer delays (Node's
// MAX_TIMER_DELAY_MILLIS); kept for DSH parity of the validation bound.
const maxTimerDelayMillis = int64(2_147_483_647)

// severityFor maps the three-level vocabulary onto OTel severity numbers and
// texts (DSH `SEVERITY`).
func severityFor(s Severity) (log.Severity, string) {
	switch s {
	case SeverityInfo:
		return log.SeverityInfo, "INFO"
	case SeverityWarn:
		return log.SeverityWarn, "WARN"
	case SeverityError:
		return log.SeverityError, "ERROR"
	default:
		return log.SeverityInfo, "INFO"
	}
}

// resolveMode resolves the default and rejects unknown runtime values before
// transport setup (DSH `resolveMode`; fail closed on direct construction).
func resolveMode(mode Mode) (Mode, error) {
	resolved := mode
	if resolved == "" {
		resolved = DefaultMode
	}
	switch resolved {
	case ModeFull, ModeFeedbackOnly, ModeDisabled:
		return resolved, nil
	default:
		return "", fmt.Errorf("otellog: unsupported mode %q", string(mode))
	}
}

// sharingStatusFor maps a resolved mode onto the sharing vocabulary.
func sharingStatusFor(mode Mode) SharingStatus {
	switch mode {
	case ModeFull:
		return SharingFull
	case ModeFeedbackOnly:
		return SharingFeedbackOnly
	default:
		return SharingDisabled
	}
}

// providerBuilder constructs the SDK pipeline for a validated config. It is a
// variable so tests can substitute an in-memory pipeline.
var providerBuilder = func(ctx context.Context, cfg Config, res *resource.Resource) (*sdklog.LoggerProvider, error) {
	opts := []otlploghttp.Option{otlploghttp.WithEndpointURL(cfg.URL)}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlploghttp.WithHeaders(cfg.Headers))
	}
	exporter, err := otlploghttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("otellog: exporter: %w", err)
	}
	batchOpts := []sdklog.BatchProcessorOption{}
	if cfg.MaxExportBatchSize > 0 {
		batchOpts = append(batchOpts, sdklog.WithExportMaxBatchSize(cfg.MaxExportBatchSize))
	}
	return sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter, batchOpts...)),
	), nil
}

// Backend is the OTLP log-record backend (DSH `OpenTelemetrySessionBackend`).
// Uploading modes wire the SDK pipeline; DISABLED constructs no SDK state and
// drops every record. All emission is a non-blocking enqueue into the SDK's
// batch processor.
type Backend struct {
	mu              sync.Mutex
	mode            Mode
	sharing         SharingStatus
	provider        *sdklog.LoggerProvider
	ledger          log.Logger
	ops             log.Logger
	shutdownTimeout time.Duration
	directEmit      func(Record)
	feedbackEmit    func(Record)
	closed          bool
}

// NewBackend validates the config and constructs the backend. Uploading modes
// require a valid http(s) endpoint URL and a positive shutdown deadline;
// misconfiguration fails at load (DSH's load-time validation), never at emit.
func NewBackend(cfg Config) (*Backend, error) {
	mode, err := resolveMode(cfg.Mode)
	if err != nil {
		return nil, err
	}
	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = DefaultShutdownTimeout
	}
	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = "hawk"
	}
	serviceVersion := cfg.ServiceVersion
	if serviceVersion == "" {
		serviceVersion = "0.1.0"
	}

	if mode == ModeDisabled {
		return &Backend{
			mode:            mode,
			sharing:         sharingStatusFor(mode),
			shutdownTimeout: shutdownTimeout,
		}, nil
	}

	if cfg.URL == "" {
		return nil, fmt.Errorf("otellog: exporter URL is required (the full OTLP logs endpoint) outside DISABLED")
	}
	parsed, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("otellog: exporter URL is not a valid URL: %q", cfg.URL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("otellog: exporter URL must be http(s), got %q", parsed.Scheme)
	}
	// The one processor field checked beyond the SDK's own validation: the
	// SDK accepts a non-positive batch size, but its shutdown drain then
	// splices empty batches without consuming the queue — dispose would hang
	// forever with records queued. Misconfiguration fails at load instead
	// (DSH invariant).
	if cfg.MaxExportBatchSize < 0 {
		return nil, fmt.Errorf("otellog: MaxExportBatchSize must be a positive integer, got %d", cfg.MaxExportBatchSize)
	}
	ms := shutdownTimeout.Milliseconds()
	if shutdownTimeout <= 0 || ms > maxTimerDelayMillis {
		return nil, fmt.Errorf("otellog: shutdownTimeout must be positive and no greater than %dms, got %s", maxTimerDelayMillis, shutdownTimeout)
	}

	res := buildResource(serviceName, serviceVersion)
	provider, err := providerBuilder(context.Background(), cfg, res)
	if err != nil {
		return nil, err
	}

	const scope = "github.com/GrayCodeAI/hawk/internal/observability/otellog"
	enqueue := func(record Record) {
		logger := provider.Logger(scope)
		if record.Channel == ChannelOps {
			logger = provider.Logger(scope + "/ops")
		}
		var r log.Record
		r.SetTimestamp(record.Time)
		r.SetObservedTimestamp(record.Time)
		sev, text := severityFor(record.Severity)
		r.SetSeverity(sev)
		r.SetSeverityText(text)
		r.SetBody(anyToValue(record.Body))
		r.AddAttributes(attrsToKeyValues(record.Attributes)...)
		logger.Emit(context.Background(), r)
	}

	b := &Backend{
		mode:            mode,
		sharing:         sharingStatusFor(mode),
		provider:        provider,
		ledger:          provider.Logger(scope),
		ops:             provider.Logger(scope + "/ops"),
		shutdownTimeout: shutdownTimeout,
	}
	if mode == ModeFull {
		b.directEmit = enqueue
		b.feedbackEmit = enqueue
	} else { // ModeFeedbackOnly
		b.directEmit = nil // direct calls are no-ops; feedback replay uses the private capability
		b.feedbackEmit = enqueue
	}
	return b, nil
}

// buildResource carries app identity once per export batch on the Resource
// rather than per record: the collector aggregates by Resource, and the
// anonymous user id is process-stable anyway (DSH parity).
func buildResource(serviceName, serviceVersion string) *resource.Resource {
	attrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(serviceName),
		semconv.ServiceVersionKey.String(serviceVersion),
	}
	// user.id follows OTel semconv's standard user attribute. Fail-soft:
	// an identity resolution failure must not block telemetry.
	if ident, err := identity.Resolve(); err == nil {
		if uid, err := ident.ID(); err == nil {
			attrs = append(attrs, attribute.String("user.id", uid))
		}
	}
	return resource.NewWithAttributes(semconv.SchemaURL, attrs...)
}

// Mode returns the resolved sharing policy.
func (b *Backend) Mode() Mode { return b.mode }

// Sharing returns the backend-independent sharing status.
func (b *Backend) Sharing() SharingStatus { return b.sharing }

// Emit hands a direct service record to the SDK only in FULL mode. Direct
// calls are no-ops in FEEDBACK_ONLY and DISABLED.
func (b *Backend) Emit(record Record) {
	if b.directEmit != nil {
		b.directEmit(record)
	}
}

// EmitFeedback hands a canonical feedback record to the SDK in FULL and
// FEEDBACK_ONLY modes (the consent-gated capture path); a no-op in DISABLED.
func (b *Backend) EmitFeedback(record Record) {
	if b.feedbackEmit != nil {
		b.feedbackEmit(record)
	}
}

// Flush forwards the turn-end hint to the SDK's force-flush so records are
// exported promptly. It is fire-and-forget by contract.
func (b *Backend) Flush(ctx context.Context) error {
	if b.provider == nil {
		return nil
	}
	return b.provider.ForceFlush(ctx)
}

// Shutdown asks the SDK to drain and quiesce, but rejects after the
// backend-owned deadline. The OTel processor export timeout wraps
// exportCompleted only; shutdown awaits the exporter's force-flush first,
// which can remain pending when the transport never obtains a socket. The
// provider shutdown goroutine remains observed after the deadline so a later
// error cannot become an unhandled problem. DISABLED has no provider and
// resolves immediately (DSH shutdown semantics).
func (b *Backend) Shutdown(ctx context.Context) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	provider := b.provider
	timeout := b.shutdownTimeout
	b.mu.Unlock()

	if provider == nil {
		return nil
	}
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining < timeout {
			timeout = remaining
		}
	}
	done := make(chan error, 1)
	go func() { done <- provider.Shutdown(context.Background()) }()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return fmt.Errorf("otellog: provider shutdown exceeded %s", timeout)
	}
}

// anyToValue converts the JSON-safe payload subset onto an OTel log Value,
// matching the AnyValue subset the seam's contract guarantees (DSH
// `body as AnyValue`). Unsupported kinds fall back to their JSON encoding.
func anyToValue(v any) log.Value {
	switch t := v.(type) {
	case nil:
		return log.StringValue("")
	case string:
		return log.StringValue(t)
	case bool:
		return log.BoolValue(t)
	case int:
		return log.IntValue(t)
	case int64:
		return log.Int64Value(t)
	case uint64:
		return log.Int64Value(int64(t))
	case float64:
		return log.Float64Value(t)
	case []any:
		values := make([]log.Value, 0, len(t))
		for _, elem := range t {
			values = append(values, anyToValue(elem))
		}
		return log.SliceValue(values...)
	case map[string]any:
		kvs := make([]log.KeyValue, 0, len(t))
		for k, elem := range t {
			kvs = append(kvs, log.KeyValue{Key: k, Value: anyToValue(elem)})
		}
		return log.MapValue(kvs...)
	default:
		if data, err := json.Marshal(v); err == nil {
			return log.StringValue(string(data))
		}
		return log.StringValue(fmt.Sprintf("%v", v))
	}
}

// attrsToKeyValues maps the string|number attribute contract onto OTel
// attributes; unsupported value kinds are skipped.
func attrsToKeyValues(attrs map[string]any) []log.KeyValue {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]log.KeyValue, 0, len(attrs))
	for k, v := range attrs {
		switch t := v.(type) {
		case string:
			out = append(out, log.String(k, t))
		case int:
			out = append(out, log.Int(k, t))
		case int64:
			out = append(out, log.Int64(k, t))
		case uint64:
			out = append(out, log.Int64(k, int64(t)))
		case float64:
			out = append(out, log.Float64(k, t))
		case bool:
			out = append(out, log.Bool(k, t))
		}
	}
	return out
}

// DefaultConfig returns a config populated from environment variables,
// mirroring the oteltrace conventions: telemetry is opt-in via
// HAWK_CODE_ENABLE_TELEMETRY=1 and the logs endpoint comes from
// OTEL_EXPORTER_OTLP_LOGS_ENDPOINT. Without both, the mode stays DISABLED.
func DefaultConfig() Config {
	cfg := Config{
		Mode:            DefaultMode,
		URL:             os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"),
		Headers:         parseHeaders(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")),
		ShutdownTimeout: DefaultShutdownTimeout,
		ServiceName:     "hawk",
		ServiceVersion:  "0.1.0",
	}
	if os.Getenv("HAWK_CODE_ENABLE_TELEMETRY") == "1" && cfg.URL != "" {
		cfg.Mode = ModeFull
	}
	return cfg
}

// parseHeaders splits a comma-separated key=value header list (the
// OTEL_EXPORTER_OTLP_HEADERS convention).
func parseHeaders(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	headers := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			headers[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return headers
}
