package engine

import (
	"fmt"
	"strings"
)

// LargeResponseHandler chunks large tool outputs instead of truncating them.
// Provides pagination so the agent can request more if needed.
type LargeResponseHandler struct {
	MaxChunkSize int // max chars per chunk (default 8000)
	OverlapLines int // lines of overlap between chunks for context
}

// NewLargeResponseHandler creates a handler with defaults tuned for coding.
func NewLargeResponseHandler() *LargeResponseHandler {
	return &LargeResponseHandler{
		MaxChunkSize: 8000,
		OverlapLines: 3,
	}
}

// ChunkedResponse holds a paginated response.
type ChunkedResponse struct {
	Chunks     []string
	TotalChars int
	TotalPages int
	Current    int
}

// Process splits a large response into chunks. Returns the first chunk with metadata.
func (h *LargeResponseHandler) Process(content string) *ChunkedResponse {
	if len(content) <= h.MaxChunkSize {
		return &ChunkedResponse{
			Chunks:     []string{content},
			TotalChars: len(content),
			TotalPages: 1,
			Current:    1,
		}
	}

	lines := strings.Split(content, "\n")
	var chunks []string
	var current strings.Builder
	currentLen := 0

	for i, line := range lines {
		lineLen := len(line) + 1 // +1 for newline
		if currentLen+lineLen > h.MaxChunkSize && currentLen > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
			currentLen = 0
			// Add overlap from previous lines
			start := i - h.OverlapLines
			if start < 0 {
				start = 0
			}
			for j := start; j < i; j++ {
				current.WriteString(lines[j])
				current.WriteByte('\n')
				currentLen += len(lines[j]) + 1
			}
		}
		current.WriteString(line)
		current.WriteByte('\n')
		currentLen += lineLen
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return &ChunkedResponse{
		Chunks:     chunks,
		TotalChars: len(content),
		TotalPages: len(chunks),
		Current:    1,
	}
}

// FormatPage returns a chunk with page header/footer for the agent.
func (cr *ChunkedResponse) FormatPage(page int) string {
	if page < 1 || page > cr.TotalPages {
		return ""
	}
	chunk := cr.Chunks[page-1]
	if cr.TotalPages == 1 {
		return chunk
	}
	header := fmt.Sprintf("[Page %d/%d | %d total chars]\n", page, cr.TotalPages, cr.TotalChars)
	footer := ""
	if page < cr.TotalPages {
		footer = fmt.Sprintf("\n[... %d more page(s) available]", cr.TotalPages-page)
	}
	return header + chunk + footer
}
