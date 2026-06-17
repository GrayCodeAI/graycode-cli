package search

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// URLScraper detects URLs in conversation text and fetches/extracts their content.
type URLScraper struct {
	Enabled   bool
	MaxSize   int64
	Timeout   time.Duration
	UserAgent string
	Cache     map[string]*ScrapeResult
	mu        sync.RWMutex
}

// ScrapeResult holds the extracted content from a fetched URL.
type ScrapeResult struct {
	URL           string
	Title         string
	Content       string
	ContentType   string // "html", "json", "text", "code", "markdown"
	StatusCode    int
	FetchedAt     time.Time
	TokenEstimate int
}

// NewURLScraper creates a URLScraper with default settings.
func NewURLScraper() *URLScraper {
	return &URLScraper{
		Enabled:   true,
		MaxSize:   1 << 20, // 1MB
		Timeout:   15 * time.Second,
		UserAgent: "hawk/1.0",
		Cache:     make(map[string]*ScrapeResult),
	}
}

// scraperURLPattern matches http and https URLs in text.
var scraperURLPattern = regexp.MustCompile(`https?://[^\s<>"'\` + "`" + `\)\]\}]+`)

// binaryExtensions are file extensions that should not be fetched.
var binaryExtensions = []string{
	".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg", ".ico", ".webp",
	".mp4", ".mp3", ".avi", ".mov", ".wmv", ".flv", ".webm", ".mkv",
	".zip", ".tar", ".gz", ".bz2", ".7z", ".rar", ".xz",
	".exe", ".dll", ".so", ".dylib", ".bin",
	".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
	".woff", ".woff2", ".ttf", ".eot", ".otf",
}

// DetectURLs finds all URLs in text, deduplicates them, and filters out binary URLs.
func (s *URLScraper) DetectURLs(text string) []string {
	matches := scraperURLPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var results []string
	for _, u := range matches {
		// Trim trailing punctuation that may have been captured.
		u = strings.TrimRight(u, ".,;:!?")
		if seen[u] {
			continue
		}
		if isBinaryURL(u) {
			continue
		}
		seen[u] = true
		results = append(results, u)
	}
	return results
}

// isBinaryURL checks if the URL points to a binary/media file.
func isBinaryURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	lower := strings.ToLower(parsed.Path)
	for _, ext := range binaryExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// Fetch retrieves the URL content, respecting timeout and size limits.
func (s *URLScraper) Fetch(ctx context.Context, rawURL string) (*ScrapeResult, error) {
	// Check cache first.
	if cached := s.CacheGet(rawURL); cached != nil {
		return cached, nil
	}

	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", s.UserAgent)
	req.Header.Set("Accept", "text/html, application/json, text/plain, */*")

	client := &http.Client{Timeout: s.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching URL: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Limit read size.
	limited := io.LimitReader(resp.Body, s.MaxSize)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	bodyStr := string(body)
	ct := determineContentType(resp.Header.Get("Content-Type"), rawURL, bodyStr)

	result := &ScrapeResult{
		URL:         rawURL,
		StatusCode:  resp.StatusCode,
		ContentType: ct,
		FetchedAt:   time.Now(),
	}

	switch ct {
	case "html":
		result.Title, result.Content = ExtractHTML(bodyStr)
	case "json":
		result.Content = ExtractJSON(bodyStr)
	case "markdown":
		result.Content = ExtractMarkdown(bodyStr)
	case "code":
		result.Content = ExtractCode(bodyStr, rawURL)
	default:
		result.Content = bodyStr
	}

	// Estimate tokens (~4 chars per token).
	result.TokenEstimate = len(result.Content) / 4

	s.CacheSet(rawURL, result)
	return result, nil
}

// determineContentType classifies the response based on headers and URL.
func determineContentType(header, rawURL, body string) string {
	lower := strings.ToLower(header)

	if strings.Contains(lower, "application/json") {
		return "json"
	}
	if strings.Contains(lower, "text/html") {
		return "html"
	}
	if strings.Contains(lower, "text/markdown") {
		return "markdown"
	}

	// Infer from URL.
	parsed, _ := url.Parse(rawURL)
	if parsed != nil {
		path := strings.ToLower(parsed.Path)
		if strings.HasSuffix(path, ".md") {
			return "markdown"
		}
		if strings.HasSuffix(path, ".json") {
			return "json"
		}
		codeExts := []string{".go", ".py", ".js", ".ts", ".rs", ".c", ".cpp", ".java", ".rb", ".sh", ".yaml", ".yml", ".toml"}
		for _, ext := range codeExts {
			if strings.HasSuffix(path, ext) {
				return "code"
			}
		}
		// GitHub raw content.
		if strings.Contains(parsed.Host, "raw.githubusercontent.com") {
			return "code"
		}
	}

	// Try to detect JSON from body.
	trimmed := strings.TrimSpace(body)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		var js json.RawMessage
		if json.Unmarshal([]byte(trimmed), &js) == nil {
			return "json"
		}
	}

	return "text"
}

// ExtractHTML strips tags and extracts readable text from HTML.
func ExtractHTML(body string) (title, content string) {
	// Extract title.
	titleRe := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	if m := titleRe.FindStringSubmatch(body); len(m) > 1 {
		title = html.UnescapeString(strings.TrimSpace(m[1]))
	}

	// Extract meta description as fallback content hint.
	metaDesc := ""
	metaRe := regexp.MustCompile(`(?i)<meta[^>]+name=["']description["'][^>]+content=["']([^"']+)["']`)
	if m := metaRe.FindStringSubmatch(body); len(m) > 1 {
		metaDesc = html.UnescapeString(strings.TrimSpace(m[1]))
	}

	// Preserve code/pre block content.
	preBlocks := []string{}
	preRe := regexp.MustCompile(`(?is)<(pre|code)[^>]*>(.*?)</(pre|code)>`)
	for _, m := range preRe.FindAllStringSubmatch(body, -1) {
		cleaned := stripTags(m[2])
		cleaned = html.UnescapeString(cleaned)
		preBlocks = append(preBlocks, "```\n"+strings.TrimSpace(cleaned)+"\n```")
	}

	// Remove head, script and style tags and their content.
	headRe := regexp.MustCompile(`(?is)<head[^>]*>.*?</head>`)
	body = headRe.ReplaceAllString(body, "")
	scriptRe := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	body = scriptRe.ReplaceAllString(body, "")
	styleRe := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	body = styleRe.ReplaceAllString(body, "")

	// Strip all remaining HTML tags.
	text := stripTags(body)
	text = html.UnescapeString(text)

	// Collapse whitespace.
	spaceRe := regexp.MustCompile(`[ \t]+`)
	text = spaceRe.ReplaceAllString(text, " ")
	nlRe := regexp.MustCompile(`\n{3,}`)
	text = nlRe.ReplaceAllString(text, "\n\n")
	text = strings.TrimSpace(text)

	// Append preserved code blocks.
	if len(preBlocks) > 0 {
		text += "\n\n" + strings.Join(preBlocks, "\n\n")
	}

	// If content is empty, use meta description.
	if text == "" && metaDesc != "" {
		text = metaDesc
	}

	content = text
	return
}

// stripTags removes all HTML tags from a string.
func stripTags(s string) string {
	tagRe := regexp.MustCompile(`<[^>]*>`)
	return tagRe.ReplaceAllString(s, "\n")
}

// ExtractJSON pretty-prints JSON and truncates arrays to 5 elements.
func ExtractJSON(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	var raw interface{}
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		// Not valid JSON; return as-is.
		return body
	}

	// Truncate arrays.
	raw = truncateArrays(raw, 5)

	pretty, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return body
	}
	return string(pretty)
}

