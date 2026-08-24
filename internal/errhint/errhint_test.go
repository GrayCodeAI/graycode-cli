package errhint

import (
	"errors"
	"testing"
)

func TestClassifyGatesOnProviderMarker(t *testing.T) {
	// A local error must not draw a provider hint even if it contains a keyword.
	if got := Classify(errors.New("permission denied")); got != Unknown {
		t.Fatalf("local permission denied classified as %v, want Unknown", got)
	}
	if got := Classify(errors.New("provider error: 401 unauthorized")); got != Auth {
		t.Fatalf("classified = %v, want Auth", got)
	}
}

func TestClassifyCategories(t *testing.T) {
	cases := []struct {
		msg  string
		want Category
	}{
		{"provider error: invalid_api_key", Auth},
		{"auth error: 403 forbidden", Auth},
		{"rate limit error: too many requests", RateLimit},
		{"provider request error: 429", RateLimit},
		{"provider error: 529", RateLimit},
		{"provider error: context length exceeded", ContextOverflow},
		{"provider error: prompt is too long", ContextOverflow},
		{"provider error: model not found", ModelNotFound},
		{"provider error: unsupported model", ModelNotFound},
		{"provider stream error: dial tcp 10.0.0.1:443: i/o timeout", Connectivity},
		{"provider error: connection refused", Connectivity},
		{"some unrelated thing", Unknown},
		{"", Unknown},
	}
	for _, tc := range cases {
		got := Classify(errors.New(tc.msg))
		if got != tc.want {
			t.Errorf("Classify(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}

func TestClassifyNil(t *testing.T) {
	if got := Classify(nil); got != Unknown {
		t.Fatalf("Classify(nil) = %v, want Unknown", got)
	}
}

func TestHints(t *testing.T) {
	if TUIHint(errors.New("provider error: invalid api key")) == "" {
		t.Fatal("expected a TUI hint for auth")
	}
	if CLIHint(errors.New("provider error: invalid api key")) == "" {
		t.Fatal("expected a CLI hint for auth")
	}
	if TUIHint(errors.New("local file error")) != "" {
		t.Fatal("expected no hint for local error")
	}
	if CLIHint(errors.New("local file error")) != "" {
		t.Fatal("expected no hint for local error")
	}
}

func TestHasStatusCode(t *testing.T) {
	if !HasStatusCode("provider error: 401", "401") {
		t.Fatal("expected standalone 401 to match")
	}
	if HasStatusCode("completed in 4290ms", "429") {
		t.Fatal("429 embedded in 4290 must not match")
	}
	if HasStatusCode("request id 14015", "401") {
		t.Fatal("401 embedded in 14015 must not match")
	}
	if !HasStatusCode("provider: 429 too many", "429") {
		t.Fatal("expected standalone 429 to match")
	}
}
