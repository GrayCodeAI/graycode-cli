package plugin

import (
	"context"
	"testing"
)

func TestNewPluginBridge_NoBridge(t *testing.T) {
	m := &Manifest{Name: "p", Version: "1.0.0"}
	pb := NewPluginBridge(m)
	if pb.Ready() {
		t.Error("bridge with no Bridge def should not be ready")
	}
}

func TestNewPluginBridge_MissingCommand(t *testing.T) {
	m := &Manifest{
		Name:    "p",
		Version: "1.0.0",
		Bridge:  &BridgeDef{Command: "definitely-not-a-real-binary-xyz"},
	}
	pb := NewPluginBridge(m)
	if pb.Ready() {
		t.Error("bridge with missing command should not be ready")
	}
	if _, err := pb.Run(context.Background()); err == nil {
		t.Error("Run on non-ready bridge should error")
	}
}

func TestPluginBridge_Run(t *testing.T) {
	m := &Manifest{
		Name:    "echo-plugin",
		Version: "1.0.0",
		Bridge:  &BridgeDef{Command: "echo", Args: []string{"hello"}},
	}
	pb := NewPluginBridge(m)
	if !pb.Ready() {
		// FIXME: echo not available in PATH
		t.Skip("echo not available in PATH")
	}
	out, err := pb.Run(context.Background(), "world")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "hello world" {
		t.Errorf("expected 'hello world', got %q", out)
	}
}

func TestPluginBridge_Name(t *testing.T) {
	m := &Manifest{Name: "mybridge", Version: "1.0.0", Bridge: &BridgeDef{Command: "echo"}}
	pb := NewPluginBridge(m)
	if pb.Name() != "mybridge" {
		t.Errorf("expected name 'mybridge', got %q", pb.Name())
	}
}

func TestManifestValidate_Bridge(t *testing.T) {
	m := &Manifest{Name: "p", Version: "1.0.0", Bridge: &BridgeDef{}}
	if err := m.Validate(); err == nil {
		t.Error("expected validation error for bridge with empty command")
	}

	m.Bridge.Command = "echo"
	if err := m.Validate(); err != nil {
		t.Errorf("valid bridge should pass validation, got: %v", err)
	}
}
