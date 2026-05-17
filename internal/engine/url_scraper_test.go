package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDetectURLs_HTTPAndHTTPS(t *testing.T) {
	scraper := NewURLScraper()
	text := "Check out https://example.com/docs and also http://api.example.org/v2/data for reference."
	urls := scraper.DetectURLs(text)

	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com/docs" {
		t.Errorf("expected https://example.com/docs, got %s", urls[0])
	}
	if urls[1] != "http://api.example.org/v2/data" {
		t.Errorf("expected http://api.example.org/v2/data, got %s", urls[1])
	}
}

func TestDetectURLs_FiltersBinaryURLs(t *testing.T) {
	scraper := NewURLScraper()
	text := `
		Here are some links:
		https://example.com/image.png
		https://example.com/photo.jpg
		https://example.com/video.mp4
		https://example.com/archive.zip
		https://example.com/docs/guide
		https://example.com/api.json
	`
	urls := scraper.DetectURLs(text)

	// Only non-binary URLs should remain.
	for _, u := range urls {
		for _, ext := range []string{".png", ".jpg", ".mp4", ".zip"} {
			if strings.HasSuffix(u, ext) {
				t.Errorf("binary URL should be filtered: %s", u)
			}
		}
	}

	// Should have the docs/guide and api.json URLs.
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs after filtering, got %d: %v", len(urls), urls)
	}
}

