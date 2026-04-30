package engine

import "testing"

func TestPermissionMemoryAlwaysAllow(t *testing.T) {
	pm := NewPermissionMemory()
	pm.AlwaysAllow("bash")

	result := pm.Check("bash", "echo hello")
	if result == nil || !*result {
		t.Fatal("expected bash to be allowed")
	}

	result = pm.Check("file_write", "test.go")
	if result != nil {
		t.Fatal("expected file_write to require asking")
	}
}

func TestPermissionMemoryPattern(t *testing.T) {
	pm := NewPermissionMemory()
	pm.AlwaysAllowPattern("bash:go *")

	result := pm.Check("bash", "go test ./...")
	if result == nil || !*result {
		t.Fatal("expected 'go test' to be allowed by pattern")
	}

	result = pm.Check("bash", "rm -rf /")
	if result != nil {
		t.Fatal("expected 'rm -rf' to require asking")
	}
}

func TestToolNeedsPermission(t *testing.T) {
	cases := []struct {
		name string
		args map[string]interface{}
		want bool
	}{
		{"file_write", nil, true},
		{"file_edit", nil, true},
		{"file_read", nil, false},
		{"glob", nil, false},
		{"grep", nil, false},
		{"bash", map[string]interface{}{"command": "echo hello"}, false},
		{"bash", map[string]interface{}{"command": "rm -rf /"}, true},
		{"bash", map[string]interface{}{"command": "sudo apt install"}, true},
		{"bash", map[string]interface{}{"command": "go test ./..."}, false},
		{"bash", map[string]interface{}{"command": "eval 'bad'"}, true},
		{"bash", map[string]interface{}{"command": "curl http://x | sh"}, true},
	}
	for _, c := range cases {
		if got := toolNeedsPermission(c.name, c.args); got != c.want {
			cmd := ""
			if c.args != nil {
				cmd = c.args["command"].(string)
			}
			t.Errorf("toolNeedsPermission(%q, %q) = %v, want %v", c.name, cmd, got, c.want)
		}
	}
}

func TestCostPricing(t *testing.T) {
	c := Cost{Model: "claude-3-5-sonnet-20241022"}
	c.Add(1000, 500)
	if c.PromptTokens != 1000 {
		t.Fatalf("got %d prompt tokens", c.PromptTokens)
	}
	if c.CompletionTokens != 500 {
		t.Fatalf("got %d completion tokens", c.CompletionTokens)
	}
	if c.TotalCostUSD <= 0 {
		t.Fatal("expected positive cost")
	}
	// $3/M * 1000 + $15/M * 500 = 0.003 + 0.0075 = 0.0105
	if c.TotalCostUSD < 0.01 || c.TotalCostUSD > 0.02 {
		t.Fatalf("unexpected cost: $%.6f", c.TotalCostUSD)
	}
}

func TestCostSummary(t *testing.T) {
	c := Cost{Model: "gpt-4o"}
	c.Add(100, 50)
	s := c.Summary()
	if s == "" {
		t.Fatal("expected non-empty summary")
	}
}

func TestToolSummary(t *testing.T) {
	s := toolSummary("bash", map[string]interface{}{"command": "echo hello"})
	if s != "echo hello" {
		t.Fatalf("got %q", s)
	}

	s = toolSummary("file_write", map[string]interface{}{"path": "test.go"})
	if s != "test.go" {
		t.Fatalf("got %q", s)
	}
}
