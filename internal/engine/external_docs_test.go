package engine

import (
	"strings"
	"testing"
)

func TestNewExternalDocs(t *testing.T) {
	ed := NewExternalDocs()
	if ed == nil {
		t.Fatal("NewExternalDocs returned nil")
	}
	if len(ed.Sources) == 0 {
		t.Fatal("expected pre-loaded sources, got 0")
	}
	if ed.Cache == nil {
		t.Fatal("Cache map not initialized")
	}
	if ed.MaxTokens != 4096 {
		t.Fatalf("expected MaxTokens=4096, got %d", ed.MaxTokens)
	}

	// Verify we have sources for each major language
	languages := make(map[string]bool)
	for _, src := range ed.Sources {
		languages[src.Language] = true
	}
	for _, lang := range []string{"go", "python", "javascript", "rust", "common"} {
		if !languages[lang] {
			t.Errorf("missing source for language: %s", lang)
		}
	}
}

func TestNewExternalDocs_PackageCount(t *testing.T) {
	ed := NewExternalDocs()
	totalPkgs := 0
	for _, src := range ed.Sources {
		totalPkgs += len(src.Packages)
	}
	if totalPkgs < 200 {
		t.Errorf("expected at least 200 packages across all sources, got %d", totalPkgs)
	}
}

func TestExtractPackageRefs_GoPackages(t *testing.T) {
	ed := NewExternalDocs()

	tests := []struct {
		input    string
		expected []string
	}{
		{"use chi router for HTTP", []string{"chi"}},
		{"import gin framework", []string{"gin"}},
		{"add cobra for CLI", []string{"cobra"}},
		{"configure viper and zap", []string{"viper", "zap"}},
		{"write tests with testify", []string{"testify"}},
	}

	for _, tt := range tests {
		refs := ed.ExtractPackageRefs(tt.input)
		for _, exp := range tt.expected {
			found := false
			for _, ref := range refs {
				if strings.EqualFold(ref, exp) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ExtractPackageRefs(%q): expected %q in results %v", tt.input, exp, refs)
			}
		}
	}
}

func TestExtractPackageRefs_PythonPackages(t *testing.T) {
	ed := NewExternalDocs()

	tests := []struct {
		input    string
		expected []string
	}{
		{"import fastapi for the web server", []string{"fastapi"}},
		{"use pandas for data analysis", []string{"pandas"}},
		{"install requests library", []string{"requests"}},
		{"add flask and sqlalchemy", []string{"flask", "sqlalchemy"}},
	}

	for _, tt := range tests {
		refs := ed.ExtractPackageRefs(tt.input)
		for _, exp := range tt.expected {
			found := false
			for _, ref := range refs {
				if strings.EqualFold(ref, exp) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ExtractPackageRefs(%q): expected %q in results %v", tt.input, exp, refs)
			}
		}
	}
}

func TestExtractPackageRefs_JSPackages(t *testing.T) {
	ed := NewExternalDocs()

	tests := []struct {
		input    string
		expected []string
	}{
		{"use express for the API server", []string{"express"}},
		{"add lodash utility functions", []string{"lodash"}},
		{"install prisma ORM", []string{"prisma"}},
		{"integrate react with redux", []string{"react", "redux"}},
	}

	for _, tt := range tests {
		refs := ed.ExtractPackageRefs(tt.input)
		for _, exp := range tt.expected {
			found := false
			for _, ref := range refs {
				if strings.EqualFold(ref, exp) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ExtractPackageRefs(%q): expected %q in results %v", tt.input, exp, refs)
			}
		}
	}
}

func TestExtractPackageRefs_RustPackages(t *testing.T) {
	ed := NewExternalDocs()

	tests := []struct {
		input    string
		expected []string
	}{
		{"use tokio for async runtime", []string{"tokio"}},
		{"add serde for serialization", []string{"serde"}},
		{"integrate axum web framework", []string{"axum"}},
	}

	for _, tt := range tests {
		refs := ed.ExtractPackageRefs(tt.input)
		for _, exp := range tt.expected {
			found := false
			for _, ref := range refs {
				if strings.EqualFold(ref, exp) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ExtractPackageRefs(%q): expected %q in results %v", tt.input, exp, refs)
			}
		}
	}
}