// truncateArrays recursively truncates arrays to maxLen elements.
func truncateArrays(v interface{}, maxLen int) interface{} {
	switch val := v.(type) {
	case []interface{}:
		truncated := val
		if len(val) > maxLen {
			truncated = val[:maxLen]
		}
		result := make([]interface{}, len(truncated))
		for i, item := range truncated {
			result[i] = truncateArrays(item, maxLen)
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, item := range val {
			result[k] = truncateArrays(item, maxLen)
		}
		return result
	default:
		return v
	}
}

// ExtractMarkdown returns the markdown body, truncated if too long.
func ExtractMarkdown(body string) string {
	const maxLen = 8000
	body = strings.TrimSpace(body)
	if len(body) > maxLen {
		return body[:maxLen] + "\n\n... (truncated)"
	}
	return body
}

// ExtractCode wraps raw code content in a fenced code block with language detection.
func ExtractCode(body string, rawURL string) string {
	lang := detectLanguageFromURL(rawURL)
	body = strings.TrimSpace(body)
	return fmt.Sprintf("```%s\n%s\n```", lang, body)
}

// detectLanguageFromURL guesses the programming language from the URL path extension.
func detectLanguageFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := parsed.Path
	extMap := map[string]string{
		".go":   "go",
		".py":   "python",
		".js":   "javascript",
		".ts":   "typescript",
		".rs":   "rust",
		".c":    "c",
		".cpp":  "cpp",
		".java": "java",
		".rb":   "ruby",
		".sh":   "bash",
		".yaml": "yaml",
		".yml":  "yaml",
		".toml": "toml",
		".json": "json",
		".html": "html",
		".css":  "css",
		".sql":  "sql",
	}
	for ext, lang := range extMap {
		if strings.HasSuffix(strings.ToLower(path), ext) {
			return lang
		}
	}
	return ""
}

