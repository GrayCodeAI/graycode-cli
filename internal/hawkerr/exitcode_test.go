package hawkerr

import (
	"errors"
	"testing"
)

func TestClassifyExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, ExitOK},
		{"rate limit 429", errors.New("HTTP 429 Too Many Requests"), ExitRateLimit},
		{"rate limit phrase", errors.New("you have hit the rate limit"), ExitRateLimit},
		{"insufficient credits", errors.New("requires more credits to run"), ExitRateLimit},
		{"auth 401", errors.New("401 Unauthorized"), ExitAuth},
		{"invalid api key", errors.New("invalid_api_key provided"), ExitAuth},
		{"forbidden 403", errors.New("403 Forbidden: access denied"), ExitAuth},
		{"context window", errors.New("prompt is too long for the context window"), ExitContextLimit},
		{"token limit", errors.New("maximum context length exceeded"), ExitContextLimit},
		{"policy block", errors.New("operation not allowed by guardrail policy"), ExitPolicyBlock},
		{"permission denied", errors.New("permission denied writing file"), ExitPolicyBlock},
		{"tool timeout", errors.New("tool timeout after 120s"), ExitToolFailure},
		{"disk full", errors.New("write failed: no space left on device"), ExitDiskFull},
		{"bad config", errors.New("failed to parse settings: invalid character '}'"), ExitConfig},
		{"model not found", errors.New("model_not_found: gpt-5-ultra"), ExitNotFound},
		{"404 model", errors.New("404 unknown model"), ExitNotFound},
		{"dns no such host", errors.New("dial tcp: lookup api.example.com: no such host"), ExitNetwork},
		{"connection refused", errors.New("connection refused"), ExitNetwork},
		{"provider 503", errors.New("503 Service Unavailable"), ExitNetwork},
		{"tls", errors.New("x509: certificate signed by unknown authority"), ExitNetwork},
		{"timeout", errors.New("context deadline exceeded"), ExitTimeout},
		{"plain timeout", errors.New("request timed out"), ExitTimeout},
		{"unclassified", errors.New("something weird happened"), ExitGeneral},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyExitCode(tc.err); got != tc.want {
				t.Errorf("ClassifyExitCode(%q) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// Wrapped errors must classify by their flattened message too.
func TestClassifyExitCode_Wrapped(t *testing.T) {
	wrapped := errors.New("call failed: 429 rate_limit")
	if got := ClassifyExitCode(wrapped); got != ExitRateLimit {
		t.Errorf("wrapped rate-limit = %d, want %d", got, ExitRateLimit)
	}
}