func TestDetectURLs_Deduplication(t *testing.T) {
	scraper := NewURLScraper()
	text := "Visit https://example.com and later again https://example.com for more info."
	urls := scraper.DetectURLs(text)

	if len(urls) != 1 {
		t.Fatalf("expected 1 URL after dedup, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com" {
		t.Errorf("expected https://example.com, got %s", urls[0])
	}
}

func TestDetectURLs_WithQueryStrings(t *testing.T) {
	scraper := NewURLScraper()
	text := "See https://example.com/search?q=golang&page=2#results for details."
	urls := scraper.DetectURLs(text)

	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d: %v", len(urls), urls)
	}
	if !strings.Contains(urls[0], "q=golang") {
		t.Errorf("expected URL with query string, got %s", urls[0])
	}
	if !strings.Contains(urls[0], "page=2") {
		t.Errorf("expected URL with page param, got %s", urls[0])
	}
}

func TestDetectURLs_NoURLs(t *testing.T) {
	scraper := NewURLScraper()
	text := "This text has no links at all."
	urls := scraper.DetectURLs(text)

	if urls != nil {
		t.Errorf("expected nil, got %v", urls)
	}
}

func TestExtractHTML_StripsTags(t *testing.T) {
	htmlBody := `<html>
<head><title>Test Page</title></head>
<body>
<script>var x = 1;</script>
<style>.foo { color: red; }</style>
<h1>Hello World</h1>
<p>This is a <strong>test</strong> paragraph.</p>
<pre><code>func main() { fmt.Println("hi") }</code></pre>
</body>
</html>`

	title, content := ExtractHTML(htmlBody)

	if title != "Test Page" {
		t.Errorf("expected title 'Test Page', got '%s'", title)
	}
	if strings.Contains(content, "<script>") {
		t.Error("content should not contain script tags")
	}
	if strings.Contains(content, "<style>") {
		t.Error("content should not contain style tags")
	}
	if strings.Contains(content, "var x = 1") {
		t.Error("content should not contain script content")
	}
	if !strings.Contains(content, "Hello World") {
		t.Error("content should contain heading text")
	}
	if !strings.Contains(content, "test") {
		t.Error("content should contain paragraph text")
	}
	if !strings.Contains(content, "func main()") {
		t.Error("content should preserve code block content")
	}
}

func TestExtractHTML_MetaDescription(t *testing.T) {
	htmlBody := `<html>
<head>
<title>Empty Page</title>
<meta name="description" content="This is the meta description.">
</head>
<body></body>
</html>`

	title, content := ExtractHTML(htmlBody)

	if title != "Empty Page" {
		t.Errorf("expected title 'Empty Page', got '%s'", title)
	}
	// Body is empty so meta description should be used.
	if !strings.Contains(content, "meta description") {
		t.Errorf("expected meta description fallback, got '%s'", content)
	}
}

func TestExtractJSON_PrettyPrints(t *testing.T) {
	input := `{"name":"hawk","version":"1.0","active":true}`
	result := ExtractJSON(input)

	if !strings.Contains(result, "  ") {
		t.Error("expected indented JSON output")
	}
	if !strings.Contains(result, `"name": "hawk"`) {
		t.Error("expected formatted key-value pair")
	}
}

func TestExtractJSON_TruncatesArrays(t *testing.T) {
	items := make([]int, 20)
	for i := range items {
		items[i] = i
	}
	input, _ := json.Marshal(map[string]interface{}{"items": items})
	result := ExtractJSON(string(input))

	// Parse back to verify truncation.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	arr, ok := parsed["items"].([]interface{})
	if !ok {
		t.Fatal("expected items array")
	}
	if len(arr) != 5 {
		t.Errorf("expected array truncated to 5 elements, got %d", len(arr))
	}
}

func TestExtractJSON_InvalidJSON(t *testing.T) {
	input := "not valid json at all"
	result := ExtractJSON(input)
	if result != input {
		t.Errorf("expected original string returned for invalid JSON, got '%s'", result)
	}
}

func TestShouldAutoFetch_Allowlist(t *testing.T) {
	scraper := NewURLScraper()

	allowed := []string{
		"https://github.com/user/repo",
		"https://stackoverflow.com/questions/12345",
		"https://pkg.go.dev/net/http",
		"https://developer.mozilla.org/en-US/docs/Web",
		"https://docs.python.org/3/library/json.html",
		"https://docs.example.com/guide",
	}
	for _, u := range allowed {
		if !scraper.ShouldAutoFetch(u) {
			t.Errorf("expected ShouldAutoFetch=true for %s", u)
		}
	}
}

func TestShouldAutoFetch_Blocklist(t *testing.T) {
	scraper := NewURLScraper()

	blocked := []string{
		"https://youtube.com/watch?v=abc",
		"https://twitter.com/user/status/123",
		"https://x.com/user/status/456",
		"https://facebook.com/post/789",
		"https://www.instagram.com/p/abc",
		"https://www.reddit.com/r/golang",
	}
	for _, u := range blocked {
		if scraper.ShouldAutoFetch(u) {
			t.Errorf("expected ShouldAutoFetch=false for %s", u)
		}
	}
}

func TestShouldAutoFetch_Unknown(t *testing.T) {
	scraper := NewURLScraper()

	// Unknown domains should return false (not in allowlist).
	if scraper.ShouldAutoFetch("https://random-site.example.org/page") {
		t.Error("expected ShouldAutoFetch=false for unknown domain")
	}
}

func TestFormatForContext_Basic(t *testing.T) {
	scraper := NewURLScraper()
	result := &ScrapeResult{
		URL:     "https://example.com/docs",
		Title:   "Example Docs",
		Content: "This is the documentation content.",
	}

	formatted := scraper.FormatForContext(result, 1000)

	if !strings.Contains(formatted, "## Content from https://example.com/docs") {
		t.Error("expected header with URL")
	}
	if !strings.Contains(formatted, "Title: Example Docs") {
		t.Error("expected title line")
	}
	if !strings.Contains(formatted, "This is the documentation content.") {
		t.Error("expected content body")
	}
}

func TestFormatForContext_Truncation(t *testing.T) {
	scraper := NewURLScraper()
	longContent := strings.Repeat("word ", 1000)
	result := &ScrapeResult{
		URL:     "https://example.com",
		Title:   "Test",
		Content: longContent,
	}

	// maxTokens=10 means ~40 chars.
	formatted := scraper.FormatForContext(result, 10)

	if !strings.Contains(formatted, "... (truncated)") {
		t.Error("expected truncation marker")
	}
	// Should be much shorter than original.
	if len(formatted) > 200 {
		t.Errorf("formatted output too long: %d chars", len(formatted))
	}
}

func TestFormatForContext_NilResult(t *testing.T) {
	scraper := NewURLScraper()
	result := scraper.FormatForContext(nil, 1000)
	if result != "" {
		t.Errorf("expected empty string for nil result, got '%s'", result)
	}
}

func TestCacheHitAndMiss(t *testing.T) {
	scraper := NewURLScraper()

	// Cache miss.
	if got := scraper.CacheGet("https://example.com"); got != nil {
		t.Error("expected nil for cache miss")
	}

	// Cache set and hit.
	expected := &ScrapeResult{
		URL:       "https://example.com",
		Title:     "Example",
		Content:   "Hello",
		FetchedAt: time.Now(),
	}
	scraper.CacheSet("https://example.com", expected)

	got := scraper.CacheGet("https://example.com")
	if got == nil {
		t.Fatal("expected cache hit")
	}
	if got.Title != "Example" {
		t.Errorf("expected title 'Example', got '%s'", got.Title)
	}
	if got.Content != "Hello" {
		t.Errorf("expected content 'Hello', got '%s'", got.Content)
	}
}

func TestCachePreventsRefetch(t *testing.T) {
	// Set up a test server that counts requests.
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><head><title>Test</title></head><body>Hello</body></html>")
	}))
	defer ts.Close()

	scraper := NewURLScraper()
	ctx := context.Background()

	// First fetch should hit server.
	_, err := scraper.Fetch(ctx, ts.URL)
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 server call, got %d", callCount)
	}

	// Second fetch should hit cache.
	_, err = scraper.Fetch(ctx, ts.URL)
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 server call (cached), got %d", callCount)
	}
}

