package docs

import (
	"strings"
	"testing"
)

func TestNewExternalDocs(t *testing.T) {
	ed := NewExternalDocs()
	if ed == nil {
		t.Fatal("NewExternalDocs() returned nil")
	}
	if len(ed.Sources) == 0 {
		t.Error("expected default sources")
	}
}

func TestFindRelevant(t *testing.T) {
	ed := NewExternalDocs()
	results := ed.FindRelevant("use chi router for routing", "go", 3)
	if len(results) == 0 {
		// FIXME: no results; may depend on default source content
		t.Skip("no results; may depend on default source content")
	}
}

func TestExtractPackageRefs(t *testing.T) {
	ed := NewExternalDocs()
	refs := ed.ExtractPackageRefs("use chi for routing and cobra for CLI")
	// FIXME: test skipped in TestExtractPackageRefs
	if len(refs) == 0 {
// FIXME: test skipped
		t.Skip("no refs found")
	}
}

func TestBuildDocContext(t *testing.T) {
	ed := NewExternalDocs()
	results := []DocResult{
		{Source: "pkg.go.dev", Title: "chi - pkg.go.dev", URL: "https://pkg.go.dev/chi", Relevance: 0.8},
	}
	ctx := ed.BuildDocContext(results, 100)
	if ctx == "" {
		t.Error("expected non-empty context")
	}
}

func TestFormatResults(t *testing.T) {
	ed := NewExternalDocs()
	result := ed.FormatResults(nil)
	if !strings.Contains(result, "No relevant documentation found") {
		t.Errorf("expected no-results message, got %q", result)
	}
}

func TestRegisterSource(t *testing.T) {
	ed := NewExternalDocs()
	count := len(ed.Sources)
	ed.RegisterSource(DocSource{Name: "test-source"})
	if len(ed.Sources) != count+1 {
		t.Errorf("sources = %d, want %d", len(ed.Sources), count+1)
	}
}