func TestExtractPackageRefs_Empty(t *testing.T) {
	ed := NewExternalDocs()

	refs := ed.ExtractPackageRefs("")
	if refs != nil {
		t.Errorf("expected nil for empty input, got %v", refs)
	}

	refs = ed.ExtractPackageRefs("hello world no packages here")
	if len(refs) != 0 {
		t.Errorf("expected no refs for generic text, got %v", refs)
	}
}

func TestFindRelevant_Go(t *testing.T) {
	ed := NewExternalDocs()

	results := ed.FindRelevant("build an HTTP API with chi router", "go", 5)
	if len(results) == 0 {
		t.Fatal("expected results for chi router task")
	}

	found := false
	for _, r := range results {
		if strings.Contains(r.URL, "chi") {
			found = true
			if r.Relevance <= 0 {
				t.Error("expected positive relevance")
			}
			if r.Source != "pkg.go.dev" {
				t.Errorf("expected source pkg.go.dev, got %s", r.Source)
			}
			break
		}
	}
	if !found {
		t.Error("expected chi in results")
	}
}

func TestFindRelevant_LanguageFilter(t *testing.T) {
	ed := NewExternalDocs()

	// When filtering for Go, should not return Python packages
	results := ed.FindRelevant("use flask web framework", "go", 10)
	for _, r := range results {
		if strings.Contains(r.Title, "flask") {
			t.Error("should not return flask when filtering for go")
		}
	}

	// Python filter should find flask
	results = ed.FindRelevant("use flask web framework", "python", 10)
	found := false
	for _, r := range results {
		if strings.Contains(strings.ToLower(r.Title), "flask") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected flask in python results")
	}
}

func TestFindRelevant_NoLanguageFilter(t *testing.T) {
	ed := NewExternalDocs()

	// With empty language, should search all sources
	results := ed.FindRelevant("use redis for caching", "", 10)
	if len(results) == 0 {
		t.Fatal("expected results for redis")
	}
}

