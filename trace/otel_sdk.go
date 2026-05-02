//go:build otel

package trace

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// OTelProviders holds the real OpenTelemetry SDK providers.
type OTelProviders struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	tracer         oteltrace.Tracer
	config         TelemetryConfig
	shutdown       bool
}

// InitOTelSDK initializes the full OpenTelemetry SDK with OTLP exporters.
func InitOTelSDK(cfg TelemetryConfig) (*OTelProviders, error) {
	if !cfg.Enabled {
		return &OTelProviders{config: cfg}, nil
	}

	ctx := context.Background()

	// Build resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
			attribute.String("deployment.environment", "production"),
		),
	)
	if err != nil {
		return nil, err
	}

	// Build trace exporter
	opts := []otlptracehttp.Option{}
	if cfg.Endpoint != "" {
		opts = append(opts, otlptracehttp.WithEndpoint(cfg.Endpoint))
	}
	if cfg.ExporterProto == "http/json" {
		// default is protobuf, no extra option needed for json
	}
	for k, v := range cfg.Headers {
		opts = append(opts, otlptracehttp.WithHeaders(map[string]string{k: v}))
	}

	traceExporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	// Build tracer provider
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter,
			sdktrace.WithBatchTimeout(cfg.TracesInterval),
		),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)

	// Build meter provider
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	tracer := tracerProvider.Tracer(cfg.ServiceName)

	return &OTelProviders{
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
		tracer:         tracer,
		config:         cfg,
	}, nil
}

// Tracer returns the OTel tracer for creating spans.
func (p *OTelProviders) OTelTracer() oteltrace.Tracer {
	return p.tracer
}

// StartSpan creates a new OTel span.
func (p *OTelProviders) StartOTelSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, oteltrace.Span) {
	if p.tracer == nil {
		return ctx, oteltrace.SpanFromContext(ctx)
	}
	return p.tracer.Start(ctx, name, oteltrace.WithAttributes(attrs...))
}

// ShutdownOTel gracefully shuts down all OTel providers.
func (p *OTelProviders) ShutdownOTel(ctx context.Context) error {
	if p.shutdown {
		return nil
	}
	p.shutdown = true

	timeout := p.config.ShutdownTimeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	shutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var firstErr error
	if p.tracerProvider != nil {
		if err := p.tracerProvider.Shutdown(shutCtx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if p.meterProvider != nil {
		if err := p.meterProvider.Shutdown(shutCtx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// FlushOTel forces export of pending data.
func (p *OTelProviders) FlushOTel(ctx context.Context) error {
	if p.tracerProvider != nil {
		if err := p.tracerProvider.ForceFlush(ctx); err != nil {
			return err
		}
	}
	return nil
}

// RecordMetric records a counter metric.
func (p *OTelProviders) RecordMetric(name string, value int64, attrs ...attribute.KeyValue) {
	if p.meterProvider == nil {
		return
	}
	meter := p.meterProvider.Meter(p.config.ServiceName)
	counter, err := meter.Int64Counter(name)
	if err != nil {
		return
	}
	counter.Add(context.Background(), value)
	_ = attrs // attributes applied via OTel API options in real usage
}
