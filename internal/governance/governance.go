// Package governance implements a two-level, tightest-wins permission
// ceiling modeled on POLICY ∩ PROFILE. POLICY is an administrator-controlled
// ceiling loaded from a trust-root path; PROFILE is a per-session scope that
// can only narrow the ceiling. A tool is permitted only when both layers
// allow it, so an app or agent can never loosen the enterprise ceiling.
package governance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// DefaultProfileName is the profile applied when a session does not name one.
const DefaultProfileName = "default"

// ScopeName is a policy capability scope (e.g. "bash", "filesystem_write").
type ScopeName string

// Action is the decision for a single capability row.
type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
)

// ScopeCatalog maps every known scope to the tool families it governs. The
// evaluator is scope-name-agnostic: adding a capability is a data change in
// this catalog, never an evaluator edit.
var ScopeCatalog = map[ScopeName]string{
	"bash":              "Bash",
	"filesystem_read":   "Read,LS,Glob,Grep,SmartReader",
	"filesystem_write":  "Write,Edit,StructuredEdit,MultiEdit,FileEdit",
	"filesystem_delete": "Delete",
	"network":           "WebFetch,WebSearch,Browser,Screenshot,Download",
	"secrets":           "CoreMemoryRead,CoreMemorySearch",
	"spec":              "Proposal,Specify,Design,Plan,Tasks,Clarify,Constitution,Analyze,Checklist,Converge",
	"mcp":               "McpTool",
	"code_intel":        "CodeSearch,CodeGraph,Impact,GitHistory,LSP",
}

// toolToScope reverse-indexes the catalog for evaluation.
var toolToScope = buildToolToScope()

func buildToolToScope() map[string][]ScopeName {
	m := make(map[string][]ScopeName)
	for scope, list := range ScopeCatalog {
		for _, name := range strings.Split(list, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			m[name] = append(m[name], scope)
		}
	}
	return m
}

