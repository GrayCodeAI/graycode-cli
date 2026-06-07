package hawkerr

import "strings"

// Exit-code taxonomy.
//
// hawk historically collapsed every failure to exit code 1, which gives an
// agent or shell script driving `hawk --print` no way to branch on *why* the
// run failed without scraping stderr. The codes below assign a stable,
// documented small-integer meaning to each failure class so a caller can, for
// example, retry on a rate-limit (5) but stop immediately on bad credentials
// (3).
//
// The numbers are part of hawk's public CLI contract — append new codes, do
// not renumber existing ones. Codes are kept below 64 to avoid colliding with
// the shell's 128+signal convention and 126/127 (not-executable / not-found).
const (
	ExitOK           = 0  // success
	ExitGeneral      = 1  // unclassified / general failure
	ExitUsage        = 2  // bad flags or arguments (reserved for the CLI layer)
	ExitAuth         = 3  // missing/invalid/expired credentials, 401/403
	ExitNetwork      = 4  // DNS, connection refused/reset, provider 5xx, TLS
	ExitRateLimit    = 5  // 429 / quota / insufficient credits
	ExitTimeout      = 6  // request timeout, deadline exceeded, cancellation
	ExitToolFailure  = 7  // a tool execution failed or timed out
	ExitPolicyBlock  = 8  // permission/guardrail/policy denial
	ExitConfig       = 9  // malformed settings/config
	ExitContextLimit = 10 // prompt exceeds the model's context window
	ExitNotFound     = 11 // model/endpoint/resource not found (404)
	ExitDiskFull     = 12 // out of disk space or quota
)

// ClassifyExitCode maps an error to a stable exit code from the taxonomy above.
//
// It deliberately mirrors the textual classification already performed by the
// CLI's friendlyError (cmd/errors.go): the same provider/network/auth signals
// that produce a friendly message here produce a stable exit code, so the two
// never disagree about what kind of failure occurred. Order matters — the most
// specific and most actionable classes are checked first.
//
// A nil error yields ExitOK; an unrecognized error yields ExitGeneral.
func ClassifyExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	low := strings.ToLower(err.Error())

	contains := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(low, s) {
				return true
			}
		}
		return false
	}

	switch {
	// Rate limiting / quota / credits — checked before generic auth because a
	// 429 is retriable whereas a 401 is not.
	case contains("429", "rate limit", "rate_limit", "too many requests",
		"insufficient credits", "insufficient balance", "out of credits",
		"requires more credits", "can only afford", "quota exceeded"):
		return ExitRateLimit

	// Authentication / authorization — bad or missing key, 401/403.
	case contains("401", "unauthorized", "invalid api key", "invalid_api_key",
		"authentication", "api key is missing", "403", "forbidden",
		"access denied", "payment required"):
		return ExitAuth

	// Context window overflow — distinct from a generic 400 so callers can
	// react by compacting rather than aborting.
	case contains("context length", "context_length", "context window",
		"token limit", "too many tokens", "maximum context",
		"max_tokens exceeded", "max tokens exceeded", "prompt is too long"):
		return ExitContextLimit

	// Policy / permission / guardrail denial.
	case contains("permission denied", "guardrail", "policy", "blocked by",
		"approval denied", "not permitted", "operation not allowed"):
		return ExitPolicyBlock

	// Tool execution failures and tool timeouts.
	case contains("tool timeout", "tool_timeout", "tool execution",
		"tool failed", "tool error"):
		return ExitToolFailure

	// Disk space / quota.
	case contains("no space left", "disk full", "not enough space", "disk quota"):
		return ExitDiskFull

	// Malformed configuration.
	case contains("invalid json in config") ||
		(contains("settings", "config") && contains("unmarshal", "syntax error", "invalid character")):
		return ExitConfig

	// Not found — model/endpoint/resource (404).
	case contains("model not found", "model_not_found", "unknown model",
		"invalid model", "404", "no such host"):
		// "no such host" is a DNS failure, not a 404 — route it to network.
		if contains("no such host") {
			return ExitNetwork
		}
		return ExitNotFound

	// Network errors — DNS, refused/reset connections, provider 5xx, TLS.
	case contains("network is unreachable", "network unreachable",
		"connection refused", "connection reset", "broken pipe",
		"dns", "lookup", "500", "502", "503", "504",
		"internal server error", "bad gateway", "service unavailable",
		"gateway timeout", "certificate", "tls", "x509"):
		return ExitNetwork

	// Timeouts / cancellation — checked after the 5xx gateway-timeout cases so
	// "504 gateway timeout" stays a network error.
	case contains("timeout", "timed out", "deadline exceeded", "context canceled"):
		return ExitTimeout

	default:
		return ExitGeneral
	}
}
