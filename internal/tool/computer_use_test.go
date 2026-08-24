package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type mockComputer struct{}

func (mockComputer) Name() string { return "mock" }
func (mockComputer) Snapshot(ctx context.Context) (string, error) {
	return "UI: button @e1 Submit", nil
}
func (mockComputer) Click(ctx context.Context, ref string) error       { return nil }
func (mockComputer) Type(ctx context.Context, text string) error       { return nil }
func (mockComputer) Scroll(ctx context.Context, ref, dir string) error { return nil }
func (mockComputer) Press(ctx context.Context, chord string) error     { return nil }
func (mockComputer) Screenshot(ctx context.Context) (string, error)    { return "/tmp/shot.png", nil }

func TestComputerUseRequiresBackend(t *testing.T) {
	SetComputerBackend(nil)
	tool := ComputerUseTool{}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"snapshot"}`)); err == nil || !strings.Contains(err.Error(), "no computer backend") {
		t.Fatalf("err = %v", err)
	}
}

func TestComputerUseSnapshot(t *testing.T) {
	SetComputerBackend(mockComputer{})
	defer SetComputerBackend(nil)
	out, err := ComputerUseTool{}.Execute(context.Background(), json.RawMessage(`{"action":"snapshot"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "@e1 Submit") {
		t.Fatalf("out = %q", out)
	}
}

func TestComputerUseClickRequiresTarget(t *testing.T) {
	SetComputerBackend(mockComputer{})
	defer SetComputerBackend(nil)
	tool := ComputerUseTool{}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"click"}`)); err == nil || !strings.Contains(err.Error(), "requires a target") {
		t.Fatalf("err = %v", err)
	}
}

func TestComputerUseTypeAndPress(t *testing.T) {
	SetComputerBackend(mockComputer{})
	defer SetComputerBackend(nil)
	out, err := ComputerUseTool{}.Execute(context.Background(), json.RawMessage(`{"action":"type","text":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "5 characters") {
		t.Fatalf("out = %q", out)
	}
	out, err = ComputerUseTool{}.Execute(context.Background(), json.RawMessage(`{"action":"press","text":"cmd+k"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cmd+k") {
		t.Fatalf("out = %q", out)
	}
}

func TestComputerUseScreenshot(t *testing.T) {
	SetComputerBackend(mockComputer{})
	defer SetComputerBackend(nil)
	out, err := ComputerUseTool{}.Execute(context.Background(), json.RawMessage(`{"action":"screenshot"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "shot.png") {
		t.Fatalf("out = %q", out)
	}
}

func TestComputerUseInvalidAction(t *testing.T) {
	SetComputerBackend(mockComputer{})
	defer SetComputerBackend(nil)
	tool := ComputerUseTool{}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"nope"}`)); err == nil {
		t.Fatal("expected error for invalid action")
	}
}
