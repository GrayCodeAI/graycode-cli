package engine

import (
	"context"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/tool"
)

// TestSessionLifecycle tests the full session lifecycle.
func TestSessionLifecycle(t *testing.T) {
	registry := tool.NewRegistry(
		tool.BashTool{},
		tool.FileReadTool{},
		tool.FileWriteTool{},
	)

	sess := NewSession("", "", "test system prompt", registry)
	if sess == nil {
		t.Fatal("expected session")
	}

	// Test basic properties: no implicit provider/model defaults in Hawk.
	if sess.Provider() != "" {
		t.Fatalf("expected empty provider by default, got %q", sess.Provider())
	}
	if sess.Model() != "" {
		t.Fatalf("expected empty model by default, got %q", sess.Model())
	}

	// Test message management
	sess.AddUser("Hello")
	if sess.MessageCount() != 1 {
		t.Fatalf("expected 1 message, got %d", sess.MessageCount())
	}

	sess.AddAssistant("Hi there")
	if sess.MessageCount() != 2 {
		t.Fatalf("expected 2 messages, got %d", sess.MessageCount())
	}

	// Test raw messages
	raw := sess.RawMessages()
	if len(raw) != 2 {
		t.Fatalf("expected 2 raw messages, got %d", len(raw))
	}

	// Test system context
	sess.AppendSystemContext("Additional context")
	if sess.system == "" {
		t.Fatal("expected system prompt")
	}

	// Test allowed dirs
	sess.SetAllowedDirs([]string{"/tmp", "/home"})
	if len(sess.AllowedDirs) != 2 {
		t.Fatalf("expected 2 allowed dirs, got %d", len(sess.AllowedDirs))
	}
}

// TestPermissionModes tests all permission modes.
func TestPermissionModes(t *testing.T) {
	sess := NewSession("", "", "test", nil)

	modes := []string{"default", "acceptEdits", "bypassPermissions", "dontAsk", "plan"}
	for _, mode := range modes {
		if err := sess.SetPermissionMode(mode); err != nil {
			t.Errorf("SetPermissionMode(%q) failed: %v", mode, err)
		}
		if string(sess.Mode) != mode {
			t.Errorf("expected mode %q, got %q", mode, sess.Mode)
		}
	}

	// Test invalid mode
	if err := sess.SetPermissionMode("invalid"); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

// TestBudgetAndTurns tests budget and turn limiting.
func TestBudgetAndTurns(t *testing.T) {
	sess := NewSession("", "", "test", nil)

	// Test max turns
	if err := sess.SetMaxTurns(10); err != nil {
		t.Fatal(err)
	}
	if sess.MaxTurns != 10 {
		t.Fatalf("expected max turns 10, got %d", sess.MaxTurns)
	}

	// Test negative turns
	if err := sess.SetMaxTurns(-1); err == nil {
		t.Fatal("expected error for negative turns")
	}

	// Test budget
	if err := sess.SetMaxBudgetUSD(5.0); err != nil {
		t.Fatal(err)
	}
	if sess.MaxBudgetUSD != 5.0 {
		t.Fatalf("expected budget 5.0, got %f", sess.MaxBudgetUSD)
	}

	// Test negative budget
	if err := sess.SetMaxBudgetUSD(-1.0); err == nil {
		t.Fatal("expected error for negative budget")
	}
}

// TestCompact tests conversation compaction.
func TestCompact(t *testing.T) {
	sess := NewSession("", "", "test", nil)

	// Add many messages
	for i := 0; i < 30; i++ {
		if i%2 == 0 {
			sess.AddUser("Message " + string(rune('0'+i%10)))
		} else {
			sess.AddAssistant("Response " + string(rune('0'+i%10)))
		}
	}

	before := sess.MessageCount()
	sess.Compact()
	after := sess.MessageCount()

	if after >= before {
		t.Fatalf("expected compaction to reduce messages, before=%d after=%d", before, after)
	}
}

// TestCostTracking tests cost tracking.
func TestCostTracking(t *testing.T) {
	c := Cost{Model: "gpt-4o"}
	c.Add(1000, 500)

	if c.PromptTokens != 1000 {
		t.Fatalf("expected 1000 prompt tokens, got %d", c.PromptTokens)
	}
	if c.CompletionTokens != 500 {
		t.Fatalf("expected 500 completion tokens, got %d", c.CompletionTokens)
	}
	if c.Total() <= 0 {
		t.Fatal("expected positive cost")
	}

	// Test summary
	summary := c.Summary()
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
}

// TestToolRegistry tests tool registry functionality.
func TestToolRegistry(t *testing.T) {
	registry := tool.NewRegistry(
		tool.BashTool{},
		tool.FileReadTool{},
	)

	// Test primary tools
	tools := registry.PrimaryTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 primary tools, got %d", len(tools))
	}

	// Test alias resolution
	bash, ok := registry.Get("bash")
	if !ok {
		t.Fatal("expected to find 'bash' alias")
	}
	if bash.Name() != "Bash" {
		t.Fatalf("expected primary name 'Bash', got %q", bash.Name())
	}

	// Test unknown tool
	_, ok = registry.Get("nonexistent")
	if ok {
		t.Fatal("expected not to find unknown tool")
	}
}

// TestPermissionMemory tests permission memory.
func TestPermissionMemory(t *testing.T) {
	pm := NewPermissionMemory()

	// Test always allow
	pm.AlwaysAllow("Bash")
	if decision := pm.Check("Bash", "echo hello"); decision == nil || !*decision {
		t.Fatal("expected Bash to be allowed")
	}

	// Test always deny
	pm.AlwaysDeny("Write")
	if decision := pm.Check("Write", "/etc/passwd"); decision == nil || *decision {
		t.Fatal("expected Write to be denied")
	}

	// Test pattern allow
	pm.AlwaysAllowPattern("Bash:git *")
	if decision := pm.Check("Bash", "git status"); decision == nil || !*decision {
		t.Fatal("expected 'git status' to be allowed")
	}

	// Test unknown
	if decision := pm.Check("Read", "file.txt"); decision != nil {
		t.Fatal("expected nil decision for unknown")
	}
}

// TestStreamTimeout tests stream timeout handling.
func TestStreamTimeout(t *testing.T) {
	sess := NewSession("", "", "test", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	ch, err := sess.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Should eventually close due to timeout
	select {
	case <-ch:
		// Expected
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not close within timeout")
	}
}
