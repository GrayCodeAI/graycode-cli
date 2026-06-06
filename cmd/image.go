package cmd

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ImageAttachment represents an image ready to be attached to a message.
type ImageAttachment struct {
	Base64   string
	MIMEType string
	FilePath string
}

// ReadImageFile reads an image file and returns a base64-encoded attachment.
// Supports common image formats: png, jpg, jpeg, gif, webp, bmp.
func ReadImageFile(path string) (*ImageAttachment, error) {
	clean := filepath.Clean(path)

	// Check file exists
	info, err := os.Stat(clean)
	if err != nil {
		return nil, fmt.Errorf("image file not found: %s", path)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, not an image: %s", path)
	}

	// Check extension
	ext := strings.ToLower(filepath.Ext(clean))
	if !isImageExtension(ext) {
		return nil, fmt.Errorf("unsupported image format: %s (supported: png, jpg, jpeg, gif, webp, bmp)", ext)
	}

	// Read file
	data, err := os.ReadFile(clean)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	// Detect MIME type
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "image/png" // default
	}

	return &ImageAttachment{
		Base64:   base64.StdEncoding.EncodeToString(data),
		MIMEType: mimeType,
		FilePath: clean,
	}, nil
}

// ReadImageBytes creates an ImageAttachment from raw bytes.
func ReadImageBytes(data []byte, mimeType string) *ImageAttachment {
	if mimeType == "" {
		mimeType = "image/png"
	}
	return &ImageAttachment{
		Base64:   base64.StdEncoding.EncodeToString(data),
		MIMEType: mimeType,
	}
}

// isImageExtension returns true if the extension is a supported image format.
func isImageExtension(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

// IsImageFile returns true if the path points to a supported image file.
func IsImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return isImageExtension(ext)
}

// FormatImageMessage formats a user message with image attachment info for display.
func FormatImageMessage(content string, imgPath string) string {
	if content == "" {
		return fmt.Sprintf("📷 [Image: %s]", filepath.Base(imgPath))
	}
	return fmt.Sprintf("%s\n📷 [Image: %s]", content, filepath.Base(imgPath))
}

// extractImagePath looks for image file references in user input.
// It checks for:
//   - Direct file paths ending in image extensions
//   - @path/to/image.png syntax
//   - Drag-and-drop paths (quoted or unquoted)
//   - Paths with spaces (common in drag-and-drop)
//
// Returns empty string if no image is found.
func extractImagePath(text string) string {
	// Check for @-mentioned image paths
	if strings.Contains(text, "@") {
		parts := strings.Fields(text)
		for _, part := range parts {
			if strings.HasPrefix(part, "@") {
				candidate := strings.TrimPrefix(part, "@")
				candidate = strings.Trim(candidate, "\"'")
				if IsImageFile(candidate) {
					if _, err := os.Stat(candidate); err == nil {
						return candidate
					}
				}
			}
		}
	}

	// Check for direct file paths (possibly quoted)
	words := strings.Fields(text)
	for _, word := range words {
		clean := strings.Trim(word, "\"'")
		if IsImageFile(clean) {
			if _, err := os.Stat(clean); err == nil {
				return clean
			}
		}
	}

	// Check for drag-and-drop paths (often quoted with spaces)
	// Pattern: "path/to my image.png" or '/path/to my image.png'
	if strings.Contains(text, "\"") || strings.Contains(text, "'") {
		// Extract quoted strings
		quoted := extractQuotedPaths(text)
		for _, path := range quoted {
			if IsImageFile(path) {
				if _, err := os.Stat(path); err == nil {
					return path
				}
			}
		}
	}

	// Check if the entire text (trimmed) is an image path
	// This handles drag-and-drop where terminal pastes just the path
	trimmed := strings.TrimSpace(text)
	if IsImageFile(trimmed) {
		if _, err := os.Stat(trimmed); err == nil {
			return trimmed
		}
	}

	return ""
}

// IsPDFFile returns true if the path points to a PDF file.
func IsPDFFile(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".pdf"
}

