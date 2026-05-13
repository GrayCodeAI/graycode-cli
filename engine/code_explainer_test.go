package engine

import (
	"strings"
	"testing"
)

func TestNewCodeExplainer(t *testing.T) {
	ce := NewCodeExplainer()
	if ce == nil {
		t.Fatal("NewCodeExplainer returned nil")
	}
}

func TestExplainFunction_Basic(t *testing.T) {
	ce := NewCodeExplainer()
	src := `package example

import "fmt"

// ValidateToken checks whether a JWT token is valid and returns the parsed claims.
func ValidateToken(token string) (*Claims, error) {
	if token == "" {
		return nil, fmt.Errorf("empty token: %w", ErrInvalid)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed token: %w", ErrInvalid)
	}
	claims, err := parseClaims(parts[1])
	if err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}
	return claims, nil
}
`
	exp, err := ce.ExplainFunction("token.go", src, "ValidateToken")
	if err != nil {
		t.Fatalf("ExplainFunction failed: %v", err)
	}

	if exp.File != "token.go" {
		t.Errorf("expected file token.go, got %s", exp.File)
	}
	if exp.Symbol != "ValidateToken" {
		t.Errorf("expected symbol ValidateToken, got %s", exp.Symbol)
	}
	if exp.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if len(exp.Sections) == 0 {
		t.Error("expected non-empty sections")
	}

	// Check for expected sections
	sectionTitles := make(map[string]bool)
	for _, s := range exp.Sections {
		sectionTitles[s.Title] = true
	}
	for _, expected := range []string{"Purpose", "Parameters", "Returns", "Control Flow", "Error Handling", "Side Effects"} {
		if !sectionTitles[expected] {
			t.Errorf("missing section %q", expected)
		}
	}

	// Check complexity
	if !strings.Contains(exp.Complexity, "CC:") {
		t.Errorf("expected complexity with CC, got %q", exp.Complexity)
	}
}

