package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree writes the given files (relative path -> content) under a fresh
// temp dir and returns the root.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

// goTree returns a small Go module where "hub" is imported by two other
// packages, so it should rank highest.
func goTree() map[string]string {
	return map[string]string{
		"hub/hub.go": `package hub

func Connect() {}
func disconnect() {}

type Session struct{}
`,
		"a/a.go": `package a

import "example.com/m/hub"

func UseA() { hub.Connect() }
`,
		"b/b.go": `package b

import "example.com/m/hub"

func UseB() { hub.Connect() }
`,
		"lonely/lonely.go": `package lonely

func Alone() {}
`,
	}
}

func TestRepoMap_RanksMostReferencedFirst(t *testing.T) {
	root := writeTree(t, goTree())

	out, err := RepoMap(root, 0)
	if err != nil {
		t.Fatalf("RepoMap: %v", err)
	}
	if out == "" {
		t.Fatal("empty map")
	}

	// hub/hub.go is imported by both a and b, so it must appear before a, b, and lonely.
	idxHub := strings.Index(out, "hub/hub.go")
	if idxHub < 0 {
		t.Fatalf("hub not in map:\n%s", out)
	}
	for _, other := range []string{"a/a.go", "b/b.go", "lonely/lonely.go"} {
		if idx := strings.Index(out, other); idx >= 0 && idx < idxHub {
			t.Errorf("%s ranked before hub/hub.go:\n%s", other, out)
		}
	}

	// hub's exported symbol should be listed.
	if !strings.Contains(out, "Connect") {
		t.Errorf("expected Connect symbol in map:\n%s", out)
	}
}

func TestRepoMap_RespectsBudget(t *testing.T) {
	root := writeTree(t, goTree())

	big, err := RepoMap(root, 1000)
	if err != nil {
		t.Fatalf("RepoMap big: %v", err)
	}
	small, err := RepoMap(root, 10)
	if err != nil {
		t.Fatalf("RepoMap small: %v", err)
	}

	if EstimateTokens(small) > 10 {
		// Allow the single-header fallback, but it must be <= budget.
		t.Errorf("small map exceeds budget: %d tokens\n%s", EstimateTokens(small), small)
	}
	if len(small) >= len(big) {
		t.Errorf("expected small budget map (%d) shorter than large (%d)", len(small), len(big))
	}
	// The tight-budget map should still lead with the top-ranked file.
	if small != "" && !strings.HasPrefix(small, "hub/hub.go") {
		t.Errorf("tight budget map should start with top file, got:\n%s", small)
	}
}

func TestRepoMap_BudgetMonotonic(t *testing.T) {
	root := writeTree(t, goTree())
	var prev int
	for _, budget := range []int{10, 30, 60, 200, 1000} {
		out, err := RepoMap(root, budget)
		if err != nil {
			t.Fatalf("budget %d: %v", budget, err)
		}
		got := EstimateTokens(out)
		if got > budget && budget >= EstimateTokens("hub/hub.go\n") {
			t.Errorf("budget %d exceeded: %d tokens", budget, got)
		}
		if got < prev {
			t.Errorf("non-monotonic: budget %d -> %d tokens, was %d", budget, got, prev)
		}
		prev = got
	}
}

func TestExtractGo(t *testing.T) {
	src := []byte(`package x

import (
	"fmt"
	"example.com/m/dep"
)

const Public = 1
var private = 2

type T struct{}

func (t T) Method() {}
func Exported() {}
func unexported() {}
`)
	syms, imps, ok := extractGo("x.go", src)
	if !ok {
		t.Fatal("extractGo failed to parse")
	}

	wantImports := map[string]bool{"fmt": false, "example.com/m/dep": false}
	for _, i := range imps {
		if _, ok := wantImports[i]; ok {
			wantImports[i] = true
		}
	}
	for imp, seen := range wantImports {
		if !seen {
			t.Errorf("missing import %q in %v", imp, imps)
		}
	}

	kinds := map[string]string{}
	exported := map[string]bool{}
	for _, s := range syms {
		kinds[s.Name] = s.Kind
		exported[s.Name] = s.Exported
	}
	cases := []struct {
		name, kind string
		exp        bool
	}{
		{"Public", "const", true},
		{"private", "var", false},
		{"T", "type", true},
		{"Method", "method", true},
		{"Exported", "func", true},
		{"unexported", "func", false},
	}
	for _, c := range cases {
		if kinds[c.name] != c.kind {
			t.Errorf("%s kind = %q, want %q", c.name, kinds[c.name], c.kind)
		}
		if exported[c.name] != c.exp {
			t.Errorf("%s exported = %v, want %v", c.name, exported[c.name], c.exp)
		}
	}
}

func TestExtractRegex(t *testing.T) {
	tests := []struct {
		name     string
		lang     string
		src      string
		wantSym  string
		wantKind string
		wantImp  string
	}{
		{
			name:     "python",
			lang:     "python",
			src:      "import os\nfrom pkg.sub import thing\n\ndef run():\n    pass\n\nclass Worker:\n    pass\n",
			wantSym:  "run",
			wantKind: "func",
			wantImp:  "os",
		},
		{
			name:     "javascript",
			lang:     "javascript",
			src:      "import {x} from './util'\n\nexport function handler() {}\nclass Box {}\n",
			wantSym:  "handler",
			wantKind: "func",
			wantImp:  "./util",
		},
		{
			name:     "rust",
			lang:     "rust",
			src:      "use std::io;\n\npub fn main() {}\npub struct Config {}\n",
			wantSym:  "main",
			wantKind: "func",
			wantImp:  "std::io",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			syms, imps := extractRegex(tc.lang, []byte(tc.src))
			foundSym := false
			for _, s := range syms {
				if s.Name == tc.wantSym && s.Kind == tc.wantKind {
					foundSym = true
				}
			}
			if !foundSym {
				t.Errorf("missing symbol %s/%s in %v", tc.wantKind, tc.wantSym, syms)
			}
			foundImp := false
			for _, i := range imps {
				if i == tc.wantImp {
					foundImp = true
				}
			}
			if !foundImp {
				t.Errorf("missing import %q in %v", tc.wantImp, imps)
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	if EstimateTokens("") != 0 {
		t.Error("empty should be 0 tokens")
	}
	if EstimateTokens("hello") < 1 {
		t.Error("non-empty should be >= 1 token")
	}
}

func TestBuild_SkipsVendorAndHidden(t *testing.T) {
	root := writeTree(t, map[string]string{
		"main.go":             "package main\nfunc main() {}\n",
		"vendor/dep/dep.go":   "package dep\nfunc V() {}\n",
		".git/hooks/pre.go":   "package x\nfunc G() {}\n",
		"node_modules/m/i.js": "export function n() {}\n",
	})
	g, err := Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := g.Nodes["main.go"]; !ok {
		t.Error("main.go should be scanned")
	}
	for skipped := range g.Nodes {
		if strings.Contains(skipped, "vendor/") || strings.Contains(skipped, ".git/") || strings.Contains(skipped, "node_modules/") {
			t.Errorf("should have skipped %s", skipped)
		}
	}
}
