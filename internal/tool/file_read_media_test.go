package tool

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestImageDimensions(t *testing.T) {
	// Create a small 4x3 PNG
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	w, h := getImageDimensions(buf.Bytes())
	if w != 4 || h != 3 {
		t.Fatalf("expected 4x3, got %dx%d", w, h)
	}
}

func TestIsSupportedImage(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"photo.png", true},
		{"photo.jpg", true},
		{"photo.jpeg", true},
		{"photo.gif", true},
		{"photo.webp", true},
		{"photo.svg", true},
		{"photo.PNG", true},
		{"doc.txt", false},
		{"code.go", false},
		{"data.pdf", false},
	}
	for _, tt := range tests {
		// Create a temp file with the extension to test
		tmp := t.TempDir()
		p := filepath.Join(tmp, tt.path)
		os.WriteFile(p, []byte("x"), 0o644)
		got := isImageFile(p)
		if got != tt.want {
			t.Errorf("isImageFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
