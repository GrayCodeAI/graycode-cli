//nolint:errcheck
package permissions

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// --- extractFirstJSONObject ---

func TestExtractFirstJSONObject_Simple(t *testing.T) {
	got := extractFirstJSONObject(`{"a": 1}`)
	want := `{"a": 1}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractFirstJSONObject_WithSurroundingText(t *testing.T) {
	// LLM-style response: explanation, then JSON, then more text.
	in := `Sure, here is the JSON: {"allowed": true, "reason": "ok"} and that's it.`
	want := `{"allowed": true, "reason": "ok"}`
	if got := extractFirstJSONObject(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractFirstJSONObject_Nested(t *testing.T) {
	// Nested braces must count correctly: the inner {"b": 2} should
	// not be returned standalone.
	in := `{"a": {"b": 2}, "c": 3}`
	want := `{"a": {"b": 2}, "c": 3}`
	if got := extractFirstJSONObject(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractFirstJSONObject_MultipleObjects(t *testing.T) {
	// First balanced object wins; the second object is ignored.
	in := `{"a": 1} some text {"b": 2}`
	want := `{"a": 1}`
	if got := extractFirstJSONObject(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractFirstJSONObject_BraceInString(t *testing.T) {
	// A "}" inside a string literal must not close the object.
	in := `{"a": "with } brace"}`
	want := `{"a": "with } brace"}`
	if got := extractFirstJSONObject(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractFirstJSONObject_BraceAndQuoteInString(t *testing.T) {
	// String with both a brace AND an escaped quote.
	in := `{"a": "with } and \" inside"}`
	want := `{"a": "with } and \" inside"}`
	if got := extractFirstJSONObject(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractFirstJSONObject_OpenBraceInString(t *testing.T) {
	// A "{" inside a string literal must not start a new object.
	in := `{"a": "with { open"}`
	want := `{"a": "with { open"}`
	if got := extractFirstJSONObject(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractFirstJSONObject_Multiline(t *testing.T) {
	// Multi-line JSON, common in LLM outputs.
	in := "{\n  \"allowed\": true,\n  \"reason\": \"ok\"\n}"
	want := "{\n  \"allowed\": true,\n  \"reason\": \"ok\"\n}"
	if got := extractFirstJSONObject(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractFirstJSONObject_NoObject(t *testing.T) {
	if got := extractFirstJSONObject("no braces here at all"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractFirstJSONObject_Unbalanced(t *testing.T) {
	// Open brace without close — should not return a partial.
	if got := extractFirstJSONObject("{ this is never closed"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractFirstJSONObject_Empty(t *testing.T) {
	if got := extractFirstJSONObject(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestExtractFirstJSONObject_OnlyCloseBrace(t *testing.T) {
	if got := extractFirstJSONObject("}"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// --- parseGuardianResponse ---

func TestParseGuardianResponse_Valid(t *testing.T) {
	d, err := parseGuardianResponse(`{"allowed": true, "reason": "read-only op", "confidence": 0.9}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Allowed {
		t.Errorf("Allowed = false, want true")
	}
	if d.Reason != "read-only op" {
		t.Errorf("Reason = %q, want 'read-only op'", d.Reason)
	}
	if d.Confidence != 0.9 {
		t.Errorf("Confidence = %v, want 0.9", d.Confidence)
	}
}

func TestParseGuardianResponse_WithSurroundingText(t *testing.T) {
	// LLM-style preamble + JSON. The old strings.Index + strings.LastIndex
	// would have failed here if the preamble contained a literal '}'.
	in := `Sure, here is the JSON: {"allowed": false, "reason": "rm -rf is dangerous", "confidence": 0.95}`
	d, err := parseGuardianResponse(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Allowed {
		t.Errorf("Allowed = true, want false")
	}
	if d.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95", d.Confidence)
	}
}

func TestParseGuardianResponse_MultipleObjects(t *testing.T) {
	// LLM sometimes streams multiple JSON objects; the first
	// brace-balanced one wins.
	in := `{"allowed": true, "reason": "first"} {"allowed": false, "reason": "ignored"}`
	d, err := parseGuardianResponse(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Allowed {
		t.Errorf("Allowed = false, want true (first object should win)")
	}
	if d.Reason != "first" {
		t.Errorf("Reason = %q, want 'first'", d.Reason)
	}
}

func TestParseGuardianResponse_BraceInString(t *testing.T) {
	in := `{"allowed": true, "reason": "looks like {} in the args"}`
	d, err := parseGuardianResponse(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Allowed {
		t.Errorf("Allowed = false, want true")
	}
}

func TestParseGuardianResponse_ConfidenceClamp(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want float64
	}{
		{"negative clamped to 0", `{"allowed":true,"reason":"x","confidence":-0.5}`, 0},
		{"above 1 clamped to 1", `{"allowed":true,"reason":"x","confidence":1.5}`, 1},
		{"exact 0 stays 0", `{"allowed":true,"reason":"x","confidence":0}`, 0},
		{"exact 1 stays 1", `{"allowed":true,"reason":"x","confidence":1}`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := parseGuardianResponse(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d.Confidence != tc.want {
				t.Errorf("Confidence = %v, want %v", d.Confidence, tc.want)
			}
		})
	}
}

func TestParseGuardianResponse_NoObject(t *testing.T) {
	_, err := parseGuardianResponse("the LLM gave us plain text with no JSON")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrGuardianUnparseable) {
		t.Errorf("err = %v, want ErrGuardianUnparseable", err)
	}
}

func TestParseGuardianResponse_MalformedJSON(t *testing.T) {
	_, err := parseGuardianResponse(`{"allowed": tru, "reason": "typo"}`)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrGuardianUnparseable) {
		t.Errorf("err = %v, want ErrGuardianUnparseable", err)
	}
}

func TestParseGuardianResponse_Unbalanced(t *testing.T) {
	_, err := parseGuardianResponse(`{"allowed": true`)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrGuardianUnparseable) {
		t.Errorf("err = %v, want ErrGuardianUnparseable", err)
	}
}

// --- Guardian cap configuration ---

func TestGuardian_DefaultCapIsFive(t *testing.T) {
	g := NewGuardian(nil)
	if g.MaxConsecutiveDenials != 5 {
		t.Errorf("default MaxConsecutiveDenials = %d, want 5", g.MaxConsecutiveDenials)
	}
}

func TestGuardian_SetMaxConsecutiveDenials(t *testing.T) {
	g := NewGuardian(nil)
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"0 clamped to 1", 0, 1},
		{"negative clamped to 1", -5, 1},
		{"1 stays 1", 1, 1},
		{"5 stays 5", 5, 5},
		{"20 stays 20", 20, 20},
		{"21 clamped to 20", 21, 20},
		{"999 clamped to 20", 999, 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := g.SetMaxConsecutiveDenials(tc.in)
			if got != tc.want {
				t.Errorf("SetMaxConsecutiveDenials(%d) = %d, want %d", tc.in, got, tc.want)
			}
			if g.MaxConsecutiveDenials != tc.want {
				t.Errorf("g.MaxConsecutiveDenials = %d, want %d", g.MaxConsecutiveDenials, tc.want)
			}
		})
	}
}

// TestGuardian_ParseFailureDoesNotIncrementCounter is the regression
// test for the review finding: a malformed LLM response (parse
// failure) must NOT count toward the circuit-breaker cap. It's a
// model artefact, not a security signal.
func TestGuardian_ParseFailureDoesNotIncrementCounter(t *testing.T) {
	// LLM returns malformed JSON every time. After 10 calls, the
	// counter should still be 0 (parse failures don't bump it).
	g := NewGuardian(func(ctx context.Context, prompt string) (string, error) {
		return "not json", nil
	})
	g.SetMaxConsecutiveDenials(100) // high cap so we don't hit it

	for i := 0; i < 10; i++ {
		_, err := g.Review(context.Background(), GuardianRequest{
			ToolName:  "Bash",
			Arguments: map[string]interface{}{"cmd": "ls"},
		})
		if !errors.Is(err, ErrGuardianUnparseable) {
			t.Errorf("call %d: err = %v, want ErrGuardianUnparseable", i, err)
		}
	}

	// Counter must be zero — parse failures don't trip the breaker.
	if g.consecutiveDenials != 0 {
		t.Errorf("consecutiveDenials = %d, want 0 (parse failures must not increment)", g.consecutiveDenials)
	}
}

// TestGuardian_SuccessfulDenyIncrementsCounter is the positive
// counter-test: a successful parse + Allowed=false DOES increment
// the counter. This is the existing behavior, preserved.
func TestGuardian_SuccessfulDenyIncrementsCounter(t *testing.T) {
	g := NewGuardian(func(ctx context.Context, prompt string) (string, error) {
		return `{"allowed": false, "reason": "dangerous", "confidence": 0.95}`, nil
	})
	g.SetMaxConsecutiveDenials(100)

	for i := 0; i < 5; i++ {
		_, _ = g.Review(context.Background(), GuardianRequest{
			ToolName:  "Bash",
			Arguments: map[string]interface{}{"cmd": "rm -rf /"},
		})
	}

	if g.consecutiveDenials != 5 {
		t.Errorf("consecutiveDenials = %d, want 5", g.consecutiveDenials)
	}
}

// TestGuardian_SurroundingTextDoesNotBreakParse is an integration
// test: the full Review path with a LLM-style preamble must
// succeed (regression for the strings.Index + strings.LastIndex bug).
func TestGuardian_SurroundingTextDoesNotBreakParse(t *testing.T) {
	g := NewGuardian(func(ctx context.Context, prompt string) (string, error) {
		// Note the literal "}" in the explanation text — the old
		// strings.LastIndex would have closed the object early.
		return "I reviewed this. The answer is: {\"allowed\": true, \"reason\": \"safe\", \"confidence\": 0.95} That's my call.", nil
	})
	d, err := g.Review(context.Background(), GuardianRequest{
		ToolName:  "Bash",
		Arguments: map[string]interface{}{"cmd": "ls"},
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if !d.Allowed {
		t.Errorf("Allowed = false, want true")
	}
	if d.Reason != "safe" {
		t.Errorf("Reason = %q, want 'safe'", d.Reason)
	}
}

// TestGuardian_ResetCircuitBreaker is a sanity check on the
// existing reset method, paired with the configurable cap.
func TestGuardian_ResetCircuitBreaker(t *testing.T) {
	g := NewGuardian(func(ctx context.Context, prompt string) (string, error) {
		return `{"allowed": false, "reason": "no", "confidence": 0.95}`, nil
	})
	g.SetMaxConsecutiveDenials(10)
	for i := 0; i < 5; i++ {
		_, _ = g.Review(context.Background(), GuardianRequest{ToolName: "Bash"})
	}
	if g.consecutiveDenials != 5 {
		t.Fatalf("consecutiveDenials = %d, want 5 before reset", g.consecutiveDenials)
	}
	g.ResetCircuitBreaker()
	if g.consecutiveDenials != 0 {
		t.Errorf("consecutiveDenials = %d, want 0 after reset", g.consecutiveDenials)
	}
}

// TestTruncateForLog: a tiny helper sanity test.
func TestTruncateForLog(t *testing.T) {
	if got := truncateForLog("short", 100); got != "short" {
		t.Errorf("got %q, want 'short'", got)
	}
	long := strings.Repeat("x", 250)
	got := truncateForLog(long, 100)
	if len(got) != 103 { // 100 chars + "..."
		t.Errorf("len(got) = %d, want 103", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("got %q, want suffix '...'", got)
	}
}