// autoFetchAllowlist contains domains that should be automatically fetched.
var autoFetchAllowlist = []string{
	"github.com",
	"stackoverflow.com",
	"pkg.go.dev",
	"docs.rs",
	"docs.python.org",
	"docs.oracle.com",
	"docs.microsoft.com",
	"learn.microsoft.com",
	"developer.mozilla.org",
	"developer.apple.com",
	"wiki.archlinux.org",
	"en.wikipedia.org",
	"go.dev",
	"rust-lang.org",
	"npmjs.com",
	"pypi.org",
	"crates.io",
	"raw.githubusercontent.com",
	"gist.github.com",
}

// autoFetchBlocklist contains domains that should NOT be automatically fetched.
var autoFetchBlocklist = []string{
	"youtube.com",
	"youtu.be",
	"twitter.com",
	"x.com",
	"facebook.com",
	"instagram.com",
	"tiktok.com",
	"reddit.com",
	"linkedin.com",
	"discord.com",
	"twitch.tv",
	"spotify.com",
	"apple.com/music",
	"soundcloud.com",
}

// ShouldAutoFetch determines whether a URL should be automatically fetched.
func (s *URLScraper) ShouldAutoFetch(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Host)
	fullURL := strings.ToLower(rawURL)

	// Check blocklist first.
	for _, blocked := range autoFetchBlocklist {
		if strings.Contains(host, blocked) || strings.Contains(fullURL, blocked) {
			return false
		}
	}

	// Check allowlist.
	for _, allowed := range autoFetchAllowlist {
		if strings.Contains(host, allowed) {
			return true
		}
	}

	// Allow docs.* subdomains.
	if strings.HasPrefix(host, "docs.") {
		return true
	}

	return false
}

// FormatForContext formats a ScrapeResult for injection into agent context.
func (s *URLScraper) FormatForContext(result *ScrapeResult, maxTokens int) string {
	if result == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Content from %s\n", result.URL))
	if result.Title != "" {
		sb.WriteString(fmt.Sprintf("Title: %s\n", result.Title))
	}
	sb.WriteString("\n")

	content := result.Content
	// Truncate to approximate maxTokens (4 chars per token).
	maxChars := maxTokens * 4
	if maxChars > 0 && len(content) > maxChars {
		content = content[:maxChars] + "\n\n... (truncated)"
	}
	sb.WriteString(content)

	return sb.String()
}

// CacheGet retrieves a cached ScrapeResult for the given URL.
func (s *URLScraper) CacheGet(rawURL string) *ScrapeResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Cache[rawURL]
}

// CacheSet stores a ScrapeResult in the cache.
func (s *URLScraper) CacheSet(rawURL string, result *ScrapeResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Cache[rawURL] = result
}