func TestExplainFunction_NotFound(t *testing.T) {
	ce := NewCodeExplainer()
	src := `package example

func Foo() {}
`
	_, err := ce.ExplainFunction("test.go", src, "Bar")
	if err == nil {
		t.Fatal("expected error for missing function")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}

func TestExplainFunction_ParseError(t *testing.T) {
	ce := NewCodeExplainer()
	_, err := ce.ExplainFunction("bad.go", "not valid go code {{{{", "Foo")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestExplainFunction_WithSideEffects(t *testing.T) {
	ce := NewCodeExplainer()
	src := `package example

import (
	"os"
	"net/http"
)

func FetchAndSave(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
`
	exp, err := ce.ExplainFunction("io.go", src, "FetchAndSave")
	if err != nil {
		t.Fatalf("ExplainFunction failed: %v", err)
	}

	// Find side effects section
	var sideEffects string
	for _, s := range exp.Sections {
		if s.Title == "Side Effects" {
			sideEffects = s.Content
			break
		}
	}
	if sideEffects == "None (pure function)" {
		t.Error("expected side effects to be detected")
	}
	if !strings.Contains(sideEffects, "Network") && !strings.Contains(sideEffects, "File") {
		t.Errorf("expected network or file side effects, got %q", sideEffects)
	}
}

func TestExplainType_Struct(t *testing.T) {
	ce := NewCodeExplainer()
	src := `package example

import "sync"

// TokenCache stores validated tokens for quick lookup.
type TokenCache struct {
	mu      sync.RWMutex
	tokens  map[string]*Token
	maxSize int
}

func NewTokenCache(maxSize int) *TokenCache {
	return &TokenCache{
		tokens:  make(map[string]*Token),
		maxSize: maxSize,
	}
}

func (tc *TokenCache) Get(key string) (*Token, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	t, ok := tc.tokens[key]
	return t, ok
}

func (tc *TokenCache) Set(key string, token *Token) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.tokens[key] = token
}
`
	exp, err := ce.ExplainType("cache.go", src, "TokenCache")
	if err != nil {
		t.Fatalf("ExplainType failed: %v", err)
	}

	if exp.Symbol != "TokenCache" {
		t.Errorf("expected symbol TokenCache, got %s", exp.Symbol)
	}
	if exp.Summary == "" {
		t.Error("expected non-empty summary")
	}

	sectionTitles := make(map[string]bool)
	for _, s := range exp.Sections {
		sectionTitles[s.Title] = true
	}

	if !sectionTitles["Purpose"] {
		t.Error("missing Purpose section")
	}
	if !sectionTitles["Fields"] {
		t.Error("missing Fields section")
	}
	if !sectionTitles["Methods"] {
		t.Error("missing Methods section")
	}
	if !sectionTitles["Constructor"] {
		t.Error("missing Constructor section")
	}
}

func TestExplainType_Interface(t *testing.T) {
	ce := NewCodeExplainer()
	src := `package example

// Validator checks whether a value meets certain criteria.
type Validator interface {
	Validate(value interface{}) error
	Name() string
}
`
	exp, err := ce.ExplainType("iface.go", src, "Validator")
	if err != nil {
		t.Fatalf("ExplainType failed: %v", err)
	}

	if exp.Symbol != "Validator" {
		t.Errorf("expected symbol Validator, got %s", exp.Symbol)
	}

	var methodsSection string
	for _, s := range exp.Sections {
		if s.Title == "Methods" {
			methodsSection = s.Content
			break
		}
	}
	if !strings.Contains(methodsSection, "Validate") {
		t.Errorf("expected Validate in methods, got %q", methodsSection)
	}
	if !strings.Contains(methodsSection, "Name") {
		t.Errorf("expected Name in methods, got %q", methodsSection)
	}
}

func TestExplainType_NotFound(t *testing.T) {
	ce := NewCodeExplainer()
	src := `package example

type Foo struct{}
`
	_, err := ce.ExplainType("test.go", src, "Bar")
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestExplainFile(t *testing.T) {
	ce := NewCodeExplainer()
	src := `// Package auth provides JWT token validation and session management.
package auth

import (
	"context"
	"sync"
	"time"
)

type TokenManager struct {
	mu      sync.Mutex
	store   TokenStore
	timeout time.Duration
}

type TokenStore interface {
	Get(ctx context.Context, id string) (*Token, error)
	Set(ctx context.Context, token *Token) error
}

func NewTokenManager(store TokenStore, timeout time.Duration) *TokenManager {
	return &TokenManager{store: store, timeout: timeout}
}

func (tm *TokenManager) Validate(ctx context.Context, raw string) (*Claims, error) {
	return nil, nil
}

func parseHeader(raw string) (Header, error) {
	return Header{}, nil
}
`
	exp, err := ce.ExplainFile("auth.go", src)
	if err != nil {
		t.Fatalf("ExplainFile failed: %v", err)
	}

	if exp.Symbol != "auth" {
		t.Errorf("expected symbol auth, got %s", exp.Symbol)
	}

	sectionTitles := make(map[string]bool)
	for _, s := range exp.Sections {
		sectionTitles[s.Title] = true
	}

	if !sectionTitles["Package Purpose"] {
		t.Error("missing Package Purpose section")
	}
	if !sectionTitles["Exported API"] {
		t.Error("missing Exported API section")
	}
	if !sectionTitles["Internal Structure"] {
		t.Error("missing Internal Structure section")
	}
	if !sectionTitles["Key Patterns"] {
		t.Error("missing Key Patterns section")
	}

	// Verify exported items found
	var apiSection string
	for _, s := range exp.Sections {
		if s.Title == "Exported API" {
			apiSection = s.Content
			break
		}
	}
	if !strings.Contains(apiSection, "TokenManager") {
		t.Errorf("expected TokenManager in API section, got %q", apiSection)
	}
	if !strings.Contains(apiSection, "NewTokenManager") {
		t.Errorf("expected NewTokenManager in API section, got %q", apiSection)
	}
}

func TestInferPurpose(t *testing.T) {
	tests := []struct {
		name     string
		params   []string
		returns  []string
		contains string
	}{
		{
			name:     "NewClient",
			params:   []string{"string", "int"},
			returns:  []string{"*Client"},
			contains: "Creates a new",
		},
		{
			name:     "GetUser",
			params:   []string{"string"},
			returns:  []string{"*User", "error"},
			contains: "Retrieves",
		},
		{
			name:     "ValidateToken",
			params:   []string{"string"},
			returns:  []string{"*Claims", "error"},
			contains: "Validates",
		},
		{
			name:     "IsActive",
			params:   []string{},
			returns:  []string{"bool"},
			contains: "Returns whether",
		},
		{
			name:     "DeleteSession",
			params:   []string{"string"},
			returns:  []string{"error"},
			contains: "Removes",
		},
		{
			name:     "ParseConfig",
			params:   []string{"[]byte"},
			returns:  []string{"*Config", "error"},
			contains: "Parses",
		},
		{
			name:     "SetTimeout",
			params:   []string{"time.Duration"},
			returns:  []string{},
			contains: "Updates",
		},
		{
			name:     "Close",
			params:   []string{},
			returns:  []string{"error"},
			contains: "Shuts down",
		},
		{
			name:     "StartWorker",
			params:   []string{"context.Context"},
			returns:  []string{"error"},
			contains: "Starts",
		},
		{
			name:     "FindMatches",
			params:   []string{"string"},
			returns:  []string{"[]Match"},
			contains: "Searches",
		},
		{
			name:     "FormatOutput",
			params:   []string{"*Result"},
			returns:  []string{"string"},
			contains: "Formats",
		},
		{
			name:     "HandleRequest",
			params:   []string{"*http.Request"},
			returns:  []string{"*http.Response", "error"},
			contains: "Handles",
		},
		{
			name:     "WriteConfig",
			params:   []string{"*Config", "string"},
			returns:  []string{"error"},
			contains: "Persists",
		},
		{
			name:     "InitDatabase",
			params:   []string{"string"},
			returns:  []string{"error"},
			contains: "Initializes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InferPurpose(tt.name, tt.params, tt.returns)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("InferPurpose(%q) = %q, want to contain %q", tt.name, result, tt.contains)
			}
		})
	}
}

func TestDescribeControlFlow(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		contains string
	}{
		{
			name:     "linear with error returns",
			body:     `if err != nil { return nil, err }`,
			contains: "early error returns",
		},
		{
			name:     "loop with break",
			body:     `for i := 0; i < n; i++ { if done { break } }`,
			contains: "Loop with conditional break",
		},
		{
			name:     "range iteration",
			body:     `for _, v := range items { process(v) }`,
			contains: "Range iteration",
		},
		{
			name:     "switch dispatch",
			body:     `switch kind { case "a": doA() case "b": doB() }`,
			contains: "Switch dispatch",
		},
		{
			name:     "select",
			body:     `select { case <-ctx.Done(): return case msg := <-ch: handle(msg) }`,
			contains: "Channel select",
		},
		{
			name:     "with defer",
			body:     `defer f.Close() if err != nil { return err }`,
			contains: "deferred cleanup",
		},
		{
			name:     "with goroutine",
			body:     `go worker(ctx)`,
			contains: "concurrent goroutines",
		},
		{
			name:     "range with continue",
			body:     `for _, v := range items { if v == nil { continue } }`,
			contains: "conditional skip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DescribeControlFlow(tt.body)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("DescribeControlFlow(%q) = %q, want to contain %q", tt.name, result, tt.contains)
			}
		})
	}
}

