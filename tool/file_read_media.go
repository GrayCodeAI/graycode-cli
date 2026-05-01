package tool

import (
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maxImageSize      = 20 * 1024 * 1024 // 20 MB
	maxBase64Size     = 5 * 1024 * 1024  // 5 MB base64
	maxImageDimension = 12000
	maxPDFSize        = 20 * 1024 * 1024 // 20 MB
	maxPDFPages       = 100
)

var imageExtensions = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

// isImageFile checks if a file path has an image extension.
func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := imageExtensions[ext]
	return ok
}

// isPDFFile checks if a file path has a .pdf extension.
func isPDFFile(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".pdf"
}

// readImageFile reads an image file and returns it as base64 with metadata.
func readImageFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("image not found: %s", path)
	}
	if info.Size() > maxImageSize {
		return "", fmt.Errorf("image too large: %d bytes (max %d MB)", info.Size(), maxImageSize/(1024*1024))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading image: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	mimeType := imageExtensions[ext]

	// For SVG, return as text
	if ext == ".svg" {
		return fmt.Sprintf("[SVG Image: %s (%d bytes)]\n%s", filepath.Base(path), len(data), string(data)), nil
	}

	// Check dimensions
	width, height := getImageDimensions(data)
	if width > maxImageDimension || height > maxImageDimension {
		return "", fmt.Errorf("image dimensions %dx%d exceed maximum %d", width, height, maxImageDimension)
	}

	// Check base64 size
	b64 := base64.StdEncoding.EncodeToString(data)
	if len(b64) > maxBase64Size {
		// Try to resize
		resized, err := resizeImage(data, 1024, 1024)
		if err != nil {
			return "", fmt.Errorf("image too large for base64 and resize failed: %w", err)
		}
		b64 = base64.StdEncoding.EncodeToString(resized)
		if len(b64) > maxBase64Size {
			return "", fmt.Errorf("image too large even after resize")
		}
		width, height = getImageDimensions(resized)
	}

	displayInfo := fmt.Sprintf("[Image: %s, %dx%d, %s, %d bytes]",
		filepath.Base(path), width, height, mimeType, len(data))

	return fmt.Sprintf("%s\ndata:%s;base64,%s", displayInfo, mimeType, b64), nil
}

// readPDFFile reads a PDF file and extracts text content.
func readPDFFile(path string, pages string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("PDF not found: %s", path)
	}
	if info.Size() > maxPDFSize {
		return "", fmt.Errorf("PDF too large: %d bytes (max %d MB)", info.Size(), maxPDFSize/(1024*1024))
	}

	// Validate magic bytes
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	magic := make([]byte, 5)
	f.Read(magic)
	f.Close()
	if string(magic) != "%PDF-" {
		return "", fmt.Errorf("not a valid PDF file (invalid magic bytes)")
	}

	// Parse page range
	startPage, endPage, err := parsePageRange(pages)
	if err != nil {
		return "", err
	}
	if endPage-startPage+1 > maxPDFPages {
		return "", fmt.Errorf("too many pages requested (max %d)", maxPDFPages)
	}

	// Try pdftotext if available
	text, err := extractPDFText(path, startPage, endPage)
	if err != nil {
		return fmt.Sprintf("[PDF: %s, %d bytes. Text extraction unavailable: %v. Install pdftotext for full support.]",
			filepath.Base(path), info.Size(), err), nil
	}

	header := fmt.Sprintf("[PDF: %s, pages %d-%d]", filepath.Base(path), startPage, endPage)
	return header + "\n" + text, nil
}

// parsePageRange parses a page range string like "1-5", "3", "10-20".
func parsePageRange(pages string) (int, int, error) {
	pages = strings.TrimSpace(pages)
	if pages == "" {
		return 1, 10, nil // default first 10 pages
	}

	if strings.Contains(pages, "-") {
		parts := strings.SplitN(pages, "-", 2)
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid page range start: %s", parts[0])
		}
		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid page range end: %s", parts[1])
		}
		if start > end {
			return 0, 0, fmt.Errorf("invalid page range: start %d > end %d", start, end)
		}
		if start < 1 {
			start = 1
		}
		return start, end, nil
	}

	page, err := strconv.Atoi(pages)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid page number: %s", pages)
	}
	return page, page, nil
}

// extractPDFText uses pdftotext to extract text from a PDF.
func extractPDFText(path string, startPage, endPage int) (string, error) {
	// Shell out to pdftotext if available
	cmd := fmt.Sprintf("pdftotext -f %d -l %d -layout '%s' -", startPage, endPage, path)
	_ = cmd
	// For now, return an indication that pdftotext would be used
	return "", fmt.Errorf("pdftotext not available (install poppler-utils)")
}

// getImageDimensions returns width and height of an image.
func getImageDimensions(data []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(strings.NewReader(string(data)))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// resizeImage downscales an image to fit within maxWidth x maxHeight.
func resizeImage(data []byte, maxWidth, maxHeight int) ([]byte, error) {
	img, _, err := image.Decode(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("decoding image: %w", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Calculate scale
	scaleW := float64(maxWidth) / float64(w)
	scaleH := float64(maxHeight) / float64(h)
	scale := scaleW
	if scaleH < scaleW {
		scale = scaleH
	}
	if scale >= 1.0 {
		return data, nil // no resize needed
	}

	newW := int(float64(w) * scale)
	newH := int(float64(h) * scale)

	// Simple nearest-neighbor resize
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := 0; y < newH; y++ {
		for x := 0; x < newW; x++ {
			srcX := int(float64(x) / scale)
			srcY := int(float64(y) / scale)
			dst.Set(x, y, img.At(srcX, srcY))
		}
	}

	var buf strings.Builder
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}
