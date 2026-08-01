package engine

import "testing"

func TestToolServiceNormalizeOutputKeepsSmallResults(t *testing.T) {
	service := NewToolService(nil)
	const want = "short tool result"
	if got := service.NormalizeOutput(want, "Read", "call-1", 128_000); got != want {
		t.Fatalf("NormalizeOutput changed a small result: got %q, want %q", got, want)
	}
}
