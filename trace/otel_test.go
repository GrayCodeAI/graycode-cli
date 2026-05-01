package trace

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDefaultTelemetryConfig(t *testing.T) {
	os.Setenv("HAWK_CODE_ENABLE_TELEMETRY", "1")
	defer os.Unsetenv("HAWK_CODE_ENABLE_TELEMETRY")

	cfg := DefaultTelemetryConfig()
	if !cfg.Enabled {
		t.Error("expected telemetry enabled when env var set")
	}
	if cfg.ServiceName != "hawk-code" {
		t.Errorf("expected service name hawk-code, got %s", cfg.ServiceName)
	}
	if cfg.ShutdownTimeout != 2*time.Second {
		t.Errorf("expected 2s shutdown timeout, got %v", cfg.ShutdownTimeout)
	}
}

func TestDefaultTelemetryConfig_Disabled(t *testing.T) {
	os.Unsetenv("HAWK_CODE_ENABLE_TELEMETRY")
	os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	cfg := DefaultTelemetryConfig()
	if cfg.Enabled {
		t.Error("expected telemetry disabled when no env vars set")
	}
}

func TestInitTelemetry(t *testing.T) {
	cfg := TelemetryConfig{
		Enabled:     true,
		ServiceName: "test-service",
	}

	providers, err := InitTelemetry(cfg)
	if err != nil {
		t.Fatalf("InitTelemetry error: %v", err)
	}
	if providers == nil {
		t.Fatal("expected non-nil providers")
	}
	if !providers.IsEnabled() {
		t.Error("expected providers to be enabled")
	}
	if providers.Tracer() == nil {
		t.Error("expected non-nil tracer")
	}
}

func TestProviders_Shutdown(t *testing.T) {
	cfg := TelemetryConfig{Enabled: true}
	providers, _ := InitTelemetry(cfg)

	ctx := context.Background()
	if err := providers.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown error: %v", err)
	}
	if providers.IsEnabled() {
		t.Error("expected disabled after shutdown")
	}

	// Double shutdown should be safe
	if err := providers.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown error: %v", err)
	}
}

func TestProviders_Disabled(t *testing.T) {
	cfg := TelemetryConfig{Enabled: false}
	providers, _ := InitTelemetry(cfg)

	if providers.IsEnabled() {
		t.Error("expected disabled")
	}
}

func TestStartAgentLoopSpan(t *testing.T) {
	tr := NewTracer()
	ctx := context.Background()

	ctx, span := StartAgentLoopSpan(ctx, tr, "anthropic", "claude-4", 10)
	if span == nil {
		t.Fatal("expected non-nil span")
	}
	if span.Name != "agent_loop" {
		t.Errorf("expected name agent_loop, got %s", span.Name)
	}
	if span.Tags["provider"] != "anthropic" {
		t.Errorf("expected provider=anthropic, got %s", span.Tags["provider"])
	}

	span.Finish()
	_ = ctx

	spans := tr.Spans()
	if len(spans) != 1 {
		t.Errorf("expected 1 span, got %d", len(spans))
	}
}

func TestStartToolSpan(t *testing.T) {
	tr := NewTracer()
	ctx := context.Background()

	_, span := StartToolSpan(ctx, tr, "Bash", "tool-123")
	if span.Name != "tool.Bash" {
		t.Errorf("expected name tool.Bash, got %s", span.Name)
	}
	if span.Tags["tool.name"] != "Bash" {
		t.Errorf("expected tool.name=Bash")
	}
	span.Finish()
}

func TestEndSpanWithError(t *testing.T) {
	tr := NewTracer()
	ctx := context.Background()

	_, span := tr.StartSpan(ctx, "test")
	EndSpanWithError(span, nil)
	if _, ok := span.Tags["error"]; ok {
		t.Error("should not have error tag when err is nil")
	}

	_, span2 := tr.StartSpan(ctx, "test2")
	EndSpanWithError(span2, context.DeadlineExceeded)
	if span2.Tags["error"] != "true" {
		t.Error("should have error=true tag")
	}
	if span2.Tags["error.message"] == "" {
		t.Error("should have error.message tag")
	}
}

func TestParseHeaders(t *testing.T) {
	headers := parseHeaders("Authorization=Bearer token123,X-Custom=value")
	if headers["Authorization"] != "Bearer token123" {
		t.Errorf("expected Authorization header, got %v", headers)
	}
	if headers["X-Custom"] != "value" {
		t.Errorf("expected X-Custom header, got %v", headers)
	}
}
