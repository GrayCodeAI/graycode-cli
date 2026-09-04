package graycodeerr

import (
	"errors"
	"testing"
)

func TestBridgeError_Error(t *testing.T) {
	err := NewBridgeError("harrier", "Remember", "connection refused", nil)
	want := "harrier bridge: Remember: connection refused"
	if got := err.Error(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBridgeError_Wrap(t *testing.T) {
	inner := errors.New("timeout")
	err := NewBridgeError("kestrel", "Review", "provider failed", inner)
	if !errors.Is(err, inner) {
		t.Error("Unwrap should return inner error")
	}
}