// ScopesForTool returns the capability scopes governing a tool name.
func ScopesForTool(toolName string) []ScopeName {
	scopes := toolToScope[strings.TrimSpace(toolName)]
	sorted := make([]ScopeName, len(scopes))
	copy(sorted, scopes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted
}

// Capability is a single allow/deny row in a policy document.
type Capability struct {
	Scope   ScopeName `json:"scope"`
	Action  Action    `json:"action"`
	Pattern string    `json:"pattern,omitempty"` // glob over the tool summary; empty = all calls
	Reason  string    `json:"reason,omitempty"`
}

// Document is the serialized form of a POLICY or PROFILE.
type Document struct {
	Version        int                    `json:"version"`
	FailClosed     bool                   `json:"fail_closed,omitempty"` // no capabilities => deny everything
	Capabilities   []Capability           `json:"capabilities"`
	DeniedTools    []string               `json:"denied_tools,omitempty"` // flat tool-name deny list
	DeniedBash     []string               `json:"denied_bash,omitempty"`  // bash command glob patterns
	SensitivePaths []string               `json:"sensitive_paths,omitempty"`
	Extra          map[string]interface{} `json:"-"`
}

// Layer is a loaded policy or profile with its parsed capabilities indexed by
// scope for fast evaluation.
type Layer struct {
	Name           string
	FailClosed     bool
	Capabilities   []Capability
	DeniedTools    map[string]struct{}
	DeniedBash     []string
	SensitivePaths []string
}

// Decision is the result of evaluating a request against the effective policy.
type Decision struct {
	Allowed bool
	Source  string // "policy" or "profile"
	Scope   ScopeName
	Rule    string
	Reason  string
}

// Engine evaluates tool requests against the composed POLICY ∩ PROFILE.
type Engine struct {
	mu      sync.RWMutex
	policy  *Layer
	profile *Layer
}

// New returns an empty engine (fail-open until LoadPolicy is called).
func New() *Engine {
	return &Engine{}
}

// LoadPolicy loads the admin ceiling. It is a fatal configuration error to
// ship a malformed policy, so a parse failure returns an error rather than
// silently falling open.
func (e *Engine) LoadPolicy(path string) error {
	layer, err := loadLayer("policy", path)
	if err != nil {
		return err
	}
	e.SetPolicy(layer)
	return nil
}

// SetPolicy installs an in-memory policy ceiling. Exported so embedding
// hosts and tests can install a policy without touching the filesystem.
func (e *Engine) SetPolicy(layer *Layer) {
	e.mu.Lock()
	e.policy = layer
	e.mu.Unlock()
}

// SetProfile replaces the session's narrow scope.
func (e *Engine) SetProfile(layer *Layer) {
	e.mu.Lock()
	e.profile = layer
	e.mu.Unlock()
}

// Profile returns the active profile layer (nil if none).
func (e *Engine) Profile() *Layer {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.profile
}

// loadLayer reads and parses a policy or profile document.
func loadLayer(name, path string) (*Layer, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is the configured trust-root policy file, not external input
	if err != nil {
		return nil, fmt.Errorf("governance: load %s: %w", name, err)
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("governance: parse %s %s: %w", name, path, err)
	}
	return buildLayer(name, doc)
}

// BuildProfile constructs a profile layer from a document in memory.
func BuildProfile(name string, doc Document) (*Layer, error) {
	return buildLayer(name, doc)
}

// LoadLayer loads and parses a policy or profile document from disk. The name
// is used only for error messages; the layer's behavior is identical for
// policy and profile documents.
func LoadLayer(name, path string) (*Layer, error) {
	return loadLayer(name, path)
}

func buildLayer(name string, doc Document) (*Layer, error) {
	if doc.Version != 1 {
		return nil, fmt.Errorf("governance: unsupported version %d", doc.Version)
	}
	l := &Layer{
		Name:           name,
		FailClosed:     doc.FailClosed,
		Capabilities:   append([]Capability(nil), doc.Capabilities...),
		DeniedTools:    make(map[string]struct{}),
		DeniedBash:     append([]string(nil), doc.DeniedBash...),
		SensitivePaths: append([]string(nil), doc.SensitivePaths...),
	}
	for _, t := range doc.DeniedTools {
		l.DeniedTools[strings.TrimSpace(t)] = struct{}{}
	}
	return l, nil
}

// Evaluate checks a tool call. summary is the one-line tool summary used to
// match pattern rows (e.g. the bash command text). The tightest-wins rule is:
// any deny in either layer denies; otherwise any allow permits; otherwise the
// request is denied when a layer is fail-closed, else not governed.
func (e *Engine) Evaluate(toolName, summary string) Decision {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.policy == nil {
		return Decision{Allowed: true, Source: "unconfigured", Reason: "no governance policy loaded"}
	}

	// Flat deny list at the policy ceiling is absolute.
	if _, denied := e.policy.DeniedTools[strings.TrimSpace(toolName)]; denied {
		return Decision{Allowed: false, Source: "policy", Rule: "denied_tools", Reason: "tool denied by policy ceiling"}
	}

	scopes := ScopesForTool(toolName)

	// POLICY layer: deny wins; allow passes the layer only when matched.
	policyDeny, policyAllow, governedByPolicy := evaluateLayer(e.policy, toolName, summary, scopes)
	if policyDeny {
		return Decision{Allowed: false, Source: "policy", Scope: policyAllow.Scope, Rule: "capability", Reason: "denied by policy ceiling"}
	}

	// PROFILE layer: may only narrow. Deny in the profile denies; allow or
	// silence does not expand beyond what the policy already permitted.
	var profileDeny bool
	var profileAllow Decision
	if e.profile != nil {
		profileDeny, profileAllow, _ = evaluateLayer(e.profile, toolName, summary, scopes)
		if profileDeny {
			return Decision{Allowed: false, Source: "profile", Scope: profileAllow.Scope, Rule: "capability", Reason: "denied by session profile"}
		}
	}

	// If the policy explicitly allowed, the profile did not deny: allow.
	if governedByPolicy && policyAllow.Allowed {
		return Decision{Allowed: true, Source: "policy", Scope: policyAllow.Scope, Rule: "capability", Reason: "allowed by policy"}
	}

	// Otherwise a fail-closed policy denies ungoverned calls; an open policy
	// leaves the decision to the normal permission pipeline.
	if e.policy.FailClosed {
		return Decision{Allowed: false, Source: "policy", Reason: "fail_closed", Rule: "not_governed"}
	}
	return Decision{Allowed: true, Source: "ungoverned", Reason: "no matching capability"}
}

// evaluateLayer returns (denyHit, allowHit, governed). denyHit means a deny
// capability matched; allowHit carries the allow Decision when one matched.
func evaluateLayer(l *Layer, toolName, summary string, scopes []ScopeName) (bool, Decision, bool) {
	denyHit := false
	var allowHit Decision
	for _, cap := range l.Capabilities {
		if !scopeContains(cap.Scope, scopes) {
			continue
		}
		if cap.Pattern != "" && !globMatch(cap.Pattern, summary) {
			continue
		}
		if cap.Action == ActionDeny {
			denyHit = true
		} else {
			allowHit = Decision{Allowed: true, Scope: cap.Scope, Rule: "capability"}
		}
	}
	// Sensitive-path protection is a deny-only floor at every layer.
	if matchesAnyPattern(summary, append(l.SensitivePaths, l.DeniedBash...)) {
		denyHit = true
	}
	// A layer that names the scope with an allow grants; a layer with no
	// capability rows for the scope leaves it ungoverned unless fail-closed.
	return denyHit, allowHit, len(l.Capabilities) > 0 || len(l.DeniedTools) > 0
}

func scopeContains(scope ScopeName, scopes []ScopeName) bool {
	for _, s := range scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func matchesAnyPattern(summary string, patterns []string) bool {
	for _, p := range patterns {
		if globMatch(p, summary) {
			return true
		}
	}
	return false
}

// globMatch reports whether pattern matches subject with * wildcard and
// prefix matching for trailing *. For path patterns it also matches against
// every suffix of the subject that starts at a path separator, so a pattern
// like ".ssh/*" matches "~/.ssh/config".
func globMatch(pattern, subject string) bool {
	if pattern == "" {
		return true
	}
	norm := strings.ReplaceAll(subject, "~", homePath())
	if matched, err := filepath.Match(pattern, norm); err == nil && matched {
		return true
	}
	if matched, err := filepath.Match(pattern, subject); err == nil && matched {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		if strings.HasPrefix(norm, prefix) || strings.HasPrefix(subject, prefix) {
			return true
		}
	}
	sep := string(filepath.Separator)
	for i := 0; i < len(subject); i++ {
		if string(subject[i]) == sep || subject[i] == '/' {
			if matched, err := filepath.Match(pattern, subject[i+1:]); err == nil && matched {
				return true
			}
		}
	}
	return false
}

func homePath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "~"
	}
	return home
}

// defaultManagedPaths returns the platform-specific trust-root locations for
// the governance policy, mirroring the IT-managed rule locations. These are
// writable only by administrators, which is what makes the ceiling
// un-disableable from inside the agent.
func defaultManagedPaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/Library/Application Support/Graycode/security_policy.json"}
	default:
		return []string{"/etc/graycode/security_policy.json"}
	}
}

// ManagedPolicyPath resolves the trust-root policy file for the platform.
func ManagedPolicyPath() string {
	paths := defaultManagedPaths()
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}
