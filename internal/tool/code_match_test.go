package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMatchTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestCodeMatchGoFunctionPattern(t *testing.T) {
	root := writeMatchTree(t, map[string]string{
		"a/a.go": "package a\n\n// helper comment mentioning Handler\nfunc Handler(w io.Writer) { }\n\nfunc ignored() {}\n",
	})
	out, err := CodeMatchTool{}.Execute(context.Background(), json.RawMessage(
		`{"pattern":"(function_declaration name: (identifier) @name) @fn","path":"`+root+`","language":"go"}`,
	))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var resp struct {
		Matches int `json:"matches"`
		Hits    []struct {
			File      string   `json:"file"`
			StartLine int      `json:"start_line"`
			Captures  []string `json:"captures"`
			Snippet   string   `json:"snippet"`
		} `json:"hits"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Matches != 2 {
		t.Fatalf("matches = %d (%s)", resp.Matches, out)
	}
	if len(resp.Hits[0].Captures) == 0 {
		t.Fatal("expected captures in hits")
	}
}

func TestCodeMatchCommentsDoNotFalsePositive(t *testing.T) {
	root := writeMatchTree(t, map[string]string{
		"a/a.go": "package a\n\n// func fakeDeclaration name: (identifier)\nfunc Real() {}\n",
	})
	out, err := CodeMatchTool{}.Execute(context.Background(), json.RawMessage(
		`{"pattern":"(function_declaration name: (identifier) @n) @f","path":"`+root+`","language":"go","limit":10}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, `"start_line"`) != 1 {
		t.Fatalf("comment produced a structural match: %s", out)
	}
}

func TestCodeMatchPythonDef(t *testing.T) {
	root := writeMatchTree(t, map[string]string{
		"svc.py": "def handler(req):\n    return req\n\nclass C:\n    def method(self):\n        pass\n",
	})
	out, err := CodeMatchTool{}.Execute(context.Background(), json.RawMessage(
		`{"pattern":"(function_definition name: (identifier) @name) @fn","path":"`+root+`","language":"python"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Matches int `json:"matches"`
	}
	_ = json.Unmarshal([]byte(out), &resp)
	if resp.Matches != 2 { // handler + method
		t.Fatalf("python defs matched = %d (%s)", resp.Matches, out)
	}
}

func TestCodeMatchLanguageFilterAndLimit(t *testing.T) {
	root := writeMatchTree(t, map[string]string{
		"x.go": "package x\nfunc A() {}\nfunc B() {}\nfunc C() {}\n",
	})
	out, err := CodeMatchTool{}.Execute(context.Background(), json.RawMessage(
		`{"pattern":"(function_declaration) @f","path":"`+root+`","language":"go","limit":2}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Matches   int  `json:"matches"`
		Truncated bool `json:"truncated"`
	}
	_ = json.Unmarshal([]byte(out), &resp)
	if resp.Matches != 2 || !resp.Truncated {
		t.Fatalf("limit not applied: matches=%d truncated=%v", resp.Matches, resp.Truncated)
	}
}

func TestCodeMatchInvalidPatternFailsBeforeWalk(t *testing.T) {
	root := t.TempDir()
	tool := CodeMatchTool{}
	_, err := tool.Execute(context.Background(), json.RawMessage(
		`{"pattern":"((( not-a-query","path":"`+root+`","language":"go"}`,
	))
	if err == nil {
		t.Fatal("invalid pattern must error before scanning")
	}
}

func TestCodeMatchUnsupportedLanguage(t *testing.T) {
	tool := CodeMatchTool{}
	if _, err := tool.Execute(context.Background(), json.RawMessage(
		`{"pattern":"(x)","language":"ruby"}`,
	)); err == nil {
		t.Fatal("unsupported language must error")
	}
}