func TestFindRelevant_Limit(t *testing.T) {
	ed := NewExternalDocs()

	results := ed.FindRelevant("use react redux lodash axios express next", "javascript", 2)
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestFindRelevant_ZeroLimit(t *testing.T) {
	ed := NewExternalDocs()

	// Zero limit should default to 5
	results := ed.FindRelevant("use chi gin echo cobra viper zap testify sqlx", "go", 0)
	if len(results) > 5 {
		t.Errorf("expected at most 5 results with zero limit, got %d", len(results))
	}
}

func TestFindRelevant_RelevanceSorted(t *testing.T) {
	ed := NewExternalDocs()

	results := ed.FindRelevant("use gin and zap for logging in gin web server", "go", 10)
	if len(results) < 2 {
		t.Skip("need at least 2 results to test sorting")
	}

	for i := 1; i < len(results); i++ {
		if results[i].Relevance > results[i-1].Relevance {
			t.Errorf("results not sorted by relevance: %f > %f at index %d",
				results[i].Relevance, results[i-1].Relevance, i)
		}
	}
}

func TestBuildDocContext_Basic(t *testing.T) {
	ed := NewExternalDocs()

	results := []DocResult{
		{
			Source:    "pkg.go.dev",
			Title:     "chi - pkg.go.dev",
			Content:   "Lightweight router for Go HTTP services.",
			URL:       "https://pkg.go.dev/chi",
			Relevance: 0.9,
			Tokens:    50,
		},
		{
			Source:    "pkg.go.dev",
			Title:     "zap - pkg.go.dev",
			Content:   "Blazing fast structured logging.",
			URL:       "https://pkg.go.dev/zap",
			Relevance: 0.7,
			Tokens:    45,
		},
	}

	ctx := ed.BuildDocContext(results, 1000)
	if ctx == "" {
		t.Fatal("expected non-empty context")
	}
	if !strings.Contains(ctx, "## Relevant Documentation") {
		t.Error("expected header in context")
	}
	if !strings.Contains(ctx, "chi") {
		t.Error("expected chi in context")
	}
	if !strings.Contains(ctx, "zap") {
		t.Error("expected zap in context")
	}
}

func TestBuildDocContext_Budget(t *testing.T) {
	ed := NewExternalDocs()

	// Create many results that exceed a tiny budget
	var results []DocResult
	for i := 0; i < 20; i++ {
		results = append(results, DocResult{
			Source:    "test",
			Title:     strings.Repeat("x", 100),
			Content:   strings.Repeat("y", 500),
			URL:       "https://example.com",
			Relevance: 0.5,
			Tokens:    200,
		})
	}

	ctx := ed.BuildDocContext(results, 50)
	// With a budget of 50 tokens, should not include all results
	if strings.Count(ctx, "###") >= 20 {
		t.Error("expected budget to limit results")
	}
}

func TestBuildDocContext_Empty(t *testing.T) {
	ed := NewExternalDocs()
	ctx := ed.BuildDocContext(nil, 1000)
	if ctx != "" {
		t.Errorf("expected empty string for nil results, got %q", ctx)
	}
}

func TestBuildDocContext_DefaultBudget(t *testing.T) {
	ed := NewExternalDocs()

	results := []DocResult{
		{Source: "test", Title: "Test", Content: "Content", URL: "http://x.com", Relevance: 0.5, Tokens: 10},
	}

	// Zero budget should use MaxTokens
	ctx := ed.BuildDocContext(results, 0)
	if ctx == "" {
		t.Error("expected non-empty context with zero budget (should use default)")
	}
}

func TestRegisterSource(t *testing.T) {
	ed := NewExternalDocs()
	initialCount := len(ed.Sources)

	ed.RegisterSource(DocSource{
		Name:     "custom-docs",
		BaseURL:  "https://custom.docs.io",
		Packages: []string{"my-pkg", "other-pkg"},
		Language: "go",
		Priority: 10,
	})

	if len(ed.Sources) != initialCount+1 {
		t.Errorf("expected %d sources after register, got %d", initialCount+1, len(ed.Sources))
	}

	// Verify the new source works in package extraction
	refs := ed.ExtractPackageRefs("use my-pkg for something")
	found := false
	for _, ref := range refs {
		if ref == "my-pkg" {
			found = true
			break
		}
	}
	if !found {
		t.Error("registered source package not found in extraction")
	}
}

func TestFormatResults_Empty(t *testing.T) {
	ed := NewExternalDocs()
	out := ed.FormatResults(nil)
	if out != "No relevant documentation found." {
		t.Errorf("unexpected output for nil results: %q", out)
	}
}

func TestFormatResults_WithResults(t *testing.T) {
	ed := NewExternalDocs()

	results := []DocResult{
		{
			Source:    "pkg.go.dev",
			Title:     "chi - pkg.go.dev",
			Content:   "Lightweight, idiomatic HTTP router for Go.",
			URL:       "https://pkg.go.dev/chi",
			Relevance: 0.85,
			Tokens:    50,
		},
		{
			Source:    "npmjs.com",
			Title:     "express - npmjs.com",
			Content:   "Fast, unopinionated, minimalist web framework.",
			URL:       "https://www.npmjs.com/package/express",
			Relevance: 0.75,
			Tokens:    60,
		},
	}

	out := ed.FormatResults(results)
	if !strings.Contains(out, "Found 2 relevant") {
		t.Error("expected count in output")
	}
	if !strings.Contains(out, "chi") {
		t.Error("expected chi in output")
	}
	if !strings.Contains(out, "express") {
		t.Error("expected express in output")
	}
	if !strings.Contains(out, "85%") {
		t.Error("expected relevance percentage")
	}
	if !strings.Contains(out, "pkg.go.dev") {
		t.Error("expected source name in output")
	}
}

func TestFormatResults_LongContent(t *testing.T) {
	ed := NewExternalDocs()

	results := []DocResult{
		{
			Source:    "test",
			Title:     "Long Content Test",
			Content:   strings.Repeat("a", 200),
			URL:       "https://example.com",
			Relevance: 0.5,
			Tokens:    100,
		},
	}

	out := ed.FormatResults(results)
	if !strings.Contains(out, "...") {
		t.Error("expected long content to be truncated with ellipsis")
	}
}

func TestBuildDocURL(t *testing.T) {
	tests := []struct {
		srcName  string
		pkg      string
		expected string
	}{
		{"pkg.go.dev", "chi", "https://pkg.go.dev/chi"},
		{"docs.python.org", "json", "https://docs.python.org/3/library/json.html"},
		{"pypi", "flask", "https://pypi.org/project/flask/"},
		{"npmjs.com", "express", "https://www.npmjs.com/package/express"},
		{"docs.rs", "tokio", "https://docs.rs/tokio/latest/tokio/"},
		{"nodejs.org", "fs", "https://nodejs.org/api/fs.html"},
	}

	for _, tt := range tests {
		src := DocSource{Name: tt.srcName, BaseURL: ""}
		url := buildDocURL(src, tt.pkg)
		if url != tt.expected {
			t.Errorf("buildDocURL(%s, %s) = %q, want %q", tt.srcName, tt.pkg, url, tt.expected)
		}
	}
}

func TestComputeRelevance(t *testing.T) {
	// Package mentioned in task should get higher score
	score1 := computeRelevance("use chi router", "chi", 9)
	score2 := computeRelevance("build a web server", "chi", 9)

	if score1 <= score2 {
		t.Errorf("direct mention should have higher relevance: %f <= %f", score1, score2)
	}

	// Higher priority source should get higher score
	score3 := computeRelevance("use redis", "redis", 9)
	score4 := computeRelevance("use redis", "redis", 1)

	if score3 <= score4 {
		t.Errorf("higher priority should have higher relevance: %f <= %f", score3, score4)
	}

	// Score should never exceed 1.0
	score5 := computeRelevance("chi chi chi chi", "chi", 100)
	if score5 > 1.0 {
		t.Errorf("relevance should be capped at 1.0, got %f", score5)
	}
}

func TestExternalDocs_ConcurrentAccess(t *testing.T) {
	ed := NewExternalDocs()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			ed.FindRelevant("use chi router", "go", 5)
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		ed.ExtractPackageRefs("import flask")
	}

	<-done

	// Also test concurrent register
	done2 := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			ed.RegisterSource(DocSource{
				Name:     "concurrent-test",
				BaseURL:  "https://example.com",
				Packages: []string{"pkg-x"},
				Language: "go",
				Priority: 1,
			})
		}
		close(done2)
	}()

	for i := 0; i < 50; i++ {
		ed.FindRelevant("use pkg-x", "go", 3)
	}

	<-done2
}

