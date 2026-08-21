package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultJinaBaseURL is the Jina Reader endpoint used to convert a web page
// into clean Markdown.
const DefaultJinaBaseURL = "https://r.jina.ai"

// JinaReader fetches web pages and returns them as clean Markdown via the
// Jina Reader API, adopting the browse flow from MiniMax-AI/minimax_search
// (page → Markdown → downstream LLM answering) without the Python server or
// tokenizer dependency.
//
// It is disabled until Enabled is set, so it never makes network calls unless
// a deployment opts in.
type JinaReader struct {
	// Enabled gates all network access; FetchMarkdown returns an error when false.
	Enabled bool
	// APIKey is the optional Jina bearer token (higher rate limits when set).
	APIKey string
	// BaseURL overrides the Reader endpoint (tests point this at a stub).
	BaseURL string
	// Timeout bounds a single fetch.
	Timeout time.Duration
	// MaxBytes caps the response body read.
	MaxBytes int64

	client *http.Client
}

// NewJinaReader creates a disabled JinaReader with conservative defaults.
func NewJinaReader() *JinaReader {
	return &JinaReader{
		Enabled:  false,
		BaseURL:  DefaultJinaBaseURL,
		Timeout:  60 * time.Second,
		MaxBytes: 4 << 20,
	}
}

// Available reports whether the reader will attempt fetches.
func (r *JinaReader) Available() bool { return r.Enabled }

// jinaRequest is the JSON body sent to the Reader endpoint.
type jinaRequest struct {
	URL string `json:"url"`
}

// jinaPreamblePrefixes are the metadata lines the Reader prepends in some
// response modes; they are stripped so callers receive pure Markdown.
var jinaPreamblePrefixes = []string{"Title:", "URL Source:", "Markdown Content:"}

// FetchMarkdown retrieves targetURL and returns its content as Markdown.
func (r *JinaReader) FetchMarkdown(ctx context.Context, targetURL string) (string, error) {
	if !r.Enabled {
		return "", fmt.Errorf("jina reader is not enabled")
	}
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return "", fmt.Errorf("jina reader: empty target URL")
	}
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		return "", fmt.Errorf("jina reader: target must be an http(s) URL: %q", targetURL)
	}

	base := r.BaseURL
	if base == "" {
		base = DefaultJinaBaseURL
	}
	body, err := json.Marshal(jinaRequest{URL: targetURL})
	if err != nil {
		return "", fmt.Errorf("jina reader: encode request: %w", err)
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, base, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("jina reader: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Return-Format", "markdown")
	req.Header.Set("X-Engine", "direct")
	req.Header.Set("X-Retain-Images", "none")
	req.Header.Set("X-Timeout", fmt.Sprintf("%d", int(timeout.Seconds())))
	if r.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.APIKey)
	}

	client := r.client
	if client == nil {
		client = &http.Client{Timeout: timeout + 10*time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("jina reader: fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("jina reader: status %d for %s", resp.StatusCode, targetURL)
	}

	max := r.MaxBytes
	if max <= 0 {
		max = 4 << 20
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, max))
	if err != nil {
		return "", fmt.Errorf("jina reader: read body: %w", err)
	}
	return stripJinaPreamble(string(raw)), nil
}

// stripJinaPreamble removes the "Title: / URL Source: / Markdown Content:"
// metadata block the Reader sometimes prepends, returning pure Markdown.
func stripJinaPreamble(s string) string {
	lines := strings.SplitN(s, "\n", -1)
	i := 0
	sawMeta := false
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			if sawMeta {
				i++ // blank separator inside/after the metadata block
				continue
			}
			break // leading blank lines before any meta — keep them out anyway
		}
		isMeta := false
		for _, p := range jinaPreamblePrefixes {
			if strings.HasPrefix(trimmed, p) {
				isMeta = true
				break
			}
		}
		if !isMeta {
			break
		}
		sawMeta = true
		i++
	}
	if !sawMeta {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(strings.Join(lines[i:], "\n"))
}
