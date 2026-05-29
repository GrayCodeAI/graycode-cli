package cmd

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
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
