package oteltrace

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TelemetryConfig controls OpenTelemetry initialization.
type TelemetryConfig struct {
	Enabled         bool
	ServiceName     string
	ServiceVersion  string
	ExporterProto   string
	Endpoint        string
	Headers         map[string]string
	MetricsInterval time.Duration
	TracesInterval  time.Duration
	LogsInterval    time.Duration
	ShutdownTimeout time.Duration
}

// DefaultTelemetryConfig returns a config populated from environment variables.
func DefaultTelemetryConfig() TelemetryConfig {
	cfg := TelemetryConfig{
		ServiceName:     "graycode",
		ServiceVersion:  "0.1.0",
		ExporterProto:   envOr("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf"),
		Endpoint:        os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		MetricsInterval: 60 * time.Second,
		TracesInterval:  5 * time.Second,
		LogsInterval:    5 * time.Second,
		ShutdownTimeout: 2 * time.Second,
	}

	// Telemetry is opt-in: only the explicit GRAYCODE_ENABLE_TELEMETRY=1
	// flag enables it. Setting the standard OTEL_EXPORTER_OTLP_ENDPOINT must
	// not implicitly enable graycode telemetry (Phase 3 hardening).
	cfg.Enabled = os.Getenv("GRAYCODE_ENABLE_TELEMETRY") == "1"

	if hdrs := os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"); hdrs != "" {
		cfg.Headers = parseHeaders(hdrs)
	}

	if timeout := os.Getenv("GRAYCODE_OTEL_SHUTDOWN_TIMEOUT_MS"); timeout != "" {
		if ms, err := strconv.Atoi(timeout); err == nil {
			cfg.ShutdownTimeout = time.Duration(ms) * time.Millisecond
		}
	}

	return cfg
}

// Providers holds the initialized telemetry providers.
type Providers struct {
	mu       sync.Mutex
	config   TelemetryConfig
	tracer   *Tracer
	otel     *OTelProviders
	shutdown bool
}

// InitTelemetry initializes telemetry based on configuration.
// When telemetry is enabled (cfg.Enabled), it initializes the real OpenTelemetry
// SDK with OTLP exporters. When disabled, it returns a no-op provider set.
// The in-memory Tracer is always available for lightweight in-session tracing.
func InitTelemetry(cfg TelemetryConfig) (*Providers, error) {
	p := &Providers{
		config: cfg,
		tracer: NewTracer(),
	}

	if !cfg.Enabled {
		p.tracer.Disable()
		return p, nil
	}

	// Initialize the real OTel SDK — this sets the global tracer/meter
	// providers and creates an OTLP trace exporter from the config.
	otelProviders, err := InitOTelSDK(cfg)
	if err != nil {
		// Telemetry is opt-in; a configuration error should not crash the
		// process. Log the error and fall back to the in-memory tracer only.
		p.tracer.Disable()
		p.config.Enabled = false
		return p, nil
	}
	p.otel = otelProviders

	// The in-memory Tracer delegates to the global OTel tracer provider
	// (set by InitOTelSDK via otel.SetTracerProvider) in StartSpan,
	// so existing engine code that calls Tracer.StartSpan gets real
	// distributed traces exported to the configured backend. No explicit
	// wiring is needed on the Tracer itself.
	return p, nil
}

// Tracer returns the active in-memory tracer. This is the primary tracer
// used by the engine; when OTel is enabled, spans are also exported to
// the configured OTLP backend.
func (p *Providers) Tracer() *Tracer {
	return p.tracer
}

// OTelProviders returns the underlying OTel SDK providers, or nil if
// the SDK was not initialized (telemetry disabled or failed to init).
func (p *Providers) OTelProviders() *OTelProviders {
	return p.otel
}

// Shutdown flushes and shuts down all telemetry providers.
func (p *Providers) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.shutdown {
		return nil
	}
	p.shutdown = true

	var firstErr error

	// Flush in-memory spans to OTel before shutting down the SDK.
	if p.otel != nil {
		if err := p.otel.FlushOTel(ctx); err != nil {
			firstErr = err
		}
		if err := p.otel.ShutdownOTel(ctx); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	p.tracer.Clear()
	return firstErr
}

// Flush forces export of pending telemetry data.
func (p *Providers) Flush(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.shutdown {
		return nil
	}
	var firstErr error
	if p.otel != nil {
		if err := p.otel.FlushOTel(ctx); err != nil {
			firstErr = err
		}
	}
	return firstErr
}

// IsEnabled returns whether telemetry is active.
func (p *Providers) IsEnabled() bool {
	return p.config.Enabled && !p.shutdown
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseHeaders(raw string) map[string]string {
	headers := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return headers
}