func TestDetectSideEffects(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		contains string
	}{
		{
			name:     "file IO",
			body:     `os.ReadFile(path)`,
			contains: "File I/O",
		},
		{
			name:     "network",
			body:     `http.Get(url)`,
			contains: "Network calls",
		},
		{
			name:     "goroutine",
			body:     `go worker(ctx)`,
			contains: "Goroutine spawning",
		},
		{
			name:     "mutex",
			body:     `m.Lock()`,
			contains: "Mutex locking",
		},
		{
			name:     "channel",
			body:     `ch <- msg`,
			contains: "Channel communication",
		},
		{
			name:     "database",
			body:     `db.Exec("INSERT INTO ...")`,
			contains: "Database operations",
		},
		{
			name:     "stdout",
			body:     `fmt.Println("hello")`,
			contains: "Standard output",
		},
		{
			name:     "pure function",
			body:     `return a + b`,
			contains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectSideEffects(tt.body)
			if tt.contains == "" {
				if len(result) != 0 {
					t.Errorf("expected no side effects for %q, got %v", tt.name, result)
				}
				return
			}
			found := false
			for _, r := range result {
				if strings.Contains(r, tt.contains) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("DetectSideEffects(%q) = %v, want to contain %q", tt.name, result, tt.contains)
			}
		})
	}
}

func TestFormatExplanation(t *testing.T) {
	exp := &CodeExplanation{
		Symbol:  "ValidateToken",
		Summary: "Validates a JWT token",
		Sections: []ExplanationSection{
			{Title: "Purpose", Content: "Validates a JWT token and returns the parsed claims."},
			{Title: "Parameters", Content: "- `token string` — The raw JWT token string to validate"},
			{Title: "Returns", Content: "`*Claims, error`"},
			{Title: "Control Flow", Content: "Linear with early error returns"},
			{Title: "Error Handling", Content: "Returns wrapped errors with context"},
			{Title: "Side Effects", Content: "None (pure function)"},
		},
		Complexity: "Low (CC: 4)",
	}

	result := FormatExplanation(exp)

	if !strings.Contains(result, "## ValidateToken") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "**Purpose:**") {
		t.Error("missing Purpose section")
	}
	if !strings.Contains(result, "**Parameters:**") {
		t.Error("missing Parameters section")
	}
	if !strings.Contains(result, "**Returns:**") {
		t.Error("missing Returns section")
	}
	if !strings.Contains(result, "**Control Flow:**") {
		t.Error("missing Control Flow section")
	}
	if !strings.Contains(result, "**Error Handling:**") {
		t.Error("missing Error Handling section")
	}
	if !strings.Contains(result, "**Side Effects:**") {
		t.Error("missing Side Effects section")
	}
	if !strings.Contains(result, "**Complexity:** Low (CC: 4)") {
		t.Error("missing Complexity line")
	}
}

