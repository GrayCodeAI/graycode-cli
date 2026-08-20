package mission

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockProvider struct {
	name string
	caps SubagentCapabilities
	run  func(ctx context.Context, req SubagentRequest) (*SubagentResult, error)
}

func (m *mockProvider) Name() string                       { return m.name }
func (m *mockProvider) Capabilities() SubagentCapabilities { return m.caps }
func (m *mockProvider) Run(ctx context.Context, req SubagentRequest) (*SubagentResult, error) {
	if m.run != nil {
		return m.run(ctx, req)
	}
	return &SubagentResult{Status: "success", Output: "done"}, nil
}

func TestProviderRegistry_RegisterAndList(t *testing.T) {
	reg := NewProviderRegistry()

	disposer1 := reg.Register(&mockProvider{name: "beta"})
	disposer2 := reg.Register(&mockProvider{name: "alpha"})

	list := reg.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(list))
	}
	if list[0].Name() != "alpha" || list[1].Name() != "beta" {
		t.Errorf("expected alphabetical sort [alpha, beta], got [%s, %s]", list[0].Name(), list[1].Name())
	}

	// Disposing alpha removes it
	disposer2()
	if _, ok := reg.Get("alpha"); ok {
		t.Error("expected alpha to be removed after disposer call")
	}

	disposer1()
	if len(reg.List()) != 0 {
		t.Errorf("expected 0 providers after all disposed, got %d", len(reg.List()))
	}
}

func TestProviderRegistry_CapabilityChecks(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(&mockProvider{
		name: "limited",
		caps: SubagentCapabilities{
			SupportsSchema:    false,
			MaxDepth:          2,
			SupportedPersonas: []string{"reviewer", "tester"},
		},
	})

	ctx := context.Background()

	// 1. OutputSchema rejection
	_, err := reg.Run(ctx, SubagentRequest{
		Name:         "limited",
		Task:         "Audit code",
		OutputSchema: map[string]interface{}{"type": "object"},
	})
	if !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("expected ErrUnsupportedCapability, got %v", err)
	}

	// 2. Depth limit rejection
	_, err = reg.Run(ctx, SubagentRequest{
		Name:  "limited",
		Task:  "Audit code",
		Depth: 3, // exceeds max depth 2
	})
	if !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("expected ErrDepthExceeded, got %v", err)
	}

	// 3. Persona rejection
	_, err = reg.Run(ctx, SubagentRequest{
		Name:    "limited",
		Task:    "Audit code",
		Persona: "unsupported-persona",
	})
	if !errors.Is(err, ErrUnsupportedPersona) {
		t.Fatalf("expected ErrUnsupportedPersona, got %v", err)
	}

	// 4. Valid run
	res, err := reg.Run(ctx, SubagentRequest{
		Name:    "limited",
		Task:    "Audit code",
		Persona: "reviewer",
		Depth:   1,
	})
	if err != nil {
		t.Fatalf("expected valid run to succeed, got error: %v", err)
	}
	if res.Status != "success" {
		t.Errorf("expected status success, got %s", res.Status)
	}
}

func TestProviderRegistry_StructuredOutput(t *testing.T) {
	reg := NewProviderRegistry()
	reg.Register(&mockProvider{
		name: "schema-agent",
		caps: SubagentCapabilities{
			SupportsSchema: true,
		},
		run: func(ctx context.Context, req SubagentRequest) (*SubagentResult, error) {
			return &SubagentResult{
				Status:   "success",
				Output:   `{"findingCount": 3, "summary": "Found 3 issues"}`,
				Duration: 10 * time.Millisecond,
			}, nil
		},
	})

	ctx := context.Background()
	res, err := reg.Run(ctx, SubagentRequest{
		Name:         "schema-agent",
		Task:         "Scan security vulnerabilities",
		OutputSchema: map[string]interface{}{"type": "object"},
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if res.Data == nil {
		t.Fatal("expected structured data to be parsed, got nil")
	}
	if res.Data["findingCount"] != float64(3) {
		t.Errorf("expected findingCount 3, got %#v", res.Data["findingCount"])
	}
}

func TestProviderRegistry_UnknownProvider(t *testing.T) {
	reg := NewProviderRegistry()
	_, err := reg.Run(context.Background(), SubagentRequest{
		Name: "nonexistent",
		Task: "do something",
	})
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("expected ErrProviderNotFound, got %v", err)
	}
}
