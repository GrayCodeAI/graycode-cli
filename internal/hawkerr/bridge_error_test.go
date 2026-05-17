package hawkerr

import (
	"errors"
	"testing"
)

func TestBridgeError_Error(t *testing.T) {
	err := NewBridgeError("yaad", "Remember", "connection refused", nil)
	want := "yaad bridge: Remember: connection refused"
	if got := err.Error(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBridgeError_Wrap(t *testing.T) {
	inner := errors.New("timeout")
	err := NewBridgeError("sight", "Review", "provider failed", inner)
	if !errors.Is(err, inner) {
		t.Error("Unwrap should return inner error")
	}
}
