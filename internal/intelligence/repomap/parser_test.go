package repomap

import (
	"testing"
)

func TestParseGo(t *testing.T) {
	src := `package main

func main() {
	fmt.Println("hello")
}

func (s *Server) Start() error {
	return nil
}

type Config struct {
	Name string
}

type Handler interface {
	Handle() error
}
`
	symbols := parseGo(src)
	if len(symbols) != 4 {
		t.Fatalf("expected 4 symbols, got %d: %+v", len(symbols), symbols)
	}

	expect := []struct {
		name string
		kind string
	}{
		{"main", "func"},
		{"Start", "func"},
		{"Config", "struct"},
		{"Handler", "interface"},
	}
	for i, e := range expect {
		if symbols[i].Name != e.name {
			t.Errorf("symbol %d: expected name %q, got %q", i, e.name, symbols[i].Name)
		}
		if symbols[i].Kind != e.kind {
			t.Errorf("symbol %d: expected kind %q, got %q", i, e.kind, symbols[i].Kind)
		}
	}
}

func TestParsePython(t *testing.T) {
	src := `class MyClass:
    pass

def my_function():
    pass

async def my_async():
    pass
`
	symbols := parsePython(src)
	if len(symbols) != 3 {
		t.Fatalf("expected 3 symbols, got %d: %+v", len(symbols), symbols)
	}

	expect := []struct {
		name string
		kind string
	}{
		{"MyClass", "class"},
		{"my_function", "func"},
		{"my_async", "async func"},
	}
	for i, e := range expect {
		if symbols[i].Name != e.name {
			t.Errorf("symbol %d: expected name %q, got %q", i, e.name, symbols[i].Name)
		}
		if symbols[i].Kind != e.kind {
			t.Errorf("symbol %d: expected kind %q, got %q", i, e.kind, symbols[i].Kind)
		}
	}
}

func TestParseTypeScript(t *testing.T) {
	src := `export function fetchData() {
}

class UserService {
}

export interface Config {
}

export type ID = string;

export const MAX_SIZE = 100;
`
	symbols := parseTypeScript(src)
	if len(symbols) != 5 {
		t.Fatalf("expected 5 symbols, got %d: %+v", len(symbols), symbols)
	}

	expect := []struct {
		name string
		kind string
	}{
		{"fetchData", "func"},
		{"UserService", "class"},
		{"Config", "interface"},
		{"ID", "type"},
		{"MAX_SIZE", "const"},
	}
	for i, e := range expect {
		if symbols[i].Name != e.name {
			t.Errorf("symbol %d: expected name %q, got %q", i, e.name, symbols[i].Name)
		}
		if symbols[i].Kind != e.kind {
			t.Errorf("symbol %d: expected kind %q, got %q", i, e.kind, symbols[i].Kind)
		}
	}
}

func TestParseRust(t *testing.T) {
	src := `pub fn new() -> Self {
}

struct Config {
}

pub trait Handler {
}

impl Config {
}

pub enum Status {
}
`
	symbols := parseRust(src)
	if len(symbols) != 5 {
		t.Fatalf("expected 5 symbols, got %d: %+v", len(symbols), symbols)
	}

	expect := []struct {
		name string
		kind string
	}{
		{"new", "func"},
		{"Config", "struct"},
		{"Handler", "trait"},
		{"Config", "impl"},
		{"Status", "enum"},
	}
	for i, e := range expect {
		if symbols[i].Name != e.name {
			t.Errorf("symbol %d: expected name %q, got %q", i, e.name, symbols[i].Name)
		}
		if symbols[i].Kind != e.kind {
			t.Errorf("symbol %d: expected kind %q, got %q", i, e.kind, symbols[i].Kind)
		}
	}
}

func TestParseJava(t *testing.T) {
	src := `public class UserService {
}

interface Repository {
}

private enum Status {
}
`
	symbols := parseJava(src)
	if len(symbols) != 3 {
		t.Fatalf("expected 3 symbols, got %d: %+v", len(symbols), symbols)
	}

	expect := []struct {
		name string
		kind string
	}{
		{"UserService", "class"},
		{"Repository", "interface"},
		{"Status", "enum"},
	}
	for i, e := range expect {
		if symbols[i].Name != e.name {
			t.Errorf("symbol %d: expected name %q, got %q", i, e.name, symbols[i].Name)
		}
		if symbols[i].Kind != e.kind {
			t.Errorf("symbol %d: expected kind %q, got %q", i, e.kind, symbols[i].Kind)
		}
	}
}

func TestParseGoLineNumbers(t *testing.T) {
	src := `package main

func first() {}

func second() {}
`
	symbols := parseGo(src)
	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}
	if symbols[0].Line != 3 {
		t.Errorf("first symbol line: expected 3, got %d", symbols[0].Line)
	}
	if symbols[1].Line != 5 {
		t.Errorf("second symbol line: expected 5, got %d", symbols[1].Line)
	}
}

func TestParsePythonAsyncPrecedence(t *testing.T) {
	// async def should be matched as async func, not as plain def
	src := `async def handler():
    pass
`
	symbols := parsePython(src)
	if len(symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(symbols))
	}
	if symbols[0].Kind != "async func" {
		t.Errorf("expected async func, got %s", symbols[0].Kind)
	}
}

func TestParseEmpty(t *testing.T) {
	if symbols := parseGo(""); len(symbols) != 0 {
		t.Errorf("expected no symbols from empty Go source, got %d", len(symbols))
	}
	if symbols := parsePython(""); len(symbols) != 0 {
		t.Errorf("expected no symbols from empty Python source, got %d", len(symbols))
	}
	if symbols := parseTypeScript(""); len(symbols) != 0 {
		t.Errorf("expected no symbols from empty TS source, got %d", len(symbols))
	}
	if symbols := parseRust(""); len(symbols) != 0 {
		t.Errorf("expected no symbols from empty Rust source, got %d", len(symbols))
	}
	if symbols := parseJava(""); len(symbols) != 0 {
		t.Errorf("expected no symbols from empty Java source, got %d", len(symbols))
	}
}
