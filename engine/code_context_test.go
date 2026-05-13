package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewContextExtractor(t *testing.T) {
	ce := NewContextExtractor("/tmp/project", 4000)
	if ce.ProjectDir != "/tmp/project" {
		t.Errorf("ProjectDir = %q, want /tmp/project", ce.ProjectDir)
	}
	if ce.MaxTokens != 4000 {
		t.Errorf("MaxTokens = %d, want 4000", ce.MaxTokens)
	}
}

func TestNewContextExtractorDefaultTokens(t *testing.T) {
	ce := NewContextExtractor("/tmp/project", 0)
	if ce.MaxTokens != 8000 {
		t.Errorf("MaxTokens = %d, want 8000 (default)", ce.MaxTokens)
	}

	ce2 := NewContextExtractor("/tmp/project", -1)
	if ce2.MaxTokens != 8000 {
		t.Errorf("MaxTokens = %d, want 8000 (default for negative)", ce2.MaxTokens)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
		min   int
		max   int
	}{
		{"", 0, 0},
		{"x", 1, 1},
		{"hello world", 2, 4},
		{strings.Repeat("a", 100), 20, 30},
		{"func main() {\n\tfmt.Println(\"hello\")\n}", 5, 15},
	}
	for _, tt := range tests {
		got := EstimateTokens(tt.input)
		if got < tt.min || got > tt.max {
			t.Errorf("EstimateTokens(%q) = %d, want in [%d, %d]", tt.input, got, tt.min, tt.max)
		}
	}
}

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		query    string
		contains []string
		excludes []string
	}{
		{
			"fix the authentication token validation",
			[]string{"fix", "authentication", "token", "validation"},
			[]string{"the"},
		},
		{
			"add a new handler for user requests",
			[]string{"add", "new", "handler", "user", "requests"},
			[]string{"a", "for"},
		},
		{
			"",
			nil,
			nil,
		},
	}
	for _, tt := range tests {
		got := extractKeywords(tt.query)
		for _, want := range tt.contains {
			found := false
			for _, kw := range got {
				if kw == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("extractKeywords(%q) missing keyword %q, got %v", tt.query, want, got)
			}
		}
		for _, exclude := range tt.excludes {
			for _, kw := range got {
				if kw == exclude {
					t.Errorf("extractKeywords(%q) should not contain stop word %q", tt.query, exclude)
				}
			}
		}
	}
}

func TestSymbolMatchesKeywords(t *testing.T) {
	tests := []struct {
		symbol   string
		keywords []string
		want     bool
	}{
		{"ValidateToken", []string{"validate", "token"}, true},
		{"HandleRequest", []string{"handle"}, true},
		{"HandleRequest", []string{"token"}, false},
		{"parseConfig", []string{"config"}, true},
		{"", []string{"anything"}, false},
	}
	for _, tt := range tests {
		got := symbolMatchesKeywords(tt.symbol, tt.keywords)
		if got != tt.want {
			t.Errorf("symbolMatchesKeywords(%q, %v) = %v, want %v", tt.symbol, tt.keywords, got, tt.want)
		}
	}
}

func TestFindBlockEnd(t *testing.T) {
	lines := []string{
		"func foo() {",
		"    if true {",
		"        bar()",
		"    }",
		"    baz()",
		"}",
		"",
		"func next() {",
	}
	end := findBlockEnd(lines, 0)
	if end != 5 {
		t.Errorf("findBlockEnd = %d, want 5", end)
	}

	// Nested block
	end = findBlockEnd(lines, 1)
	if end != 3 {
		t.Errorf("findBlockEnd for nested = %d, want 3", end)
	}
}

func TestScoreSnippet(t *testing.T) {
	snip := &CodeSnippet{
		File:      "auth/token.go",
		StartLine: 10,
		EndLine:   25,
		Content:   "func ValidateToken(token string) error {\n\t// validate the token\n\treturn nil\n}",
		Type:      "function",
		Symbol:    "ValidateToken",
	}

	keywords := []string{"validate", "token"}
	score := scoreSnippet(snip, keywords)

	if score <= 0.0 {
		t.Errorf("score should be positive, got %f", score)
	}
	if score > 1.0 {
		t.Errorf("score should be <= 1.0, got %f", score)
	}

	// Exported symbol should score higher than unexported
	snipUnexported := &CodeSnippet{
		File:      "auth/token.go",
		StartLine: 10,
		EndLine:   25,
		Content:   "func validateToken(token string) error {\n\t// validate the token\n\treturn nil\n}",
		Type:      "function",
		Symbol:    "validateToken",
	}
	scoreUnexp := scoreSnippet(snipUnexported, keywords)
	if score <= scoreUnexp {
		t.Errorf("exported symbol score (%f) should be > unexported (%f)", score, scoreUnexp)
	}
}