func TestExplainFunction_NoParams(t *testing.T) {
	ce := NewCodeExplainer()
	src := `package example

func Close() error {
	return nil
}
`
	exp, err := ce.ExplainFunction("close.go", src, "Close")
	if err != nil {
		t.Fatalf("ExplainFunction failed: %v", err)
	}

	// Should not have Parameters section
	for _, s := range exp.Sections {
		if s.Title == "Parameters" {
			t.Error("unexpected Parameters section for no-param function")
		}
	}
}

func TestExplainFunction_DocComment(t *testing.T) {
	ce := NewCodeExplainer()
	src := `package example

// ProcessBatch handles a batch of items concurrently.
func ProcessBatch(items []string) error {
	for _, item := range items {
		go handle(item)
	}
	return nil
}
`
	exp, err := ce.ExplainFunction("batch.go", src, "ProcessBatch")
	if err != nil {
		t.Fatalf("ExplainFunction failed: %v", err)
	}

	// Summary should come from doc comment
	if !strings.Contains(exp.Summary, "handles a batch") {
		t.Errorf("expected doc comment in summary, got %q", exp.Summary)
	}
}

func TestExplainType_WithStringerInterface(t *testing.T) {
	ce := NewCodeExplainer()
	src := `package example

type Status struct {
	Code    int
	Message string
}

func (s *Status) String() string {
	return s.Message
}

func (s *Status) Error() string {
	return s.Message
}
`
	exp, err := ce.ExplainType("status.go", src, "Status")
	if err != nil {
		t.Fatalf("ExplainType failed: %v", err)
	}

	var implSection string
	for _, s := range exp.Sections {
		if s.Title == "Implements" {
			implSection = s.Content
			break
		}
	}
	if !strings.Contains(implSection, "fmt.Stringer") {
		t.Errorf("expected fmt.Stringer in implements, got %q", implSection)
	}
	if !strings.Contains(implSection, "error") {
		t.Errorf("expected error in implements, got %q", implSection)
	}
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"ValidateToken", []string{"Validate", "Token"}},
		{"NewHTTPClient", []string{"New", "H", "T", "T", "P", "Client"}},
		{"getUser", []string{"get", "User"}},
		{"ID", []string{"I", "D"}},
		{"simple", []string{"simple"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitCamelCase(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("splitCamelCase(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("splitCamelCase(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestComputeCyclomaticComplexity(t *testing.T) {
	tests := []struct {
		name string
		body string
		min  int
		max  int
	}{
		{
			name: "simple",
			body: "return nil",
			min:  1,
			max:  1,
		},
		{
			name: "one if",
			body: "if x > 0 { return x }",
			min:  2,
			max:  2,
		},
		{
			name: "if with and",
			body: "if x > 0 && y > 0 { return x }",
			min:  3,
			max:  3,
		},
		{
			name: "loop with ifs",
			body: "for _, v := range items { if v > 0 { count++ } else if v < 0 { neg++ } }",
			min:  4,
			max:  5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := computeCyclomaticComplexity(tt.body)
			if cc < tt.min || cc > tt.max {
				t.Errorf("computeCyclomaticComplexity = %d, want between %d and %d", cc, tt.min, tt.max)
			}
		})
	}
}

func TestExplainFile_ParseError(t *testing.T) {
	ce := NewCodeExplainer()
	_, err := ce.ExplainFile("bad.go", "this is not valid go {{{{")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestDetectPatterns(t *testing.T) {
	src := `package example

import (
	"context"
	"sync"
	"fmt"
)

type Service struct {
	mu   sync.Mutex
	once sync.Once
	ch   chan string
}

func NewService() *Service {
	return &Service{ch: make(chan string)}
}

func (s *Service) Run(ctx context.Context) error {
	defer s.cleanup()
	select {
	case <-ctx.Done():
		return fmt.Errorf("cancelled: %w", ctx.Err())
	case msg := <-s.ch:
		_ = msg
	}
	return nil
}
`
	patterns := detectPatterns(src)
	if len(patterns) == 0 {
		t.Fatal("expected patterns to be detected")
	}

	joined := strings.Join(patterns, "\n")
	checks := []string{
		"Mutex-based concurrency",
		"Singleton/once initialization",
		"Channel-based communication",
		"Context propagation",
		"Constructor functions",
		"Deferred cleanup",
		"Error wrapping",
		"Select-based multiplexing",
	}
	for _, check := range checks {
		if !strings.Contains(joined, check) {
			t.Errorf("expected pattern %q to be detected", check)
		}
	}
}
