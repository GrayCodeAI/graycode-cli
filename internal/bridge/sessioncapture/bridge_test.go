package sessioncapture

import (
	"context"
	"testing"
)

func TestNewBridge_NoTrace(t *testing.T) {
	// Save and clear PATH so trace won't be found
	t.Setenv("PATH", "/nonexistent")
	b := NewBridge()
	if b.Ready() {
		t.Error("expected bridge to not be ready when trace is absent")
	}
}

func TestBridge_EnableWithoutCLI(t *testing.T) {
	b := &Bridge{ready: false}
	err := b.Enable(context.TODO(), "/tmp")
	if err == nil {
		t.Error("expected error when trace CLI not found")
	}
}

func TestBridge_StatusWithoutCLI(t *testing.T) {
	b := &Bridge{ready: false}
	s, err := b.GetStatus(context.TODO(), "/tmp")
	if err == nil {
		t.Error("expected error when trace CLI not found")
	}
	if s == nil {
		t.Error("expected non-nil empty status even on error")
	}
}
