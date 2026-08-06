package feature

// Daemon-specific feature flags. These are registered once at package init
// time and used throughout the daemon codebase to gate experimental or
// operational capabilities.
//
// Override at runtime via environment variables:
//
//	HAWK_FEATURE_SANDBOX_V2=1         — enable v2 Landlock profile
//	HAWK_FEATURE_TELEMETRY_OTEL=0     — disable OTel SDK (use in-memory tracer only)
//	HAWK_FEATURE_METRICS_ENDPOINT=0   — disable the /v1/metrics endpoint
//	HAWK_FEATURE_SECURITY_HEADERS=1   — enable security headers middleware
//	HAWK_FEATURE_CORS=0               — enable CORS support on daemon API
//	HAWK_FEATURE_AUDIT_LOG=1          — enable tamper-evident audit logging

var (
	// SandboxV2 enables the Landlock v2 sandboxing profile. This is
	// experimental and disabled by default.
	SandboxV2 = Register("sandbox-v2", false,
		"Enable Landlock v2 sandboxing profile (experimental)")

	// TelemetryOTel controls whether the full OpenTelemetry SDK is
	// initialized (with OTLP export). Enabled by default since the SDK is
	// always compiled in; set HAWK_CODE_ENABLE_TELEMETRY=1 to actually
	// activate the OTLP exporter. This flag gates whether the SDK code path
	// is exercised at all.
	TelemetryOTel = Register("telemetry-otel", true,
		"Enable OpenTelemetry SDK for distributed tracing and metrics")

	// MetricsEndpoint controls whether the GET /v1/metrics endpoint
	// is registered on the daemon. Enabled by default.
	MetricsEndpoint = Register("metrics-endpoint", true,
		"Expose GET /v1/metrics Prometheus endpoint on the daemon")

	// SecurityHeaders controls whether the security headers middleware
	// (X-Content-Type-Options, X-Frame-Options, CSP, HSTS, Referrer-Policy)
	// is applied to all daemon responses. Enabled by default.
	SecurityHeaders = Register("security-headers", true,
		"Apply security headers (CSP, HSTS, X-Frame-Options, etc.) to all responses")

	// CORS controls whether CORS support is enabled on the daemon API.
	// Disabled by default for security; enable only when serving
	// browser-based clients.
	CORS = Register("cors", false,
		"Enable CORS support on the daemon API")

	// AuditLog controls whether the tamper-evident security event log
	// is initialized and used for auth-denied and tool-execution events.
	// Enabled by default.
	AuditLog = Register("audit-log", true,
		"Enable tamper-evident security event logging")
)
