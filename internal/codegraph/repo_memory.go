package codegraph

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// RepoMemory integrates codegraph with harrier (persistent memory) to learn
// from past issues, fixes, and code patterns. This implements the research
// finding that repository memory improves localization by 13%.
type RepoMemory struct {
	store MemoryStore
}

// MemoryStore is the interface for persistent storage.
type MemoryStore interface {
	Save(key string, value []byte) error
	Load(key string) ([]byte, error)
	List(prefix string) ([]string, error)
	Delete(key string) error
}

// IssueMemory stores what was learned from a resolved issue.
type IssueMemory struct {
	IssueID      string    `json:"issue_id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	RootCause    string    `json:"root_cause"`
	FixPattern   string    `json:"fix_pattern"` // pattern that fixed it
	FilesChanged []string  `json:"files_changed"`
	SymbolsUsed  []string  `json:"symbols_used"` // symbols involved in the fix
	Approach     string    `json:"approach"`     // approach taken
	Success      bool      `json:"success"`
	Duration     string    `json:"duration"`
	CreatedAt    time.Time `json:"created_at"`
	Tags         []string  `json:"tags"` // "bug", "feature", "refactor", "security"
}

// CodePattern stores a learned code pattern.
type CodePattern struct {
	PatternID   string   `json:"pattern_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Example     string   `json:"example"`     // code example
	WhenToUse   string   `json:"when_to_use"` // when to apply this pattern
	Files       []string `json:"files"`       // files where pattern was found
	Frequency   int      `json:"frequency"`   // how often seen
	Tags        []string `json:"tags"`
}

// NewRepoMemory creates a new repository memory system.
func NewRepoMemory(store MemoryStore) *RepoMemory {
	return &RepoMemory{store: store}
}

// SaveIssue records what was learned from resolving an issue.
func (rm *RepoMemory) SaveIssue(mem IssueMemory) error {
	mem.CreatedAt = time.Now()

	key := fmt.Sprintf("issue:%s", hashKey(mem.IssueID))
	data, err := json.Marshal(mem)
	if err != nil {
		return err
	}

	return rm.store.Save(key, data)
}

// FindSimilarIssues finds past issues similar to the given description.
func (rm *RepoMemory) FindSimilarIssues(description string, limit int) ([]IssueMemory, error) {
	keys, err := rm.store.List("issue:")
	if err != nil {
		return nil, err
	}

	var results []IssueMemory
	for _, key := range keys {
		data, err := rm.store.Load(key)
		if err != nil {
			continue
		}

		var mem IssueMemory
		if err := json.Unmarshal(data, &mem); err != nil {
			continue
		}

		// Simple similarity: check for shared keywords
		if isSimilar(description, mem.Title+" "+mem.Description) {
			results = append(results, mem)
			if len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// SavePattern records a learned code pattern.
func (rm *RepoMemory) SavePattern(pattern CodePattern) error {
	key := fmt.Sprintf("pattern:%s", hashKey(pattern.PatternID))
	data, err := json.Marshal(pattern)
	if err != nil {
		return err
	}

	return rm.store.Save(key, data)
}

// FindRelevantPatterns finds patterns relevant to the given context.
func (rm *RepoMemory) FindRelevantPatterns(context string, limit int) ([]CodePattern, error) {
	keys, err := rm.store.List("pattern:")
	if err != nil {
		return nil, err
	}

	var results []CodePattern
	for _, key := range keys {
		data, err := rm.store.Load(key)
		if err != nil {
			continue
		}

		var pattern CodePattern
		if err := json.Unmarshal(data, &pattern); err != nil {
			continue
		}

		if isSimilar(context, pattern.Name+" "+pattern.Description+" "+pattern.WhenToUse) {
			results = append(results, pattern)
			if len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// BuildContextFromMemory builds context from past issues and patterns.
func (rm *RepoMemory) BuildContextFromMemory(query string) string {
	var ctx string

	// Find similar past issues
	issues, _ := rm.FindSimilarIssues(query, 3)
	if len(issues) > 0 {
		ctx += "## Similar Past Issues\n\n"
		for _, issue := range issues {
			ctx += fmt.Sprintf("- **%s**: %s\n  Root cause: %s\n  Fix: %s\n\n",
				issue.IssueID, issue.Title, issue.RootCause, issue.FixPattern)
		}
	}

	// Find relevant patterns
	patterns, _ := rm.FindRelevantPatterns(query, 3)
	if len(patterns) > 0 {
		ctx += "## Relevant Code Patterns\n\n"
		for _, p := range patterns {
			ctx += fmt.Sprintf("- **%s**: %s\n  When: %s\n\n",
				p.Name, p.Description, p.WhenToUse)
		}
	}

	return ctx
}

// ExtractPatternsFromCode analyzes code to extract recurring patterns.
func ExtractPatternsFromCode(nodes []Node) []CodePattern {
	patterns := make(map[string]*CodePattern)

	for _, node := range nodes {
		// Pattern: Error handling
		if node.Kind == "function" && containsString(node.Name, "Error") {
			pattern := patterns["error_handling"]
			if pattern == nil {
				pattern = &CodePattern{
					PatternID:   "error_handling",
					Name:        "Error Handling Pattern",
					Description: "Functions that handle errors",
					WhenToUse:   "When processing operations that can fail",
					Tags:        []string{"error", "handling"},
				}
				patterns["error_handling"] = pattern
			}
			pattern.Files = append(pattern.Files, node.FilePath)
			pattern.Frequency++
		}

		// Pattern: Factory method
		if node.Kind == "function" && containsString(node.Name, "New") {
			pattern := patterns["factory"]
			if pattern == nil {
				pattern = &CodePattern{
					PatternID:   "factory",
					Name:        "Factory Pattern",
					Description: "Constructor/factory functions",
					WhenToUse:   "When creating new instances of types",
					Tags:        []string{"creation", "factory"},
				}
				patterns["factory"] = pattern
			}
			pattern.Files = append(pattern.Files, node.FilePath)
			pattern.Frequency++
		}

		// Pattern: Interface implementation
		if node.Kind == "interface" {
			pattern := patterns["interface"]
			if pattern == nil {
				pattern = &CodePattern{
					PatternID:   "interface",
					Name:        "Interface Pattern",
					Description: "Interface definitions for abstraction",
					WhenToUse:   "When defining contracts between components",
					Tags:        []string{"abstraction", "interface"},
				}
				patterns["interface"] = pattern
			}
			pattern.Files = append(pattern.Files, node.FilePath)
			pattern.Frequency++
		}
	}

	result := make([]CodePattern, 0, len(patterns))
	for _, p := range patterns {
		result = append(result, *p)
	}
	return result
}

// Helper functions

func hashKey(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8])
}

func isSimilar(a, b string) bool {
	aWords := tokenizeWords(a)
	bWords := tokenizeWords(b)

	matches := 0
	for _, aw := range aWords {
		for _, bw := range bWords {
			if aw == bw {
				matches++
				break
			}
		}
	}

	// At least 2 shared words
	return matches >= 2
}

func tokenizeWords(s string) []string {
	var words []string
	var current []byte
	for _, b := range []byte(s) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') {
			current = append(current, b)
		} else {
			if len(current) > 2 {
				words = append(words, toLower(string(current)))
			}
			current = nil
		}
	}
	if len(current) > 2 {
		words = append(words, toLower(string(current)))
	}
	return words
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i, c := range []byte(s) {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		} else {
			b[i] = c
		}
	}
	return string(b)
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && len(sub) > 0 && findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