func TestFetch_HTMLContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<html><head><title>Go Docs</title></head><body><h1>Welcome</h1><p>Go is great.</p></body></html>`)
	}))
	defer ts.Close()

	scraper := NewURLScraper()
	result, err := scraper.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	if result.ContentType != "html" {
		t.Errorf("expected content type 'html', got '%s'", result.ContentType)
	}
	if result.Title != "Go Docs" {
		t.Errorf("expected title 'Go Docs', got '%s'", result.Title)
	}
	if !strings.Contains(result.Content, "Welcome") {
		t.Error("expected content to contain 'Welcome'")
	}
	if result.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", result.StatusCode)
	}
}

func TestFetch_JSONContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"hawk","items":[1,2,3,4,5,6,7,8]}`)
	}))
	defer ts.Close()

	scraper := NewURLScraper()
	result, err := scraper.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	if result.ContentType != "json" {
		t.Errorf("expected content type 'json', got '%s'", result.ContentType)
	}
	// Items should be truncated to 5.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		t.Fatalf("result content is not valid JSON: %v", err)
	}
	items := parsed["items"].([]interface{})
	if len(items) != 5 {
		t.Errorf("expected 5 items after truncation, got %d", len(items))
	}
}

func TestExtractCode(t *testing.T) {
	body := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}`
	result := ExtractCode(body, "https://raw.githubusercontent.com/user/repo/main/main.go")

	if !strings.HasPrefix(result, "```go") {
		t.Error("expected go code fence")
	}
	if !strings.Contains(result, "package main") {
		t.Error("expected code content preserved")
	}
	if !strings.HasSuffix(result, "```") {
		t.Error("expected closing code fence")
	}
}

func TestExtractMarkdown_Truncation(t *testing.T) {
	long := strings.Repeat("# Header\nSome content here.\n", 1000)
	result := ExtractMarkdown(long)

	if len(result) > 8100 { // 8000 + room for truncation message
		t.Errorf("expected truncated result, got %d chars", len(result))
	}
	if !strings.Contains(result, "... (truncated)") {
		t.Error("expected truncation marker")
	}
}

func TestExtractMarkdown_Short(t *testing.T) {
	short := "# Title\n\nSome content."
	result := ExtractMarkdown(short)
	if result != short {
		t.Errorf("expected unchanged content, got '%s'", result)
	}
}

func TestNewURLScraper_Defaults(t *testing.T) {
	s := NewURLScraper()
	if !s.Enabled {
		t.Error("expected Enabled=true")
	}
	if s.MaxSize != 1<<20 {
		t.Errorf("expected MaxSize=1MB, got %d", s.MaxSize)
	}
	if s.Timeout != 15*time.Second {
		t.Errorf("expected Timeout=15s, got %v", s.Timeout)
	}
	if s.UserAgent != "hawk/1.0" {
		t.Errorf("expected UserAgent='hawk/1.0', got '%s'", s.UserAgent)
	}
	if s.Cache == nil {
		t.Error("expected non-nil cache map")
	}
}

func TestDetectURLs_TrailingPunctuation(t *testing.T) {
	scraper := NewURLScraper()
	text := "Go to https://example.com/page. Also see https://other.org/path, and https://third.io!"
	urls := scraper.DetectURLs(text)

	expected := []string{
		"https://example.com/page",
		"https://other.org/path",
		"https://third.io",
	}
	if len(urls) != len(expected) {
		t.Fatalf("expected %d URLs, got %d: %v", len(expected), len(urls), urls)
	}
	for i, u := range urls {
		if u != expected[i] {
			t.Errorf("url[%d]: expected '%s', got '%s'", i, expected[i], u)
		}
	}
}

func TestTokenEstimate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		// 400 characters ~ 100 tokens.
		fmt.Fprint(w, strings.Repeat("abcd", 100))
	}))
	defer ts.Close()

	scraper := NewURLScraper()
	result, err := scraper.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	if result.TokenEstimate != 100 {
		t.Errorf("expected token estimate ~100, got %d", result.TokenEstimate)
	}
}
