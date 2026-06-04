package jsonc_test

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/jsonc"
)

func TestValidate_EmptyDocument(t *testing.T) {
	r := jsonc.ValidateClaudeSettings(map[string]interface{}{})
	if !r.Valid() {
		t.Errorf("expected empty doc to validate, got errors: %v", r.Errors)
	}
}

func TestValidate_GoodModel(t *testing.T) {
	r := jsonc.ValidateClaudeSettings(map[string]interface{}{
		"model": "claude-sonnet-4-6",
	})
	if !r.Valid() {
		t.Errorf("expected valid: %v", r.Errors)
	}
}

func TestValidate_EmptyModel(t *testing.T) {
	r := jsonc.ValidateClaudeSettings(map[string]interface{}{
		"model": "",
	})
	if r.Valid() {
		t.Error("expected invalid empty model")
	}
}

func TestValidate_NonStringModel(t *testing.T) {
	r := jsonc.ValidateClaudeSettings(map[string]interface{}{
		"model": 42,
	})
	if r.Valid() {
		t.Error("expected invalid non-string model")
	}
}

func TestValidate_Permissions(t *testing.T) {
	tests := []struct {
		name    string
		perms   map[string]interface{}
		wantOK  bool
		wantSub string
	}{
		{
			"good allow/deny",
			map[string]interface{}{
				"allow": []interface{}{"Read", "Grep"},
				"deny":  []interface{}{"Bash(rm -rf:*)"},
			},
			true, "",
		},
		{
			"defaultMode acceptEdits",
			map[string]interface{}{"defaultMode": "acceptEdits"},
			true, "",
		},
		{
			"defaultMode invalid",
			map[string]interface{}{"defaultMode": "maybe"},
			false, "defaultMode",
		},
		{
			"allow not array",
			map[string]interface{}{"allow": "Read"},
			false, "permissions.allow",
		},
		{
			"allow with non-string",
			map[string]interface{}{"allow": []interface{}{"Read", 42}},
			false, "permissions.allow[1]",
		},
		{
			"allow with empty string",
			map[string]interface{}{"allow": []interface{}{""}},
			false, "permissions.allow[0]",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := jsonc.ValidateClaudeSettings(map[string]interface{}{
				"permissions": tc.perms,
			})
			if tc.wantOK && !r.Valid() {
				t.Errorf("expected valid, got: %v", r.Errors)
			}
			if !tc.wantOK && r.Valid() {
				t.Errorf("expected invalid, got valid")
			}
			if !tc.wantOK && tc.wantSub != "" {
				found := false
				for _, e := range r.Errors {
					if strings.Contains(e.Error(), tc.wantSub) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got: %v", tc.wantSub, r.Errors)
				}
			}
		})
	}
}

func TestValidate_Hooks(t *testing.T) {
	good := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "/usr/local/bin/pre-bash",
						},
					},
				},
			},
		},
	}
	r := jsonc.ValidateClaudeSettings(good)
	if !r.Valid() {
		t.Errorf("expected valid good hooks: %v", r.Errors)
	}

	// Missing hooks field
	bad := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
				},
			},
		},
	}
	r = jsonc.ValidateClaudeSettings(bad)
	if r.Valid() {
		t.Error("expected invalid: missing hooks field")
	}

	// Bad type
	badType := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{
							"type": "weird",
						},
					},
				},
			},
		},
	}
	r = jsonc.ValidateClaudeSettings(badType)
	if r.Valid() {
		t.Error("expected invalid: bad hook type")
	}
}

func TestValidate_MCPServers(t *testing.T) {
	good := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"stdio-server": map[string]interface{}{
				"type":    "stdio",
				"command": "/usr/local/bin/mcp-server",
			},
			"http-server": map[string]interface{}{
				"type": "http",
				"url":  "https://example.com/mcp",
			},
		},
	}
	r := jsonc.ValidateClaudeSettings(good)
	if !r.Valid() {
		t.Errorf("expected valid: %v", r.Errors)
	}

	// Missing command for stdio
	bad := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"x": map[string]interface{}{
				"type": "stdio",
			},
		},
	}
	r = jsonc.ValidateClaudeSettings(bad)
	if r.Valid() {
		t.Error("expected invalid: stdio missing command")
	}
}

func TestValidate_Env(t *testing.T) {
	good := map[string]interface{}{
		"env": map[string]interface{}{
			"FOO": "bar",
			"BAZ": "qux",
		},
	}
	r := jsonc.ValidateClaudeSettings(good)
	if !r.Valid() {
		t.Errorf("expected valid: %v", r.Errors)
	}

	bad := map[string]interface{}{
		"env": map[string]interface{}{
			"FOO": 42,
		},
	}
	r = jsonc.ValidateClaudeSettings(bad)
	if r.Valid() {
		t.Error("expected invalid: env value not string")
	}
}

func TestValidate_IncludeCoAuthoredBy(t *testing.T) {
	good := map[string]interface{}{"includeCoAuthoredBy": true}
	r := jsonc.ValidateClaudeSettings(good)
	if !r.Valid() {
		t.Errorf("expected valid: %v", r.Errors)
	}

	bad := map[string]interface{}{"includeCoAuthoredBy": "yes"}
	r = jsonc.ValidateClaudeSettings(bad)
	if r.Valid() {
		t.Error("expected invalid: not bool")
	}
}

func TestValidate_RealisticDocument(t *testing.T) {
	// The exact document format from settings validation tests
	src := []byte(`{
		"model": "claude-sonnet-4-6",
		"permissions": {
			"allow": ["Read", "Grep", "Glob"],
			"deny": ["Bash(rm -rf:*)"],
			"defaultMode": "acceptEdits"
		},
		"hooks": {
			"PreToolUse": [{
				"matcher": "Bash",
				"hooks": [{
					"type": "command",
					"command": "/usr/local/bin/audit"
				}]
			}]
		},
		"mcpServers": {
			"github": {
				"type": "stdio",
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-github"]
			}
		}
	}`)
	var doc map[string]interface{}
	if err := jsonc.Unmarshal(src, &doc); err != nil {
		t.Fatal(err)
	}
	r := jsonc.ValidateClaudeSettings(doc)
	if !r.Valid() {
		t.Errorf("expected valid, got: %v", r.Errors)
	}
}

func TestValidation_AddAndAddErr(t *testing.T) {
	r := &jsonc.ValidationResult{}
	r.Add("foo", "bad")
	if len(r.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(r.Errors))
	}
	r.AddErr(nil)
	if len(r.Errors) != 1 {
		t.Errorf("AddErr(nil) should not add")
	}
}

func TestErrValidation_Error(t *testing.T) {
	e := &jsonc.ErrValidation{Field: "model", Reason: "must be a non-empty string"}
	if !strings.Contains(e.Error(), "model") || !strings.Contains(e.Error(), "non-empty") {
		t.Errorf("unexpected error: %q", e.Error())
	}
}

func TestIsValidation(t *testing.T) {
	if !jsonc.IsValidation(&jsonc.ErrValidation{Field: "x", Reason: "y"}) {
		t.Error("expected IsValidation true for ErrValidation")
	}
	if jsonc.IsValidation(nil) {
		t.Error("expected IsValidation false for nil")
	}
}
