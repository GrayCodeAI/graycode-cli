package minify

import (
	"strings"
	"testing"
)

func TestFileGoStripsCommentsAndWhitespace(t *testing.T) {
	src := []byte(`// header comment
package demo

// a doc comment
func Add(a, b int) int {
	// inline
	return a + b
}
`)
	res := File("x.go", src)
	if !res.Applied {
		t.Fatal("expected Go minification to apply")
	}
	if res.Language != "go" {
		t.Fatalf("language = %q", res.Language)
	}
	if strings.Contains(res.Content, "//") {
		t.Fatalf("comments not stripped:\n%s", res.Content)
	}
	if strings.Contains(res.Content, "header comment") {
		t.Fatalf("comment text leaked:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "func Add") {
		t.Fatalf("code missing:\n%s", res.Content)
	}
}

func TestFileGenericPreservesComments(t *testing.T) {
	src := []byte("// keep me\n\n\n\nfoo()\n")
	res := File("x.txt", src)
	if res.Applied {
		t.Fatal("text file should not apply comment stripping")
	}
	if !strings.Contains(res.Content, "// keep me") {
		t.Fatalf("comment should be preserved for text: %q", res.Content)
	}
	if strings.Contains(res.Content, "\n\n\n") {
		t.Fatalf("blank lines not collapsed: %q", res.Content)
	}
}

func TestFilePythonStripsCommentsButKeepsString(t *testing.T) {
	src := []byte(`# a comment
s = "http://example.com/#frag"
t = '''not a comment # still here'''
# trailing
print(s)
`)
	res := File("x.py", src)
	if !res.Applied {
		t.Fatal("expected python minification")
	}
	if strings.Contains(res.Content, "# a comment") || strings.Contains(res.Content, "# trailing") {
		t.Fatalf("python comments not stripped:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "http://example.com/#frag") {
		t.Fatalf("url in string was corrupted:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "not a comment # still here") {
		t.Fatalf("triple-quoted content corrupted:\n%s", res.Content)
	}
}

func TestFileGoInvalidFallsBackToText(t *testing.T) {
	res := File("x.go", []byte("func { not valid go"))
	if res.Applied {
		t.Fatal("invalid Go must not apply comment stripping")
	}
	if res.Language != "text" {
		t.Fatalf("language = %q", res.Language)
	}
}

func TestFileCRLFNormalized(t *testing.T) {
	res := File("x.txt", []byte("a\r\n\r\n\r\nb\r\n"))
	if strings.Contains(res.Content, "\r") {
		t.Fatalf("CRLF not normalized: %q", res.Content)
	}
}

func TestFragmentGo(t *testing.T) {
	src := []byte("// c\nfunc A() {}\nfunc B() {}\n")
	res := Fragment("x.go", src)
	if !res.Applied {
		t.Fatal("expected go fragment minification")
	}
	if strings.Contains(res.Content, "// c") {
		t.Fatalf("comment not stripped in fragment:\n%s", res.Content)
	}
}

func TestContextualFragmentInsideToken(t *testing.T) {
	// Fragment starts inside a string literal; it must not be parsed as Go.
	source := []byte(`s := "hello world"`)
	content := []byte(`o world"`)
	res := ContextualFragment("x.go", source, content, 9)
	if res.Applied {
		t.Fatal("fragment starting inside a token must not apply Go parsing")
	}
}
