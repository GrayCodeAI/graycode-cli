package trace

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
		ServiceName:     "hawk-code",
		ServiceVersion:  "0.1.0",
		ExporterProto:   envOr("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf"),
		Endpoint:        os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		MetricsInterval: 60 * time.Second,
		TracesInterval:  5 * time.Second,
		LogsInterval:    5 * time.Second,
		ShutdownTimeout: 2 * time.Second,
	}

	cfg.Enabled = os.Getenv("HAWK_CODE_ENABLE_TELEMETRY") == "1" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != ""

	if hdrs := os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"); hdrs != "" {
		cfg.Headers = parseHeaders(hdrs)
	}

	if timeout := os.Getenv("HAWK_CODE_OTEL_SHUTDOWN_TIMEOUT_MS"); timeout != "" {
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
	shutdown bool
}

// InitTelemetry initializes telemetry based on configuration.
// When OTel SDK is available (future), this will create real OTLP exporters.
// Currently uses the built-in Tracer as a lightweight fallback.
func InitTelemetry(cfg TelemetryConfig) (*Providers, error) {
	p := &Providers{
		config: cfg,
		tracer: NewTracer(),
	}

	if !cfg.Enabled {
		p.tracer.Disable()
	}

	return p, nil
}

// Tracer returns the active tracer.
func (p *Providers) Tracer() *Tracer {
	return p.tracer
}

// Shutdown flushes and shuts down all telemetry providers.
func (p *Providers) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.shutdown {
		return nil
	}
	p.shutdown = true

	// When OTel SDK is wired in, this will call:
	// - tracerProvider.Shutdown(ctx)
	// - meterProvider.Shutdown(ctx)
	// - loggerProvider.Shutdown(ctx)
	p.tracer.Clear()
	return nil
}

// Flush forces export of pending telemetry data.
func (p *Providers) Flush(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.shutdown {
		return nil
	}
	// When OTel SDK is wired in, this will call ForceFlush on providers
	return nil
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
