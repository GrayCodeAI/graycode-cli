package sandbox

import (
	"context"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.Enabled {
		t.Fatal("expected sandbox disabled by default")
	}
	if c.Type != "none" {
		t.Fatalf("expected type 'none', got %q", c.Type)
	}
}

func TestNew(t *testing.T) {
	s, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
}

func TestRunDisabled(t *testing.T) {
	s, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	cmd, err := s.Run(context.Background(), "echo hello")
	if err != nil {
		t.Fatal(err)
	}
	if cmd == nil {
		t.Fatal("expected command")
	}

	// Actually run it
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "hello\n" {
		t.Fatalf("unexpected output: %q", string(out))
	}
}

func TestIsAvailable(t *testing.T) {
	// Just make sure it doesn't panic
	_ = IsAvailable()
}

func TestRunDocker(t *testing.T) {
	config := &Config{
		Enabled:      true,
		Type:         "docker",
		ReadOnlyDirs: []string{"/tmp"},
		MaxMemoryMB:  256,
		MaxCPUPct:    25,
	}
	s, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	cmd, err := s.Run(context.Background(), "echo test")
	if err != nil {
		t.Fatal(err)
	}
	if cmd == nil {
		t.Fatal("expected command")
	}
}
