package sandbox

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// GrantAction represents the action of a grant.
type GrantAction string

const (
	GrantAllow GrantAction = "allow"
	GrantDeny  GrantAction = "deny"
)

// GrantClass represents the type of operation being granted.
type GrantClass string

const (
	ClassBash  GrantClass = "bash"
	ClassRead  GrantClass = "read"
	ClassWrite GrantClass = "write"
	ClassEdit  GrantClass = "edit"
)

// TypedGrant is a policy grant for a specific class and target.
type TypedGrant struct {
	Action  GrantAction `json:"action"`
	Class   GrantClass  `json:"class"`
	Target  string      `json:"target"`  // path pattern or command prefix
	Scope   string      `json:"scope"`   // "project" | "global"
	Expires *time.Time  `json:"expires,omitempty"`
}

// ApprovalStore persists typed grants to a JSON file.
type ApprovalStore struct {
	mu       sync.Mutex
	grants   []TypedGrant
	filePath string
}

// NewApprovalStore creates an approval store backed by the given file.
func NewApprovalStore(filePath string) *ApprovalStore {
	s := &ApprovalStore{
		filePath: filePath,
	}
	s.load()
	return s
}

// NewProjectApprovalStore creates an approval store for a project directory.
func NewProjectApprovalStore(projectDir string) *ApprovalStore {
	return NewApprovalStore(filepath.Join(projectDir, ".hawk", "sandbox.grants.jsonc"))
}

// NewGlobalApprovalStore creates an approval store for the user's home directory.
func NewGlobalApprovalStore() *ApprovalStore {
	home, _ := os.UserHomeDir()
	return NewApprovalStore(filepath.Join(home, ".hawk", "sandbox.grants.jsonc"))
}

func (s *ApprovalStore) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	// Strip JSONC comments
	cleaned := stripJSONComments(string(data))
	if err := json.Unmarshal([]byte(cleaned), &s.grants); err != nil {
		slog.Warn("sandbox: failed to parse grants", "path", s.filePath, "error", err)
	}
}

func (s *ApprovalStore) save() error {
	if s.filePath == "" {
		return nil
	}
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.grants, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0o644)
}

// Check evaluates whether a tool call is allowed, denied, or unknown.
// Returns: action (allow/deny), found (true if a matching grant exists).
func (s *ApprovalStore) Check(class GrantClass, target string) (GrantAction, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, grant := range s.grants {
		if grant.Expires != nil && grant.Expires.Before(now) {
			continue // expired
		}
		if grant.Class != class {
			continue
		}
		if matchTarget(grant.Target, target) {
			return grant.Action, true
		}
	}
	return "", false
}

// AddGrant adds a new grant to the store and persists it.
func (s *ApprovalStore) AddGrant(grant TypedGrant) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing grant for same class+target
	filtered := make([]TypedGrant, 0, len(s.grants))
	for _, g := range s.grants {
		if g.Class != grant.Class || g.Target != grant.Target {
			filtered = append(filtered, g)
		}
	}
	filtered = append(filtered, grant)
	s.grants = filtered

	return s.save()
}

// RemoveGrant removes a grant by class and target.
func (s *ApprovalStore) RemoveGrant(class GrantClass, target string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]TypedGrant, 0, len(s.grants))
	for _, g := range s.grants {
		if g.Class == class && g.Target == target {
			continue
		}
		filtered = append(filtered, g)
	}
	s.grants = filtered
	return s.save()
}

// Grants returns a snapshot of all grants.
func (s *ApprovalStore) Grants() []TypedGrant {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]TypedGrant, len(s.grants))
	copy(result, s.grants)
	return result
}

// CleanupExpired removes expired grants and persists the change.
func (s *ApprovalStore) CleanupExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	filtered := make([]TypedGrant, 0, len(s.grants))
	removed := 0
	for _, g := range s.grants {
		if g.Expires != nil && g.Expires.Before(now) {
			removed++
			continue
		}
		filtered = append(filtered, g)
	}
	s.grants = filtered
	if removed > 0 {
		_ = s.save()
	}
	return removed
}

func matchTarget(pattern, target string) bool {
	if pattern == "*" || pattern == "" {
		return true
	}
	// Prefix match for commands
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(target, strings.TrimSuffix(pattern, "*"))
	}
	// Glob match for paths
	matched, _ := filepath.Match(pattern, target)
	if matched {
		return true
	}
	// Try matching against basename
	matched, _ = filepath.Match(pattern, filepath.Base(target))
	return matched
}

func stripJSONComments(s string) string {
	var result []byte
	inString := false
	inComment := false
	inLineComment := false
	prev := byte(0)

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inLineComment {
			if c == '\n' {
				inLineComment = false
				result = append(result, c)
			}
			continue
		}
		if inComment {
			if c == '/' && prev == '*' {
				inComment = false
				prev = 0
				continue
			}
			prev = c
			continue
		}
		if c == '"' && prev != '\\' {
			inString = !inString
		}
		if !inString && c == '/' && i+1 < len(s) {
			if s[i+1] == '/' {
				inLineComment = true
				continue
			}
			if s[i+1] == '*' {
				inComment = true
				prev = 0
				continue
			}
		}
		result = append(result, c)
		prev = c
	}
	return string(result)
}
