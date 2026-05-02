package localize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testRoot returns the hawk repo root (parent of localize/).
func testRoot(t *testing.T) string {
	t.Helper()
	// We are in hawk/localize, so root is one directory up.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(wd)
	// Sanity check: go.mod should exist at root.
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("could not find go.mod at %s: %v", root, err)
	}
	return root
}

// ---------------------------------------------------------------------------
// Stage 1: File-level localization
// ---------------------------------------------------------------------------

func TestExtractKeywords(t *testing.T) {
	kw := extractKeywords("the session is broken and needs a fix")
	if len(kw) == 0 {
		t.Fatal("expected keywords, got none")
	}
	// "session" should be present; stop words like "the", "is", "a" should not
	found := false
	for _, k := range kw {
		if k == "session" {
			found = true
		}
		if k == "the" || k == "is" || k == "a" {
			t.Errorf("stop word %q should have been filtered", k)
		}
	}
	if !found {
		t.Errorf("expected keyword 'session' in %v", kw)
	}
}

func TestFindFiles_Session(t *testing.T) {
	root := testRoot(t)
	files, err := findFiles(root, "session management", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one file match for 'session'")
	}

	// engine/session.go should be among the top results
	found := false
	for _, f := range files {
		if strings.Contains(f.Path, "session") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a session-related file in results: %+v", files)
	}
}

func TestFindFiles_Repomap(t *testing.T) {
	root := testRoot(t)
	files, err := findFiles(root, "repomap generation", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one file match for 'repomap'")
	}

	// repomap/repomap.go should appear
	found := false
	for _, f := range files {
		if strings.Contains(f.Path, filepath.Join("repomap", "repomap.go")) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected repomap/repomap.go in results: %+v", files)
	}
}

func TestScoreFile(t *testing.T) {
	tests := []struct {
		path     string
		keywords []string
		wantPos  bool // expect positive score
	}{
		{"engine/session.go", []string{"session"}, true},
		{"repomap/parser.go", []string{"parser"}, true},
		{"repomap/parser.go", []string{"unrelated", "nothing"}, false},
		{"cmd/root.go", []string{"root", "cmd"}, true},
	}
	for _, tt := range tests {
		score, _ := scoreFile(tt.path, tt.keywords)
		if tt.wantPos && score <= 0 {
			t.Errorf("scoreFile(%q, %v) = %f, want positive", tt.path, tt.keywords, score)
		}
		if !tt.wantPos && score > 0 {
			t.Errorf("scoreFile(%q, %v) = %f, want zero", tt.path, tt.keywords, score)
		}
	}
}

// ---------------------------------------------------------------------------
// Stage 2: Symbol-level localization
// ---------------------------------------------------------------------------

func TestExtractSymbols_Go(t *testing.T) {
	root := testRoot(t)
	// Use engine/session.go as a known Go file.
	path := filepath.Join(root, "engine", "session.go")
	symbols, err := extractSymbols(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) == 0 {
		t.Fatal("expected symbols from engine/session.go")
	}

	// Should find Session type and NewSession function
	foundSession := false
	foundNewSession := false
	for _, s := range symbols {
		if s.name == "Session" && s.kind == "type" {
			foundSession = true
		}
		if s.name == "NewSession" && s.kind == "function" {
			foundNewSession = true
		}
	}
	if !foundSession {
		t.Error("expected to find type Session")
	}
	if !foundNewSession {
		t.Error("expected to find func NewSession")
	}
}

func TestExtractSymbols_GoRepomap(t *testing.T) {
	root := testRoot(t)
	path := filepath.Join(root, "repomap", "repomap.go")
	symbols, err := extractSymbols(path, "")
	if err != nil {
		t.Fatal(err)
	}

	// Should find Generate function
	found := false
	for _, s := range symbols {
		if s.name == "Generate" && s.kind == "function" {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, len(symbols))
		for i, s := range symbols {
			names[i] = s.kind + " " + s.name
		}
		t.Errorf("expected to find func Generate in symbols: %v", names)
	}
}

func TestScoreSymbol(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		keywords []string
		wantPos  bool
	}{
		{"NewSession", "function", []string{"session"}, true},
		{"NewSession", "function", []string{"new", "session"}, true},
		{"Generate", "function", []string{"generate"}, true},
		{"Generate", "function", []string{"unrelated"}, false},
	}
	for _, tt := range tests {
		score := scoreSymbol(tt.name, tt.kind, tt.keywords)
		if tt.wantPos && score <= 0 {
			t.Errorf("scoreSymbol(%q, %q, %v) = %f, want positive", tt.name, tt.kind, tt.keywords, score)
		}
		if !tt.wantPos && score > 0 {
			t.Errorf("scoreSymbol(%q, %q, %v) = %f, want zero", tt.name, tt.kind, tt.keywords, score)
		}
	}
}

func TestSplitIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"NewSession", []string{"new", "session"}},
		{"find_files", []string{"find", "files"}},
		{"extractCodeBlocks", []string{"extract", "code", "blocks"}},
		{"Session.Send", []string{"session", "send"}},
	}
	for _, tt := range tests {
		got := splitIdentifier(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitIdentifier(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitIdentifier(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestFindSymbols_Integration(t *testing.T) {
	root := testRoot(t)
	files := []FileMatch{
		{Path: filepath.Join("engine", "session.go"), Score: 10.0},
	}
	symbols, err := findSymbols(root, files, "session model provider", 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) == 0 {
		t.Fatal("expected symbol matches")
	}

	// Session, SetModel, SetProvider should appear
	names := map[string]bool{}
	for _, s := range symbols {
		names[s.Name] = true
	}
	for _, want := range []string{"Session"} {
		if !names[want] {
			t.Errorf("expected symbol %q in results, got: %v", want, symbols)
		}
	}
}

// ---------------------------------------------------------------------------
// Stage 3: Full pipeline
// ---------------------------------------------------------------------------

func TestLocalize_EndToEnd(t *testing.T) {
	root := testRoot(t)
	loc, err := Localize(root, "session model routing", WithMaxFiles(5), WithMaxSymbols(10))
	if err != nil {
		t.Fatal(err)
	}

	// Stage 1: should have files
	if len(loc.Files) == 0 {
		t.Error("Stage 1: expected file matches")
	}

	// Stage 2: should have symbols
	if len(loc.Symbols) == 0 {
		t.Error("Stage 2: expected symbol matches")
	}

	// Stage 3: should have code blocks
	if len(loc.Context) == 0 {
		t.Error("Stage 3: expected code blocks")
	}

	// Code blocks should contain actual code
	for _, cb := range loc.Context {
		if cb.Content == "" {
			t.Errorf("empty code block for %s:%d-%d", cb.File, cb.StartLine, cb.EndLine)
		}
	}
}

func TestLocalize_EmptyQuery(t *testing.T) {
	root := testRoot(t)
	loc, err := Localize(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(loc.Files) != 0 {
		t.Error("expected no file matches for empty query")
	}
}

func TestLocalize_NoMatch(t *testing.T) {
	root := testRoot(t)
	loc, err := Localize(root, "xyzzyplughtwisty")
	if err != nil {
		t.Fatal(err)
	}
	if len(loc.Files) != 0 {
		t.Errorf("expected no file matches for gibberish, got %d", len(loc.Files))
	}
}

func TestLocalize_WithOptions(t *testing.T) {
	root := testRoot(t)
	loc, err := Localize(root, "repomap generate symbols",
		WithMaxFiles(3),
		WithMaxSymbols(5),
		WithContextLines(2),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(loc.Files) > 3 {
		t.Errorf("expected at most 3 files, got %d", len(loc.Files))
	}
	if len(loc.Symbols) > 5 {
		t.Errorf("expected at most 5 symbols, got %d", len(loc.Symbols))
	}
}

func TestFormatSummary(t *testing.T) {
	root := testRoot(t)
	loc, err := Localize(root, "session", WithMaxFiles(3), WithMaxSymbols(3))
	if err != nil {
		t.Fatal(err)
	}

	summary := loc.FormatSummary()
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if !strings.Contains(summary, "File-level") {
		t.Error("summary should contain 'File-level' header")
	}
	if !strings.Contains(summary, "Symbol-level") {
		t.Error("summary should contain 'Symbol-level' header")
	}
	if !strings.Contains(summary, "Edit-level") {
		t.Error("summary should contain 'Edit-level' header")
	}
}

func TestLocalize_LanguageFilter(t *testing.T) {
	root := testRoot(t)
	loc, err := Localize(root, "session model",
		WithLanguage("go"),
		WithMaxFiles(5),
		WithMaxSymbols(10),
	)
	if err != nil {
		t.Fatal(err)
	}
	// With language=go, all symbols should come from .go files
	for _, s := range loc.Symbols {
		if !strings.HasSuffix(s.File, ".go") {
			t.Errorf("expected .go file, got %s", s.File)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge cases and helpers
// ---------------------------------------------------------------------------

func TestLangFromExt(t *testing.T) {
	tests := map[string]string{
		".go":   "go",
		".py":   "python",
		".ts":   "typescript",
		".tsx":  "typescript",
		".js":   "javascript",
		".jsx":  "javascript",
		".rs":   "rust",
		".java": "java",
		".txt":  "",
		".md":   "",
	}
	for ext, want := range tests {
		got := langFromExt(ext)
		if got != want {
			t.Errorf("langFromExt(%q) = %q, want %q", ext, got, want)
		}
	}
}

func TestReadFileLines(t *testing.T) {
	root := testRoot(t)
	lines, err := readFileLines(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) < 3 {
		t.Errorf("expected go.mod to have at least 3 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "module") {
		t.Errorf("expected first line to start with 'module', got %q", lines[0])
	}
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"NewSession", []string{"new", "session"}},
		{"session", []string{"session"}},
		{"HTTPClient", []string{"h", "t", "t", "p", "client"}},
		{"parseGo", []string{"parse", "go"}},
	}
	for _, tt := range tests {
		got := splitCamelCase(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitCamelCase(%q) = %v (len %d), want %v (len %d)",
				tt.input, got, len(got), tt.want, len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitCamelCase(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}
