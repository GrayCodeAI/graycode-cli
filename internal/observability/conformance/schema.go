// Package conformance verifies that emitted telemetry spans always match the
// documented OpenTelemetry schema (docs/OTEL-CONVENTIONS.md and the eyrie
// gen_ai.* semantic-convention constants), so the schema cannot silently drift
// across Graycode and its independent ecosystem repositories.
//
// The schema is declarative and typed per span: a span name (or prefix
// pattern), the required and optional attribute keys, and whether the span is
// forbidden from carrying raw prompt/response content. A Harness runs a set of
// span producers through a tracer and validates every recorded span against the
// schema. Validation is passive and non-throwing: a malformed or unknown span
// produces a finding, never a panic, so it can be used in CI to gate drift.
package conformance

import (
	"strings"
)

// Attribute vocabulary shared across the ecosystem. These mirror the keys
// emitted by the span starters in internal/observability/oteltrace and the
// gen_ai.* constants in eyrie/internal/observability.
const (
	AttrGenAISystem            = "gen_ai.system"
	AttrGenAIRequestModel      = "gen_ai.request.model"
	AttrGenAIResponseModel     = "gen_ai.response.model"
	AttrGenAIUsageInputTokens  = "gen_ai.usage.input_tokens"  // #nosec G101 -- OTel semconv attribute key string, not a secret value
	AttrGenAIUsageOutputTokens = "gen_ai.usage.output_tokens" // #nosec G101 -- OTel semconv attribute key string, not a secret value
	AttrGenAIOperationName     = "gen_ai.operation.name"
	AttrCostUSD                = "cost.usd"
	AttrToolName               = "tool.name"
	AttrSessionID              = "session.id"
	AttrAgentID                = "agent.id"
)

// sensitiveKeySubstrings identify attribute keys that may carry raw content
// (prompt/response/chat text) and are therefore forbidden by the no-raw-content
// rule unless explicitly allow-listed. The documented contract says spans MUST
// NOT carry raw prompt/response text.
var sensitiveKeySubstrings = []string{"prompt", "response", "content", "text"}

// SpanDef describes one span (or a family matched by NamePattern) in the schema.
type SpanDef struct {
	// NamePattern is the exact span name or a "*"-suffixed prefix pattern
	// (e.g. "tool.*" matches "tool.Read"). If it does not end in "*", it is an
	// exact match.
	NamePattern string
	// RequiredAttrs lists attribute keys that must be present.
	RequiredAttrs []string
	// OptionalAttrs lists attribute keys that may be present but are not required.
	OptionalAttrs []string
}

// Schema is the ordered set of span definitions.
type Schema []SpanDef

// GraycodeSchema is the schema covering every span graycode emits via the starters in
// internal/observability/oteltrace/spans.go.
var GraycodeSchema = Schema{
	{NamePattern: "agent_loop", RequiredAttrs: []string{"provider", "model", "message_count"}},
	{NamePattern: "tool.*", RequiredAttrs: []string{"tool.name", "tool.id"}},
	{NamePattern: "compact.*", RequiredAttrs: []string{"compact.strategy", "compact.tokens_before"}},
	{NamePattern: "api.chat", RequiredAttrs: []string{"api.provider", "api.model"}},
	{NamePattern: "session", RequiredAttrs: []string{"session.id"}},
}

// Finding describes a single conformance violation.
type Finding struct {
	SpanName string `json:"span_name"`
	Message  string `json:"message"`
}

// Find returns the SpanDef whose pattern matches spanName, or nil.
func (s Schema) Find(spanName string) *SpanDef {
	for i := range s {
		if matches(s[i].NamePattern, spanName) {
			return &s[i]
		}
	}
	return nil
}

// Validate checks a single span (name + attributes) against the schema. It
// returns every violation found. It never panics.
func (s Schema) Validate(spanName string, attributes map[string]string) []Finding {
	var findings []Finding
	def := s.Find(spanName)
	if def == nil {
		return append(findings, Finding{SpanName: spanName, Message: "span is not covered by the telemetry schema"})
	}
	for _, key := range def.RequiredAttrs {
		if v, ok := attributes[key]; !ok || strings.TrimSpace(v) == "" {
			findings = append(findings, Finding{SpanName: spanName, Message: "missing required attribute " + key})
		}
	}
	for key := range attributes {
		if !isAllowedKey(def, key) {
			findings = append(findings, Finding{SpanName: spanName, Message: "attribute " + key + " is not declared for this span"})
		}
		if isSensitiveKey(key) {
			findings = append(findings, Finding{SpanName: spanName, Message: "span carries sensitive content-bearing attribute " + key})
		}
	}
	return findings
}

// allowed reports whether key is a declared required or optional attribute.
func isAllowedKey(def *SpanDef, key string) bool {
	for _, r := range def.RequiredAttrs {
		if r == key {
			return true
		}
	}
	for _, o := range def.OptionalAttrs {
		if o == key {
			return true
		}
	}
	// Standard span attributes such as error/error.message are permitted.
	switch key {
	case "error", "error.message":
		return true
	}
	return false
}

// isSensitiveKey reports whether an attribute key may carry raw content.
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	// Counters and usage markers are numeric, not content.
	if strings.HasSuffix(lower, "_count") ||
		strings.HasSuffix(lower, "_tokens") ||
		strings.HasSuffix(lower, "_usage") ||
		lower == "message_count" {
		return false
	}
	for _, s := range sensitiveKeySubstrings {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

func matches(pattern, name string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == name
}
