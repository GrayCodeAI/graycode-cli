package sandbox

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// PolicyDecision represents the outcome of a policy check.
type PolicyDecision string

const (
	DecisionAllow PolicyDecision = "allow"
	DecisionDeny  PolicyDecision = "deny"
	DecisionAsk   PolicyDecision = "ask"
)

// PolicyRule defines a single policy rule.
type PolicyRule struct {
	Class   string `json:"class"`   // "bash", "read", "write", "edit"
	Pattern string `json:"pattern"` // glob or prefix pattern
	Action  string `json:"action"`  // "allow", "deny", "ask"
}

// PolicyConfig defines the sandbox policy configuration.
type PolicyConfig struct {
	Default PolicyDecision `json:"default"`
	Rules   []PolicyRule   `json:"rules"`
}

// PolicyManager adds policy-aware tool interception on top of the sandbox.
type PolicyManager struct {
	mu            sync.RWMutex
	projectDir    string
	policy        *PolicyConfig
	projectGrants *ApprovalStore
	globalGrants  *ApprovalStore
}

// NewPolicyManager creates a policy manager for the given project.
// The default posture is deny-by-default: only explicit rules, grants, or a
// configured default in the policy file allow an action.
func NewPolicyManager(projectDir string) *PolicyManager {
	m := &PolicyManager{
		projectDir:    projectDir,
		policy:        &PolicyConfig{Default: DecisionDeny},
		projectGrants: NewProjectApprovalStore(projectDir),
		globalGrants:  NewGlobalApprovalStore(),
	}
	m.loadPolicy()
	return m
}

func (m *PolicyManager) loadPolicy() {
	// Load project policy (sets the default and rules explicitly).
	projectPolicy := &PolicyConfig{}
	loadPolicyFile(filepath.Join(m.projectDir, ".agents", "sandbox.jsonc"), projectPolicy)
	// Load global policy (fills gaps only; project takes precedence).
	globalPolicy := &PolicyConfig{}
	loadPolicyFile(filepath.Join(storage.StateDir(), "sandbox.jsonc"), globalPolicy)
	m.policy = &PolicyConfig{Default: DecisionDeny}
	if projectPolicy.Default != "" {
		m.policy.Default = projectPolicy.Default
	}
	m.policy.Rules = projectPolicy.Rules
	if len(m.policy.Rules) == 0 {
		m.policy.Rules = globalPolicy.Rules
	}
	if projectPolicy.Default == "" && globalPolicy.Default != "" {
		m.policy.Default = globalPolicy.Default
	}
}

func loadPolicyFile(path string, cfg *PolicyConfig) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is either the project's .agents/sandbox.jsonc or the global state dir, both trusted internal locations
	if err != nil {
		return
	}
	cleaned := stripJSONComments(string(data))
	var loaded PolicyConfig
	if err := json.Unmarshal([]byte(cleaned), &loaded); err != nil {
		slog.Warn("sandbox: failed to parse policy", "path", path, "error", err)
		return
	}
	if loaded.Default != "" {
		cfg.Default = loaded.Default
	}
	if len(loaded.Rules) > 0 {
		cfg.Rules = loaded.Rules
	}
}

// CheckTool evaluates a tool call against the policy and approval grants.
// Returns the decision and whether the caller should prompt the user.
func (m *PolicyManager) CheckTool(class GrantClass, target string) (PolicyDecision, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 1. Check approval grants (project first, then global)
	if action, found := m.projectGrants.Check(class, target); found {
		return PolicyDecision(action), false
	}
	if action, found := m.globalGrants.Check(class, target); found {
		return PolicyDecision(action), false
	}

	// 2. Check policy rules (first match wins)
	for _, rule := range m.policy.Rules {
		if rule.Class != string(class) {
			continue
		}
		if matchTarget(rule.Pattern, target) {
			return PolicyDecision(rule.Action), false
		}
	}

	// 3. Fall back to default
	if m.policy.Default == DecisionAsk {
		return DecisionAsk, true
	}
	return m.policy.Default, false
}

// Allow persists an allow grant for the given class and target.
func (m *PolicyManager) Allow(class GrantClass, target string) error {
	return m.projectGrants.AddGrant(TypedGrant{
		Action: GrantAllow,
		Class:  class,
		Target: target,
		Scope:  "project",
	})
}

// Deny persists a deny grant for the given class and target.
func (m *PolicyManager) Deny(class GrantClass, target string) error {
	return m.projectGrants.AddGrant(TypedGrant{
		Action: GrantDeny,
		Class:  class,
		Target: target,
		Scope:  "project",
	})
}

// Policy returns the current policy config.
func (m *PolicyManager) Policy() PolicyConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.policy
}

// ReloadPolicy re-reads policy files from disk.
func (m *PolicyManager) ReloadPolicy() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadPolicy()
}