func TestEstimateDocTokens(t *testing.T) {
	short := estimateDocTokens("io")
	long := estimateDocTokens("google-cloud-go")

	if long <= short {
		t.Errorf("longer package name should estimate more tokens: %d <= %d", long, short)
	}
}

func TestExtractWords(t *testing.T) {
	words := extractWords("use chi router and add gin framework")
	if !words["chi"] {
		t.Error("expected 'chi' in words")
	}
	if !words["gin"] {
		t.Error("expected 'gin' in words")
	}
	if !words["router"] {
		t.Error("expected 'router' in words")
	}
}

func TestFindRelevant_CommonLanguage(t *testing.T) {
	ed := NewExternalDocs()

	// Common language sources should be included regardless of language filter
	results := ed.FindRelevant("deploy to kubernetes with docker", "go", 10)
	foundCommon := false
	for _, r := range results {
		if r.Source == "github" {
			foundCommon = true
			break
		}
	}
	if !foundCommon {
		t.Error("expected common source (github) in results when filtering by language")
	}
}

func TestFindRelevant_NoResults(t *testing.T) {
	ed := NewExternalDocs()

	results := ed.FindRelevant("write a hello world program", "go", 5)
	// "hello world" doesn't match any packages - results may be nil or empty
	if len(results) != 0 {
		// Verify they're reasonable at least
		for _, r := range results {
			if r.Relevance <= 0 {
				t.Error("result with non-positive relevance")
			}
		}
	}
}

func TestMatchesPackageRef(t *testing.T) {
	words := map[string]bool{"chi": true, "router": true}

	if !matchesPackageRef("use chi router", words, "chi") {
		t.Error("should match 'chi' as direct word")
	}

	if matchesPackageRef("use chi router", words, "flask") {
		t.Error("should not match 'flask'")
	}

	// Pattern matching
	words2 := map[string]bool{"the": true, "server": true}
	if !matchesPackageRef("import flask", words2, "flask") {
		t.Error("should match 'flask' via import pattern")
	}
}