func TestRankSnippets(t *testing.T) {
	ce := NewContextExtractor("/tmp", 8000)
	snippets := []CodeSnippet{
		{Symbol: "HandleRequest", Content: "func HandleRequest() {}", Type: "function"},
		{Symbol: "ValidateToken", Content: "func ValidateToken(token string) error { return nil }", Type: "function"},
		{Symbol: "Config", Content: "type Config struct {}", Type: "type"},
	}

	ranked := ce.RankSnippets(snippets, "validate token authentication")

	if len(ranked) != 3 {
		t.Fatalf("expected 3 snippets, got %d", len(ranked))
	}
	// ValidateToken should rank highest for "validate token" query
	if ranked[0].Symbol != "ValidateToken" {
		t.Errorf("expected ValidateToken first, got %s", ranked[0].Symbol)
	}
}

// setupTestProject creates a temporary Go project for testing.
func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create a sample Go file
	authDir := filepath.Join(dir, "auth")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tokenFile := filepath.Join(authDir, "token.go")
	content := `package auth

import (
	"errors"
	"strings"
)

// Claims holds token claims.
type Claims struct {
	UserID string
	Role   string
}

// ValidateToken validates a JWT token and returns its claims.
func ValidateToken(token string) (*Claims, error) {
	if token == "" {
		return nil, errors.New("empty token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}
	return &Claims{UserID: "user1", Role: "admin"}, nil
}

// RefreshToken refreshes an existing token.
func RefreshToken(token string) (string, error) {
	if token == "" {
		return "", errors.New("empty token")
	}
	return "new-token", nil
}

func internalHelper() string {
	return "helper"
}
`
	if err := os.WriteFile(tokenFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a handler file
	handlerDir := filepath.Join(dir, "handler")
	if err := os.MkdirAll(handlerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	handlerFile := filepath.Join(handlerDir, "api.go")
	handlerContent := `package handler

import "fmt"

// Handler processes API requests.
type Handler struct {
	Name string
}

// HandleRequest processes an incoming request.
func (h *Handler) HandleRequest(path string) string {
	return fmt.Sprintf("handling %s", path)
}

// NewHandler creates a new Handler.
func NewHandler(name string) *Handler {
	return &Handler{Name: name}
}
`
	if err := os.WriteFile(handlerFile, []byte(handlerContent), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestExtractFunction(t *testing.T) {
	dir := setupTestProject(t)
	ce := NewContextExtractor(dir, 8000)

	snip, err := ce.ExtractFunction("auth/token.go", "ValidateToken")
	if err != nil {
		t.Fatalf("ExtractFunction: %v", err)
	}

	if snip.Symbol != "ValidateToken" {
		t.Errorf("Symbol = %q, want ValidateToken", snip.Symbol)
	}
	if snip.Type != "function" {
		t.Errorf("Type = %q, want function", snip.Type)
	}
	if !strings.Contains(snip.Content, "func ValidateToken") {
		t.Error("content should contain function signature")
	}
	if !strings.Contains(snip.Content, "return &Claims") {
		t.Error("content should contain function body")
	}
	if snip.StartLine < 1 {
		t.Errorf("StartLine = %d, want >= 1", snip.StartLine)
	}
}

func TestExtractFunctionNotFound(t *testing.T) {
	dir := setupTestProject(t)
	ce := NewContextExtractor(dir, 8000)

	_, err := ce.ExtractFunction("auth/token.go", "NonExistentFunc")
	if err == nil {
		t.Error("expected error for non-existent function")
	}
}

func TestExtractType(t *testing.T) {
	dir := setupTestProject(t)
	ce := NewContextExtractor(dir, 8000)

	snip, err := ce.ExtractType("auth/token.go", "Claims")
	if err != nil {
		t.Fatalf("ExtractType: %v", err)
	}

	if snip.Symbol != "Claims" {
		t.Errorf("Symbol = %q, want Claims", snip.Symbol)
	}
	if snip.Type != "type" {
		t.Errorf("Type = %q, want type", snip.Type)
	}
	if !strings.Contains(snip.Content, "type Claims struct") {
		t.Error("content should contain type definition")
	}
}

func TestExtractTypeWithMethods(t *testing.T) {
	dir := setupTestProject(t)
	ce := NewContextExtractor(dir, 8000)

	snip, err := ce.ExtractType("handler/api.go", "Handler")
	if err != nil {
		t.Fatalf("ExtractType: %v", err)
	}

	if snip.Symbol != "Handler" {
		t.Errorf("Symbol = %q, want Handler", snip.Symbol)
	}
	if !strings.Contains(snip.Content, "type Handler struct") {
		t.Error("content should contain type definition")
	}
	if !strings.Contains(snip.Content, "func (h *Handler) HandleRequest") {
		t.Error("content should contain method")
	}
}

func TestExtractImports(t *testing.T) {
	dir := setupTestProject(t)
	ce := NewContextExtractor(dir, 8000)

	snip, err := ce.ExtractImports("auth/token.go")
	if err != nil {
		t.Fatalf("ExtractImports: %v", err)
	}

	if snip.Type != "import" {
		t.Errorf("Type = %q, want import", snip.Type)
	}
	if !strings.Contains(snip.Content, "errors") {
		t.Error("content should contain 'errors' import")
	}
	if !strings.Contains(snip.Content, "strings") {
		t.Error("content should contain 'strings' import")
	}
}

func TestExtractSurrounding(t *testing.T) {
	dir := setupTestProject(t)
	ce := NewContextExtractor(dir, 8000)

	snip, err := ce.ExtractSurrounding("auth/token.go", 10, 2)
	if err != nil {
		t.Fatalf("ExtractSurrounding: %v", err)
	}

	if snip.Type != "block" {
		t.Errorf("Type = %q, want block", snip.Type)
	}
	// Should have at most 5 lines (target ± 2)
	lines := strings.Split(snip.Content, "\n")
	if len(lines) > 5 {
		t.Errorf("expected at most 5 lines, got %d", len(lines))
	}
	if snip.StartLine > 10 || snip.EndLine < 10 {
		t.Errorf("range [%d, %d] should contain line 10", snip.StartLine, snip.EndLine)
	}
}

func TestExtractSurroundingOutOfRange(t *testing.T) {
	dir := setupTestProject(t)
	ce := NewContextExtractor(dir, 8000)

	_, err := ce.ExtractSurrounding("auth/token.go", 9999, 2)
	if err == nil {
		t.Error("expected error for out-of-range line")
	}
}

func TestFormatContext(t *testing.T) {
	ctx := &CodeContext{
		Query: "validate token",
		Snippets: []CodeSnippet{
			{
				File:      "auth/token.go",
				Symbol:    "ValidateToken",
				Relevance: 0.95,
				Content:   "func ValidateToken(token string) (*Claims, error) {\n    ...\n}",
				Type:      "function",
			},
			{
				File:      "handler/api.go",
				Symbol:    "HandleRequest",
				Relevance: 0.82,
				Content:   "func HandleRequest(w http.ResponseWriter, r *http.Request) {\n    ...\n}",
				Type:      "function",
			},
		},
	}

	formatted := FormatContext(ctx)

	if !strings.Contains(formatted, "## Relevant Code Context") {
		t.Error("missing header")
	}
	if !strings.Contains(formatted, "auth/token.go:ValidateToken (relevance: 0.95)") {
		t.Error("missing first snippet header")
	}
	if !strings.Contains(formatted, "handler/api.go:HandleRequest (relevance: 0.82)") {
		t.Error("missing second snippet header")
	}
	if !strings.Contains(formatted, "```go") {
		t.Error("missing code fence")
	}
}

func TestFormatContextNil(t *testing.T) {
	if got := FormatContext(nil); got != "" {
		t.Errorf("FormatContext(nil) = %q, want empty", got)
	}
	if got := FormatContext(&CodeContext{}); got != "" {
		t.Errorf("FormatContext(empty) = %q, want empty", got)
	}
}

func TestFindRelevantSymbols(t *testing.T) {
	dir := setupTestProject(t)
	ce := NewContextExtractor(dir, 8000)

	symbols := ce.FindRelevantSymbols("validate token", 10)
	if len(symbols) == 0 {
		t.Fatal("expected at least one symbol")
	}

	// Should find ValidateToken
	found := false
	for _, s := range symbols {
		if s.Symbol == "ValidateToken" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find ValidateToken symbol")
	}
}

func TestFindRelevantSymbolsEmptyQuery(t *testing.T) {
	ce := NewContextExtractor("/tmp", 8000)
	symbols := ce.FindRelevantSymbols("", 10)
	if len(symbols) != 0 {
		t.Errorf("expected no symbols for empty query, got %d", len(symbols))
	}
}

func TestExtractForTask(t *testing.T) {
	dir := setupTestProject(t)
	ce := NewContextExtractor(dir, 8000)

	ctx, err := ce.ExtractForTask("fix the token validation logic")
	if err != nil {
		t.Fatalf("ExtractForTask: %v", err)
	}

	if ctx.Query != "fix the token validation logic" {
		t.Errorf("Query = %q, want original task", ctx.Query)
	}
	if ctx.TotalTokens < 0 {
		t.Errorf("TotalTokens = %d, should be non-negative", ctx.TotalTokens)
	}
}

func TestExtractForTaskTokenBudget(t *testing.T) {
	dir := setupTestProject(t)
	// Use a very small token budget
	ce := NewContextExtractor(dir, 50)

	ctx, err := ce.ExtractForTask("validate token handler request")
	if err != nil {
		t.Fatalf("ExtractForTask: %v", err)
	}

	if ctx.TotalTokens > 50 {
		t.Errorf("TotalTokens = %d, should respect MaxTokens of 50", ctx.TotalTokens)
	}
}

func TestCodeContextConcurrency(t *testing.T) {
	ctx := &CodeContext{
		Query: "test",
		Snippets: []CodeSnippet{
			{Symbol: "Foo", Content: "func Foo() {}", Relevance: 0.9},
		},
	}

	// Test concurrent read access via FormatContext
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = FormatContext(ctx)
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
