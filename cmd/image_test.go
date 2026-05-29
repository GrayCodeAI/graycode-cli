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
