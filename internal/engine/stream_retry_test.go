package engine

import (
	"errors"
	"testing"
	"time"
)

func TestParseRetryDelayHint(t *testing.T) {
	cases := []struct {
		msg  string
		want time.Duration
	}{
		{"HTTP 429: retry in 5s", 5 * time.Second},
		{"rate limited, try again after 1200ms", 1200 * time.Millisecond},
		{"retry after 2 seconds", 2 * time.Second},
		{"no delay hint here", 0},
	}
	for _, tc := range cases {
		got := parseRetryDelayHint(tc.msg)
		if got != tc.want {
			t.Errorf("parseRetryDelayHint(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}

func TestStreamRetryDelayHonorsParsedHint(t *testing.T) {
	err := errors.New("HTTP 429: retry in 7s")
	if d := streamRetryDelay(err, 0); d != 7*time.Second {
		t.Fatalf("streamRetryDelay with hint = %v, want 7s", d)
	}
}

func TestStreamRetryDelayCappedAtMax(t *testing.T) {
	err := errors.New("retry in 300s")
	if d := streamRetryDelay(err, 0); d != maxStreamRetryDelay {
		t.Fatalf("streamRetryDelay not capped, got %v want %v", d, maxStreamRetryDelay)
	}
}

func TestStreamRetryDelayFallsBackToLinear(t *testing.T) {
	err := errors.New("connection reset by peer")
	if d := streamRetryDelay(err, 2); d != 3*time.Second {
		t.Fatalf("fallback delay = %v, want 3s", d)
	}
}

func TestIsRetryableStreamError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("connection reset by peer"), true},
		{errors.New("EOF"), true},
		{errors.New("HTTP 429 rate limit exceeded"), true},
		{errors.New("HTTP 503 unavailable"), true},
		{errors.New("HTTP 500 internal server error"), true},
		{errors.New("HTTP 401 unauthorized"), false},
		{errors.New("HTTP 404 not found"), false},
	}
	for _, c := range cases {
		if got := isRetryableStreamError(c.err); got != c.want {
			t.Errorf("isRetryableStreamError(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
