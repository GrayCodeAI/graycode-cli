package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewSmartReader(t *testing.T) {
	sr := NewSmartReader(0)
	if sr.MaxTokens != defaultMaxTokens {
		t.Errorf("expected default %d, got %d", defaultMaxTokens, sr.MaxTokens)
	}

	sr = NewSmartReader(4000)
	if sr.MaxTokens != 4000 {
		t.Errorf("expected 4000, got %d", sr.MaxTokens)
	}
	if sr.Strategy != "relevant" {
		t.Errorf("expected strategy 'relevant', got %q", sr.Strategy)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"abcd", 1},
		{"abcde", 2},     // (5+3)/4 = 2
		{"12345678", 2},  // (8+3)/4 = 2
		{"123456789", 3}, // (9+3)/4 = 3
	}
	for _, tt := range tests {
		got := estimateTokens(tt.input)
		if got != tt.expected {
			t.Errorf("estimateTokens(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

// createTempFile creates a temporary file with the given content and returns its path.
func createTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadFile_SmallFile(t *testing.T) {
	content := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	path := createTempFile(t, "small.go", content)

	sr := NewSmartReader(8000)
	result, err := sr.ReadFile(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Truncated {
		t.Error("expected Truncated=false for small file")
	}
	if result.Strategy != "full" {
		t.Errorf("expected strategy 'full', got %q", result.Strategy)
	}
	if result.Content != content {
		t.Errorf("content mismatch")
	}
	if result.TotalLines != result.ShownLines {
		t.Errorf("expected TotalLines == ShownLines for full read")
	}
}

func TestReadFile_LargeFile_HeadTail(t *testing.T) {
	// Create a file with 200 lines.
	var lines []string
	for i := 1; i <= 200; i++ {
		lines = append(lines, fmt.Sprintf("line %d: some content here to pad the file with data", i))
	}
	content := strings.Join(lines, "\n")
	path := createTempFile(t, "large.go", content)

	sr := NewSmartReader(100) // Very small budget to force truncation.
	sr.Strategy = "head_tail"
	result, err := sr.ReadFile(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated {
		t.Error("expected Truncated=true for large file")
	}
	if result.Strategy != "head_tail" {
		t.Errorf("expected strategy 'head_tail', got %q", result.Strategy)
	}
	if result.ShownLines != 100 {
		t.Errorf("expected 100 shown lines (50+50), got %d", result.ShownLines)
	}
	if len(result.Sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(result.Sections))
	}
	if result.Sections[0].Reason != "head" {
		t.Errorf("first section reason = %q, want 'head'", result.Sections[0].Reason)
	}
	if result.Sections[1].Reason != "tail" {
		t.Errorf("second section reason = %q, want 'tail'", result.Sections[1].Reason)
	}
}

func TestReadFile_Relevant(t *testing.T) {
	var lines []string
	lines = append(lines, "package auth")
	lines = append(lines, "")
	lines = append(lines, "import \"fmt\"")
	lines = append(lines, "")
	for i := 0; i < 50; i++ {
		lines = append(lines, fmt.Sprintf("// filler line %d", i))
	}
	lines = append(lines, "func ValidateToken(token string) error {")
	lines = append(lines, "    if token == \"\" {")
	lines = append(lines, "        return fmt.Errorf(\"empty token\")")
	lines = append(lines, "    }")
	lines = append(lines, "    return nil")
	lines = append(lines, "}")
	for i := 0; i < 50; i++ {
		lines = append(lines, fmt.Sprintf("// more filler %d", i))
	}
	lines = append(lines, "func RefreshToken(old string) (string, error) {")
	lines = append(lines, "    return \"new-token\", nil")
	lines = append(lines, "}")
	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("// trailing filler %d", i))
	}
	content := strings.Join(lines, "\n")
	path := createTempFile(t, "token.go", content)

	sr := NewSmartReader(200) // Small budget, force truncation.
	sr.Strategy = "relevant"
	result, err := sr.ReadFile(path, "validate token")
	if err != nil {
		t.Fatal(err)
	}
	if result.Strategy != "relevant" {
		t.Errorf("expected strategy 'relevant', got %q", result.Strategy)
	}
	if !result.Truncated {
		t.Error("expected truncated result")
	}
	// Should include the ValidateToken function.
	if !strings.Contains(result.Content, "ValidateToken") {
		t.Error("expected result to contain ValidateToken")
	}
}

func TestReadFile_Symbols(t *testing.T) {
	content := `package mypackage

import "fmt"

type Server struct {
	port int
}

func NewServer(port int) *Server {
	return &Server{port: port}
}

func (s *Server) Start() error {
	fmt.Println("starting")
	// lots of implementation
	// more code
	// even more code
	return nil
}

func (s *Server) Stop() error {
	return nil
}

var DefaultPort = 8080
`
	// Repeat the content to make it large.
	bigContent := strings.Repeat(content+"\n", 20)
	path := createTempFile(t, "server.go", bigContent)

	sr := NewSmartReader(100) // Small budget.
	result, err := sr.ReadSymbolsOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Strategy != "symbols" {
		t.Errorf("expected strategy 'symbols', got %q", result.Strategy)
	}
	// Should have extracted function signatures.
	if !strings.Contains(result.Content, "func NewServer") {
		t.Error("expected symbols to include NewServer")
	}
	if !strings.Contains(result.Content, "type Server struct") {
		t.Error("expected symbols to include Server type")
	}
}

func TestReadWithBudget(t *testing.T) {
	var lines []string
	for i := 0; i < 500; i++ {
		lines = append(lines, fmt.Sprintf("line number %d with some content", i))
	}
	content := strings.Join(lines, "\n")
	path := createTempFile(t, "budget.txt", content)

	sr := NewSmartReader(8000)
	result, err := sr.ReadWithBudget(path, 50) // Very small budget.
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated {
		t.Error("expected truncation with small budget")
	}
	if result.Tokens > 60 { // Allow some slack.
		t.Errorf("expected tokens <= ~50, got %d", result.Tokens)
	}
}

func TestReadWithBudget_SmallFile(t *testing.T) {
	content := "hello world\n"
	path := createTempFile(t, "tiny.txt", content)

	sr := NewSmartReader(8000)
	result, err := sr.ReadWithBudget(path, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if result.Truncated {
		t.Error("expected no truncation for small file")
	}
	if result.Strategy != "full" {
		t.Errorf("expected strategy 'full', got %q", result.Strategy)
	}
}

func TestReadRange(t *testing.T) {
	var lines []string
	for i := 1; i <= 100; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	content := strings.Join(lines, "\n")
	path := createTempFile(t, "range.txt", content)

	sr := NewSmartReader(8000)
	result, err := sr.ReadRange(path, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if result.ShownLines != 11 {
		t.Errorf("expected 11 lines (10-20 inclusive), got %d", result.ShownLines)
	}
	if !strings.Contains(result.Content, "line 10") {
		t.Error("expected content to contain 'line 10'")
	}
	if !strings.Contains(result.Content, "line 20") {
		t.Error("expected content to contain 'line 20'")
	}
	if strings.Contains(result.Content, "line 9\n") {
		t.Error("should not contain line 9")
	}
	if result.TotalLines != 100 {
		t.Errorf("expected TotalLines=100, got %d", result.TotalLines)
	}
	if result.Sections[0].StartLine != 10 || result.Sections[0].EndLine != 20 {
		t.Errorf("section range mismatch: got %d-%d", result.Sections[0].StartLine, result.Sections[0].EndLine)
	}
}

func TestReadRange_InvalidStart(t *testing.T) {
	content := "line 1\nline 2\nline 3\n"
	path := createTempFile(t, "short.txt", content)

	sr := NewSmartReader(8000)
	_, err := sr.ReadRange(path, 100, 200)
	if err == nil {
		t.Error("expected error for start_line beyond file")
	}
}

func TestEstimateFileTokens(t *testing.T) {
	content := strings.Repeat("x", 400) // 400 bytes => ~100 tokens.
	path := createTempFile(t, "estimate.txt", content)

	sr := NewSmartReader(8000)
	tokens, err := sr.EstimateFileTokens(path)
	if err != nil {
		t.Fatal(err)
	}
	if tokens != 100 {
		t.Errorf("expected 100, got %d", tokens)
	}
}

func TestEstimateFileTokens_NotFound(t *testing.T) {
	sr := NewSmartReader(8000)
	_, err := sr.EstimateFileTokens("/nonexistent/file.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestFormatResult(t *testing.T) {
	result := &ReadResult{
		Content:    "some content",
		Tokens:     50,
		Truncated:  true,
		Strategy:   "relevant",
		TotalLines: 450,
		ShownLines: 120,
		Sections: []ReadSection{
			{StartLine: 1, EndLine: 30, Reason: "import"},
			{StartLine: 42, EndLine: 98, Reason: "function"},
			{StartLine: 420, EndLine: 450, Reason: "tail"},
		},
	}
	output := FormatResult("src/auth/token.go", result)
	if !strings.Contains(output, "450 lines, showing 120") {
		t.Errorf("format should show line counts: %s", output)
	}
	if !strings.Contains(output, "[1-30] (import)") {
		t.Errorf("format should show import section: %s", output)
	}
	if !strings.Contains(output, "[42-98] (function)") {
		t.Errorf("format should show function section: %s", output)
	}
	if !strings.Contains(output, "330 lines omitted") {
		t.Errorf("format should show omitted lines: %s", output)
	}
}

func TestSmartReaderTool_Interface(t *testing.T) {
	tool := NewSmartReaderTool()

	// Verify it implements Tool interface.
	var _ Tool = tool

	if tool.Name() != "SmartRead" {
		t.Errorf("expected name SmartRead, got %s", tool.Name())
	}
	if tool.RiskLevel() != "low" {
		t.Errorf("expected risk level low, got %s", tool.RiskLevel())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
	params := tool.Parameters()
	if params == nil {
		t.Error("parameters should not be nil")
	}
}

func TestSmartReaderTool_Execute(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	path := createTempFile(t, "exec_test.go", content)

	tool := NewSmartReaderTool()
	input, _ := json.Marshal(map[string]interface{}{
		"path": path,
	})

	ctx := context.Background()
	// Set up ToolContext with allowed directories.
	tc := &ToolContext{AllowedDirectories: []string{filepath.Dir(path)}}
	ctx = WithToolContext(ctx, tc)

	output, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "func main()") {
		t.Error("expected output to contain file content")
	}
}

func TestSmartReaderTool_ExecuteRange(t *testing.T) {
	var lines []string
	for i := 1; i <= 50; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	content := strings.Join(lines, "\n")
	path := createTempFile(t, "range_exec.txt", content)

	tool := NewSmartReaderTool()
	input, _ := json.Marshal(map[string]interface{}{
		"path":       path,
		"start_line": 5,
		"end_line":   10,
	})

	ctx := context.Background()
	tc := &ToolContext{AllowedDirectories: []string{filepath.Dir(path)}}
	ctx = WithToolContext(ctx, tc)

	output, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "line 5") {
		t.Error("expected output to contain line 5")
	}
	if !strings.Contains(output, "line 10") {
		t.Error("expected output to contain line 10")
	}
}

func TestSmartReaderTool_MissingPath(t *testing.T) {
	tool := NewSmartReaderTool()
	input, _ := json.Marshal(map[string]interface{}{})

	ctx := context.Background()
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestSrExtractKeywords(t *testing.T) {
	tests := []struct {
		query    string
		expected []string
	}{
		{"validate token", []string{"validate", "token"}},
		{"the function is important", []string{"function", "important"}},
		{"", nil},
		{"a an the", nil},
	}
	for _, tt := range tests {
		got := srExtractKeywords(tt.query)
		if len(got) != len(tt.expected) {
			t.Errorf("srExtractKeywords(%q) = %v, want %v", tt.query, got, tt.expected)
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("srExtractKeywords(%q)[%d] = %q, want %q", tt.query, i, got[i], tt.expected[i])
			}
		}
	}
}

func TestReadFile_BinaryFile(t *testing.T) {
	// Create binary content (null bytes).
	content := []byte{0x00, 0x01, 0x02, 0x03, 0x00, 0x00}
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	sr := NewSmartReader(8000)
	_, err := sr.ReadFile(path, "")
	if err == nil {
		t.Error("expected error for binary file")
	}
	if !strings.Contains(err.Error(), "binary") {
		t.Errorf("error should mention binary: %v", err)
	}
}

func TestReadFile_NotFound(t *testing.T) {
	sr := NewSmartReader(8000)
	_, err := sr.ReadFile("/nonexistent/path/file.txt", "")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReadFile_ConcurrentAccess(t *testing.T) {
	content := "package test\n\nfunc Hello() {}\n"
	path := createTempFile(t, "concurrent.go", content)

	sr := NewSmartReader(8000)
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := sr.ReadFile(path, "hello")
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("concurrent read failed: %v", err)
	}
}

func TestMergeSections(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	sections := []ReadSection{
		{StartLine: 1, EndLine: 3, Content: "a\nb\nc", Reason: "relevant"},
		{StartLine: 2, EndLine: 5, Content: "b\nc\nd\ne", Reason: "function"},
		{StartLine: 8, EndLine: 10, Content: "h\ni\nj", Reason: "relevant"},
	}

	merged := mergeSections(sections, lines)
	if len(merged) != 2 {
		t.Errorf("expected 2 merged sections, got %d", len(merged))
	}
	if merged[0].StartLine != 1 || merged[0].EndLine != 5 {
		t.Errorf("first merged section: got %d-%d, want 1-5", merged[0].StartLine, merged[0].EndLine)
	}
	if merged[0].Reason != "function" {
		t.Errorf("merged reason should be 'function', got %q", merged[0].Reason)
	}
	if merged[1].StartLine != 8 || merged[1].EndLine != 10 {
		t.Errorf("second section: got %d-%d, want 8-10", merged[1].StartLine, merged[1].EndLine)
	}
}

func TestReadFile_HeadTail_ShortFile(t *testing.T) {
	// A file with fewer than 100 lines should not be truncated by head_tail.
	var lines []string
	for i := 1; i <= 80; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	content := strings.Join(lines, "\n")
	path := createTempFile(t, "short_ht.txt", content)

	sr := NewSmartReader(10) // Force non-full strategy.
	sr.Strategy = "head_tail"
	result, err := sr.ReadFile(path, "")
	if err != nil {
		t.Fatal(err)
	}
	// 80 lines < 100, so head_tail should return all.
	if result.Truncated {
		t.Error("expected no truncation for file with fewer lines than head+tail")
	}
}

func TestReadRelevant_NoKeywords(t *testing.T) {
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	content := strings.Join(lines, "\n")
	path := createTempFile(t, "nq.txt", content)

	sr := NewSmartReader(100)
	sr.Strategy = "relevant"
	// Empty query should fall back to head_tail.
	result, err := sr.ReadFile(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Strategy != "head_tail" {
		t.Errorf("expected fallback to head_tail, got %q", result.Strategy)
	}
}

func TestSmartReaderTool_Aliases(t *testing.T) {
	tool := NewSmartReaderTool()
	aliases := tool.Aliases()
	if len(aliases) == 0 {
		t.Error("expected at least one alias")
	}
	found := false
	for _, a := range aliases {
		if a == "smart_read" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'smart_read' alias")
	}
}
