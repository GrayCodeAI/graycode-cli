package sandbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApprovalStore_AddAndCheck(t *testing.T) {
	dir := t.TempDir()
	store := NewApprovalStore(filepath.Join(dir, "grants.json"))

	store.AddGrant(TypedGrant{
		Action: GrantAllow,
		Class:  ClassBash,
		Target: "git *",
		Scope:  "project",
	})

	action, found := store.Check(ClassBash, "git status")
	if !found || action != GrantAllow {
		t.Errorf("expected allow for 'git status', got %v/%v", action, found)
	}

	action, found = store.Check(ClassBash, "rm -rf /")
	if found {
		t.Errorf("expected no match for 'rm -rf /', got %v", action)
	}
}

func TestApprovalStore_RemoveGrant(t *testing.T) {
	dir := t.TempDir()
	store := NewApprovalStore(filepath.Join(dir, "grants.json"))

	store.AddGrant(TypedGrant{Action: GrantAllow, Class: ClassBash, Target: "git *"})
	store.RemoveGrant(ClassBash, "git *")

	_, found := store.Check(ClassBash, "git status")
	if found {
		t.Error("expected grant to be removed")
	}
}

func TestApprovalStore_ExpiredGrant(t *testing.T) {
	dir := t.TempDir()
	store := NewApprovalStore(filepath.Join(dir, "grants.json"))

	past := time.Now().Add(-time.Hour)
	store.AddGrant(TypedGrant{
		Action:  GrantAllow,
		Class:   ClassWrite,
		Target:  "/tmp/*",
		Expires: &past,
	})

	_, found := store.Check(ClassWrite, "/tmp/test.txt")
	if found {
		t.Error("expired grant should not match")
	}
}

func TestApprovalStore_CleanupExpired(t *testing.T) {
	dir := t.TempDir()
	store := NewApprovalStore(filepath.Join(dir, "grants.json"))

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	store.AddGrant(TypedGrant{Action: GrantAllow, Class: ClassRead, Target: "/a", Expires: &past})
	store.AddGrant(TypedGrant{Action: GrantAllow, Class: ClassRead, Target: "/b", Expires: &future})
	store.AddGrant(TypedGrant{Action: GrantAllow, Class: ClassRead, Target: "/c"})

	removed := store.CleanupExpired()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	if len(store.Grants()) != 2 {
		t.Errorf("expected 2 remaining, got %d", len(store.Grants()))
	}
}

func TestApprovalStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grants.json")

	store1 := NewApprovalStore(path)
	store1.AddGrant(TypedGrant{Action: GrantDeny, Class: ClassWrite, Target: "/etc/*"})

	store2 := NewApprovalStore(path)
	action, found := store2.Check(ClassWrite, "/etc/passwd")
	if !found || action != GrantDeny {
		t.Errorf("expected persisted deny, got %v/%v", action, found)
	}
}

func TestPolicyManager_CheckTool_DefaultAllow(t *testing.T) {
	dir := t.TempDir()
	m := NewPolicyManager(dir)

	decision, shouldPrompt := m.CheckTool(ClassBash, "ls -la")
	if decision != DecisionAllow {
		t.Errorf("expected allow, got %v", decision)
	}
	if shouldPrompt {
		t.Error("should not prompt with default allow")
	}
}

func TestPolicyManager_CheckTool_DefaultAsk(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hawk"), 0755)
	os.WriteFile(filepath.Join(dir, ".hawk", "sandbox.jsonc"), []byte(`{"default": "ask"}`), 0644)

	m := NewPolicyManager(dir)

	decision, shouldPrompt := m.CheckTool(ClassBash, "ls")
	if decision != DecisionAsk {
		t.Errorf("expected ask, got %v", decision)
	}
	if !shouldPrompt {
		t.Error("should prompt with default ask")
	}
}

func TestPolicyManager_CheckTool_PolicyRules(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hawk"), 0755)
	policy := `{
		"default": "ask",
		"rules": [
			{"class": "bash", "pattern": "git *", "action": "allow"},
			{"class": "write", "pattern": "/etc/*", "action": "deny"}
		]
	}`
	os.WriteFile(filepath.Join(dir, ".hawk", "sandbox.jsonc"), []byte(policy), 0644)

	m := NewPolicyManager(dir)

	decision, _ := m.CheckTool(ClassBash, "git push")
	if decision != DecisionAllow {
		t.Errorf("expected allow for git push, got %v", decision)
	}

	decision, _ = m.CheckTool(ClassWrite, "/etc/passwd")
	if decision != DecisionDeny {
		t.Errorf("expected deny for /etc/passwd, got %v", decision)
	}

	decision, shouldPrompt := m.CheckTool(ClassBash, "make build")
	if decision != DecisionAsk || !shouldPrompt {
		t.Errorf("expected ask for unmatched, got %v/%v", decision, shouldPrompt)
	}
}

func TestPolicyManager_AllowDeny(t *testing.T) {
	dir := t.TempDir()
	m := NewPolicyManager(dir)

	m.Allow(ClassBash, "npm *")
	decision, _ := m.CheckTool(ClassBash, "npm install")
	if decision != DecisionAllow {
		t.Errorf("expected allow after Allow(), got %v", decision)
	}

	m.Deny(ClassBash, "npm *")
	decision, _ = m.CheckTool(ClassBash, "npm install")
	if decision != DecisionDeny {
		t.Errorf("expected deny after Deny(), got %v", decision)
	}
}

func TestMatchTarget(t *testing.T) {
	tests := []struct {
		pattern, target string
		want            bool
	}{
		{"*", "anything", true},
		{"git *", "git status", true},
		{"git *", "git push origin", true},
		{"git *", "npm install", false},
		{"*.go", "main.go", true},
		{"*.go", "main.ts", false},
		{"", "anything", true},
	}
	for _, tt := range tests {
		if got := matchTarget(tt.pattern, tt.target); got != tt.want {
			t.Errorf("matchTarget(%q, %q) = %v, want %v", tt.pattern, tt.target, got, tt.want)
		}
	}
}

func TestStripJSONComments(t *testing.T) {
	input := `{
		// This is a comment
		"key": "value", /* block
		comment */ "key2": "value2"
	}`
	got := stripJSONComments(input)
	// Should strip comments but keep valid JSON structure
	if !containsStr(got, `"key": "value"`) {
		t.Errorf("expected key/value preserved, got: %q", got)
	}
	if !containsStr(got, `"key2": "value2"`) {
		t.Errorf("expected key2/value2 preserved, got: %q", got)
	}
	if containsStr(got, "This is a comment") {
		t.Error("line comment should be stripped")
	}
	if containsStr(got, "block") {
		t.Error("block comment should be stripped")
	}
}

// containsStr is defined in container_test.go