// ReadPDFText reads a PDF and extracts its text content using stdlib only.
//
// eyrie does not expose a native document/PDF content block, so PDFs are
// degraded to text and injected inline. This is a best-effort extractor: it
// inflates FlateDecode content streams and pulls text from PDF text-showing
// operators (Tj / TJ). It does not handle every PDF (encrypted, scanned-image,
// or exotic font encodings will yield little or no text); callers should treat
// empty output as "no extractable text" rather than an error.
//
// TODO: swap in a dedicated PDF library (e.g. ledongthuc/pdf) for layout-aware
// extraction if one is added to go.mod; the interface here is intentionally
// stable so that change is internal.
func ReadPDFText(path string) (string, error) {
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("pdf file not found: %s", path)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a pdf: %s", path)
	}
	if !IsPDFFile(clean) {
		return "", fmt.Errorf("not a pdf file: %s", path)
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return "", fmt.Errorf("failed to read pdf: %w", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return "", fmt.Errorf("not a valid pdf (missing %%PDF header): %s", path)
	}
	return extractPDFText(data), nil
}

var (
	pdfStreamRe = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	// Text shown via (literal) Tj or [ ... ] TJ operators.
	pdfTjRe = regexp.MustCompile(`\((?:[^()\\]|\\.)*\)`)
)

// extractPDFText pulls readable text from raw PDF bytes. Exported behavior is
// covered by ReadPDFText; kept separate so tests can exercise byte input.
func extractPDFText(data []byte) string {
	var out strings.Builder
	for _, match := range pdfStreamRe.FindAllSubmatch(data, -1) {
		raw := match[1]
		content := raw
		// Try to inflate; if it isn't zlib-compressed, fall back to raw bytes.
		if inflated, ok := inflate(raw); ok {
			content = inflated
		}
		for _, lit := range pdfTjRe.FindAll(content, -1) {
			out.WriteString(decodePDFString(lit))
		}
	}
	return strings.TrimSpace(collapsePDFWhitespace(out.String()))
}

// inflate attempts zlib (FlateDecode) decompression of a PDF stream body.
func inflate(b []byte) ([]byte, bool) {
	r, err := zlib.NewReader(bytes.NewReader(bytes.TrimSpace(b)))
	if err != nil {
		return nil, false
	}
	defer func() { _ = r.Close() }()
	out, err := io.ReadAll(r)
	if err != nil || len(out) == 0 {
		return nil, false
	}
	return out, true
}

// decodePDFString converts a PDF literal string token like "(Hello\)world)"
// into its plain text, handling the common backslash escapes.
func decodePDFString(lit []byte) string {
	s := string(lit)
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '(', ')', '\\':
				b.WriteByte(next)
			default:
				b.WriteByte(next)
			}
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String() + " "
}

// collapsePDFWhitespace squeezes runs of whitespace so extracted text reads
// cleanly when injected into a prompt.
func collapsePDFWhitespace(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = r == '\n'
		b.WriteRune(r)
	}
	return b.String()
}

// extractPDFPath looks for a PDF file reference in user input, mirroring
// extractImagePath: it checks @-mentions, bare words, and quoted paths.
// Returns empty string if no existing PDF is found.
func extractPDFPath(text string) string {
	consider := func(candidate string) string {
		candidate = strings.Trim(candidate, "\"'")
		if IsPDFFile(candidate) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
		return ""
	}
	for _, part := range strings.Fields(text) {
		if strings.HasPrefix(part, "@") {
			if p := consider(strings.TrimPrefix(part, "@")); p != "" {
				return p
			}
		}
		if p := consider(part); p != "" {
			return p
		}
	}
	for _, q := range extractQuotedPaths(text) {
		if p := consider(q); p != "" {
			return p
		}
	}
	if p := consider(strings.TrimSpace(text)); p != "" {
		return p
	}
	return ""
}

// extractQuotedPaths extracts file paths from quoted strings in text.
func extractQuotedPaths(text string) []string {
	var paths []string
	inQuote := false
	quoteChar := byte(0)
	start := 0

	for i := 0; i < len(text); i++ {
		if !inQuote {
			if text[i] == '"' || text[i] == '\'' {
				inQuote = true
				quoteChar = text[i]
				start = i + 1
			}
		} else {
			if text[i] == quoteChar {
				paths = append(paths, text[start:i])
				inQuote = false
			}
		}
	}

	return paths
}
