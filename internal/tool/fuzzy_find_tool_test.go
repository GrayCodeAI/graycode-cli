package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFuzzyFindToolBasic(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{
		"src/config.go",
		"src/main.go",
		"internal/engine/cache_gate.go",
	} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out, err := FuzzyFindTool{}.Execute(context.Background(), json.RawMessage(
		`{"query":"config.go","path":"`+root+`","limit":5}`,
	))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var resp struct {
		Matches int `json:"matches"`
		Results []struct {
			Path  string `json:"path"`
			Score int    `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Matches == 0 {
		t.Fatal("expected matches")
	}
	// config.go (exact basename) should rank above cache_gate.go.
	if !strings.Contains(resp.Results[0].Path, "config.go") {
		t.Fatalf("top result = %s", resp.Results[0].Path)
	}
}

func TestFuzzyFindNoResults(t *testing.T) {
	root := t.TempDir()
	out, err := FuzzyFindTool{}.Execute(context.Background(), json.RawMessage(
		`{"query":"zzz_nothing","path":"`+root+`"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No files matching") {
		t.Fatalf("out = %q", out)
	}
}

func TestFuzzyFindRequiresQuery(t *testing.T) {
	tool := FuzzyFindTool{}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"/tmp"}`)); err == nil {
		t.Fatal("expected error for empty query")
	}
}
