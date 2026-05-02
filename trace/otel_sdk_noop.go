//go:build !otel

package trace

import "context"

// OTelProviders is a no-op stub when built without the otel tag.
type OTelProviders struct{}

// InitOTelSDK returns a no-op provider set.
func InitOTelSDK(cfg TelemetryConfig) (*OTelProviders, error) {
	return &OTelProviders{}, nil
}

func (p *OTelProviders) OTelTracer() interface{} { return nil }

func (p *OTelProviders) StartOTelSpan(ctx context.Context, name string, attrs ...interface{}) (context.Context, interface{}) {
	return ctx, nil
}

func (p *OTelProviders) ShutdownOTel(ctx context.Context) error { return nil }

func (p *OTelProviders) FlushOTel(ctx context.Context) error { return nil }

func (p *OTelProviders) RecordMetric(name string, value int64, attrs ...interface{}) {}
