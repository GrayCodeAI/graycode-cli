package localize

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// langFromExt - comprehensive
// ---------------------------------------------------------------------------

func TestLangFromExt_AllSupported(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".go", "go"},
		{".py", "python"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".js", "javascript"},
		{".jsx", "javascript"},
		{".rs", "rust"},
		{".java", "java"},
	}
	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			if got := langFromExt(tt.ext); got != tt.want {
				t.Errorf("langFromExt(%q) = %q, want %q", tt.ext, got, tt.want)
			}
		})
	}
}

func TestLangFromExt_Unsupported(t *testing.T) {
	for _, ext := range []string{".txt", ".md", ".yaml", ".json", ".html", ".css", ".rb"} {
		t.Run(ext, func(t *testing.T) {
			if got := langFromExt(ext); got != "" {
				t.Errorf("langFromExt(%q) = %q, want empty", ext, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// patternsForLang
// ---------------------------------------------------------------------------

func TestPatternsForLang_AllLanguages(t *testing.T) {
	for _, lang := range []string{"go", "python", "typescript", "javascript", "rust", "java"} {
		t.Run(lang, func(t *testing.T) {
			pats := patternsForLang(lang)
			if len(pats) == 0 {
				t.Errorf("patternsForLang(%q) returned empty", lang)
			}
		})
	}
}

func TestPatternsForLang_Unknown(t *testing.T) {
	pats := patternsForLang("unknown")
	if pats != nil {
		t.Errorf("expected nil for unknown language, got %d patterns", len(pats))
	}
}

// ---------------------------------------------------------------------------
// extractSymbols - Go
// ---------------------------------------------------------------------------

func TestExtractSymbols_GoFunction(t *testing.T) {
	dir := t.TempDir()
	src := `package main

func hello() string {
	return "hello"
}

func add(a, b int) int {
	return a + b
}
`
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte(src), 0o644)

	symbols, err := extractSymbols(path, "go")
	if err != nil {
		t.Fatal(err)
	}
	names := symbolNames(symbols)
	if !contains(names, "hello") {
		t.Errorf("expected 'hello' in symbols, got %v", names)
	}
	if !contains(names, "add") {
		t.Errorf("expected 'add' in symbols, got %v", names)
	}
}

func TestExtractSymbols_GoMethod(t *testing.T) {
	dir := t.TempDir()
	src := `package main

type Server struct{}

func (s *Server) Start() error {
	return nil
}

func (s *Server) Stop() {}
`
	path := filepath.Join(dir, "server.go")
	os.WriteFile(path, []byte(src), 0o644)

	symbols, err := extractSymbols(path, "go")
	if err != nil {
		t.Fatal(err)
	}

	// Go methods should be qualified as Receiver.Method
	names := symbolNames(symbols)
	if !contains(names, "Server.Start") {
		t.Errorf("expected 'Server.Start', got %v", names)
	}
	if !contains(names, "Server.Stop") {
		t.Errorf("expected 'Server.Stop', got %v", names)
	}
}

func TestExtractSymbols_GoType(t *testing.T) {
	dir := t.TempDir()
	src := `package main

type Config struct {
	Name string
}

type Handler interface {
	Handle()
}
`
	path := filepath.Join(dir, "types.go")
	os.WriteFile(path, []byte(src), 0o644)

	symbols, err := extractSymbols(path, "go")
	if err != nil {
		t.Fatal(err)
	}

	names := symbolNames(symbols)
	if !contains(names, "Config") {
		t.Errorf("expected 'Config', got %v", names)
	}
	if !contains(names, "Handler") {
		t.Errorf("expected 'Handler', got %v", names)
	}
}

func TestExtractSymbols_GoConstVar(t *testing.T) {
	dir := t.TempDir()
	src := `package main

const MaxRetries = 3

var DefaultTimeout = 30
`
	path := filepath.Join(dir, "constants.go")
	os.WriteFile(path, []byte(src), 0o644)

	symbols, err := extractSymbols(path, "go")
	if err != nil {
		t.Fatal(err)
	}

	kinds := symbolKinds(symbols)
	names := symbolNames(symbols)
	if !contains(names, "MaxRetries") {
		t.Errorf("expected 'MaxRetries', got %v", names)
	}
	if !contains(names, "DefaultTimeout") {
		t.Errorf("expected 'DefaultTimeout', got %v", names)
	}
	// Verify kinds
	if !contains(kinds, "const") {
		t.Errorf("expected 'const' kind, got %v", kinds)
	}
	if !contains(kinds, "var") {
		t.Errorf("expected 'var' kind, got %v", kinds)
	}
}

// ---------------------------------------------------------------------------
// extractSymbols - Python
// ---------------------------------------------------------------------------

func TestExtractSymbols_PythonClass(t *testing.T) {
	dir := t.TempDir()
	src := `class MyService:
    def __init__(self):
        pass

async def fetch_data(url):
    pass
`
	path := filepath.Join(dir, "service.py")
	os.WriteFile(path, []byte(src), 0o644)

	symbols, err := extractSymbols(path, "python")
	if err != nil {
		t.Fatal(err)
	}

	names := symbolNames(symbols)
	if !contains(names, "MyService") {
		t.Errorf("expected 'MyService', got %v", names)
	}
	if !contains(names, "fetch_data") {
		t.Errorf("expected 'fetch_data', got %v", names)
	}
}

func TestExtractSymbols_PythonMethod(t *testing.T) {
	dir := t.TempDir()
	src := `class Parser:
    def parse(self, text):
        return text

    async def async_parse(self, text):
        return text
`
	path := filepath.Join(dir, "parser.py")
	os.WriteFile(path, []byte(src), 0o644)

	symbols, err := extractSymbols(path, "python")
	if err != nil {
		t.Fatal(err)
	}

	names := symbolNames(symbols)
	if !contains(names, "parse") {
		t.Errorf("expected 'parse' method, got %v", names)
	}
	if !contains(names, "async_parse") {
		t.Errorf("expected 'async_parse' method, got %v", names)
	}
}

// ---------------------------------------------------------------------------
// extractSymbols - TypeScript
// ---------------------------------------------------------------------------

func TestExtractSymbols_TypeScript(t *testing.T) {
	dir := t.TempDir()
	src := `export interface UserProps {
    name: string;
}

export type UserID = string;

export class UserService {
    async getUser(id: string): Promise<UserProps> {
        return { name: "test" };
    }
}

export function createUser(name: string): UserProps {
    return { name };
}

export const VERSION = "1.0.0";
`
	path := filepath.Join(dir, "user.ts")
	os.WriteFile(path, []byte(src), 0o644)

	symbols, err := extractSymbols(path, "typescript")
	if err != nil {
		t.Fatal(err)
	}

	names := symbolNames(symbols)
	for _, want := range []string{"UserProps", "UserID", "UserService", "createUser", "VERSION"} {
		if !contains(names, want) {
			t.Errorf("expected %q in symbols, got %v", want, names)
		}
	}
}

// ---------------------------------------------------------------------------
// extractSymbols - Rust
// ---------------------------------------------------------------------------

func TestExtractSymbols_Rust(t *testing.T) {
	dir := t.TempDir()
	src := `pub struct Config {
    name: String,
}

pub enum Status {
    Active,
    Inactive,
}

pub trait Handler {
    fn handle(&self);
}

impl Handler for Config {
    fn handle(&self) {
        println!("{}", self.name);
    }
}

pub fn new_config(name: &str) -> Config {
    Config { name: name.to_string() }
}
`
	path := filepath.Join(dir, "config.rs")
	os.WriteFile(path, []byte(src), 0o644)

	symbols, err := extractSymbols(path, "rust")
	if err != nil {
		t.Fatal(err)
	}

	names := symbolNames(symbols)
	for _, want := range []string{"Config", "Status", "Handler", "new_config"} {
		if !contains(names, want) {
			t.Errorf("expected %q in symbols, got %v", want, names)
		}
	}
}

// ---------------------------------------------------------------------------
// extractSymbols - unsupext
// ---------------------------------------------------------------------------

func TestExtractSymbols_UnsupportedExtension(t *testing.T) {
	dir := t.TempDir()
	src := `some content`
	path := filepath.Join(dir, "data.txt")
	os.WriteFile(path, []byte(src), 0o644)

	symbols, err := extractSymbols(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if symbols != nil {
		t.Errorf("expected nil symbols for unsupported ext, got %d", len(symbols))
	}
}

// ---------------------------------------------------------------------------
// scoreSymbol - comprehensive
// ---------------------------------------------------------------------------

func TestScoreSymbol_ExactMatch(t *testing.T) {
	score := scoreSymbol("Session", "type", []string{"Session"})
	if score < 10 {
		t.Errorf("expected score >= 10 for exact match, got %f", score)
	}
}

func TestScoreSymbol_ContainsKeyword(t *testing.T) {
	score := scoreSymbol("NewSession", "function", []string{"session"})
	if score < 5 {
		t.Errorf("expected score >= 5 for contains match, got %f", score)
	}
}

func TestScoreSymbol_PartMatch(t *testing.T) {
	score := scoreSymbol("HandleRequest", "function", []string{"request"})
	if score < 4 {
		t.Errorf("expected score >= 4 for part match, got %f", score)
	}
}

func TestScoreSymbol_NoMatch(t *testing.T) {
	score := scoreSymbol("ProcessData", "function", []string{"auth", "login"})
	if score != 0 {
		t.Errorf("expected score 0 for no match, got %f", score)
	}
}

func TestScoreSymbol_MultipleKeywords(t *testing.T) {
	score := scoreSymbol("HandleAuth", "function", []string{"handle", "auth"})
	if score < 8 {
		t.Errorf("expected score >= 8 for multi-keyword match, got %f", score)
	}
}

func TestScoreSymbol_CaseInsensitive(t *testing.T) {
	score := scoreSymbol("myFunction", "function", []string{"MYFUNCTION"})
	if score < 10 {
		t.Errorf("expected score >= 10 for case-insensitive exact match, got %f", score)
	}
}

// ---------------------------------------------------------------------------
// splitCamelCase
// ---------------------------------------------------------------------------

func TestSplitCamelCase_Empty(t *testing.T) {
	got := splitCamelCase("")
	if got != nil {
		t.Errorf("expected nil for empty, got %v", got)
	}
}

func TestSplitCamelCase_SingleWord(t *testing.T) {
	got := splitCamelCase("hello")
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("expected ['hello'], got %v", got)
	}
}

func TestSplitCamelCase_MultipleWords(t *testing.T) {
	got := splitCamelCase("handleRequest")
	if len(got) != 2 || got[0] != "handle" || got[1] != "request" {
		t.Errorf("expected ['handle', 'request'], got %v", got)
	}
}

func TestSplitCamelCase_ConsecutiveCaps(t *testing.T) {
	got := splitCamelCase("parseJSON")
	// Should split at the uppercase letter boundary
	if len(got) < 2 {
		t.Errorf("expected at least 2 parts, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// extractKeywords
// ---------------------------------------------------------------------------

func TestExtractKeywords_RemovesStopWords(t *testing.T) {
	kw := extractKeywords("how to fix the broken session")
	for _, stop := range []string{"how", "to", "the"} {
		for _, k := range kw {
			if k == stop {
				t.Errorf("stop word %q should be filtered", stop)
			}
		}
	}
}

func TestExtractKeywords_RemovesShortTokens(t *testing.T) {
	kw := extractKeywords("a bb ccc dddd")
	for _, k := range kw {
		if len(k) < 2 {
			t.Errorf("token %q is too short (< 2 chars)", k)
		}
	}
}

func TestExtractKeywords_Deduplication(t *testing.T) {
	kw := extractKeywords("session session session")
	seen := map[string]bool{}
	for _, k := range kw {
		if seen[k] {
			t.Errorf("duplicate keyword %q", k)
		}
		seen[k] = true
	}
}

func TestExtractKeywords_EmptyQuery(t *testing.T) {
	kw := extractKeywords("")
	if len(kw) != 0 {
		t.Errorf("expected 0 keywords for empty query, got %d", len(kw))
	}
}

func TestExtractKeywords_PunctuationSplit(t *testing.T) {
	kw := extractKeywords("fix(session, handler)")
	found := false
	for _, k := range kw {
		if k == "session" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'session' after punctuation split, got %v", kw)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func symbolNames(syms []rawSymbol) []string {
	names := make([]string, len(syms))
	for i, s := range syms {
		names[i] = s.name
	}
	return names
}

func symbolKinds(syms []rawSymbol) []string {
	kinds := make([]string, len(syms))
	for i, s := range syms {
		kinds[i] = s.kind
	}
	return kinds
}

func contains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
