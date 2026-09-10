package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsImageExtension(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{".png", true},
		{".jpg", true},
		{".jpeg", true},
		{".gif", true},
		{".webp", true},
		{".bmp", true},
		{".PNG", false}, // case-sensitive check
		{".txt", false},
		{".go", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isImageExtension(tc.ext); got != tc.want {
			t.Errorf("isImageExtension(%q) = %v, want %v", tc.ext, got, tc.want)
		}
	}
}

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"screenshot.png", true},
		{"photo.jpg", true},
		{"image.jpeg", true},
		{"anim.gif", true},
		{"icon.webp", true},
		{"bitmap.bmp", true},
		{"readme.md", false},
		{"main.go", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := IsImageFile(tc.path); got != tc.want {
			t.Errorf("IsImageFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestReadImageFile(t *testing.T) {
	// Create a temp image file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	if err := os.WriteFile(path, []byte("fake png data"), 0o644); err != nil {
		t.Fatal(err)
	}

	att, err := ReadImageFile(path)
	if err != nil {
		t.Fatalf("ReadImageFile: %v", err)
	}
	if att.Base64 == "" {
		t.Error("Base64 should not be empty")
	}
	if att.MIMEType != "image/png" {
		t.Errorf("MIMEType = %q, want image/png", att.MIMEType)
	}
}

func TestReadImageFileNotFound(t *testing.T) {
	_, err := ReadImageFile("/nonexistent/image.png")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReadImageFileUnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadImageFile(path)
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestReadImageBytes(t *testing.T) {
	data := []byte("fake image data")
	att := ReadImageBytes(data, "image/jpeg")
	if att.Base64 == "" {
		t.Error("Base64 should not be empty")
	}
	if att.MIMEType != "image/jpeg" {
		t.Errorf("MIMEType = %q, want image/jpeg", att.MIMEType)
	}
}

func TestReadImageBytesDefaultMIME(t *testing.T) {
	att := ReadImageBytes([]byte("data"), "")
	if att.MIMEType != "image/png" {
		t.Errorf("MIMEType = %q, want image/png (default)", att.MIMEType)
	}
}

func TestFormatImageMessage(t *testing.T) {
	msg := FormatImageMessage("Look at this", "/path/to/screenshot.png")
	if msg == "" {
		t.Error("message should not be empty")
	}
	if !strings.Contains(msg, "screenshot.png") {
		t.Error("message should contain filename")
	}
}

// minimalPNG is a valid 1x1 PNG (decodable by image.DecodeConfig).
var minimalPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00,
	0x1f, 0x15, 0xc4, 0x89,
	0x00, 0x00, 0x00, 0x0d, 'I', 'D', 'A', 'T', 'x', 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01,
	0x0d, 0x0a, 0x2d, 0xb4,
	0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

func TestEmitTerminalImageFallsBackWhenUnsupported(t *testing.T) {
	att := ReadImageBytes(minimalPNG, "image/png")
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	t.Setenv("KITTY_PID", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("GHOSTTY_RESOURCES_DIR", "")
	emitted, err := emitTerminalImage(att)
	if err != nil {
		t.Fatalf("emitTerminalImage: %v", err)
	}
	if emitted {
		t.Error("must not emit on an unsupported terminal")
	}
}

func TestEmitTerminalImageNonPNG(t *testing.T) {
	att := ReadImageBytes([]byte("jpeg bytes"), "image/jpeg")
	emitted, err := emitTerminalImage(att)
	if err != nil {
		t.Fatalf("emitTerminalImage: %v", err)
	}
	if emitted {
		t.Error("non-PNG attachments must not emit")
	}
}

func TestEmitTerminalImageEmitsOnKitty(t *testing.T) {
	t.Setenv("KITTY_PID", "1234")
	att := ReadImageBytes(minimalPNG, "image/png")
	emitted, err := emitTerminalImage(att)
	if err != nil {
		t.Fatalf("emitTerminalImage: %v", err)
	}
	if !emitted {
		t.Error("must emit on a kitty terminal")
	}
}
