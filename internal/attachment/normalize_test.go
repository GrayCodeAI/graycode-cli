package attachment

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// aPng is a minimal valid 1x1 PNG (deterministic, ~67 bytes).
var aPng = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func TestNormalizeImageSource_DataURI(t *testing.T) {
	t.Parallel()
	b64 := base64.StdEncoding.EncodeToString(aPng)
	uri := "data:image/png;base64," + b64
	img, err := NormalizeImageSource(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img.MediaType != MediaTypePNG {
		t.Errorf("mediaType = %s, want image/png", img.MediaType)
	}
	if string(img.Data) != string(aPng) {
		t.Errorf("decoded data mismatch")
	}
}

func TestNormalizeImageSource_RawBase64(t *testing.T) {
	t.Parallel()
	b64 := base64.StdEncoding.EncodeToString(aPng)
	img, err := NormalizeImageSource(b64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img.MediaType != MediaTypePNG {
		t.Errorf("mediaType = %s, want image/png (sniffed)", img.MediaType)
	}
	if string(img.Data) != string(aPng) {
		t.Errorf("decoded data mismatch")
	}
}

func TestNormalizeImageSource_URL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(aPng)
	}))
	defer srv.Close()

	img, err := NormalizeImageSource(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img.MediaType != MediaTypePNG {
		t.Errorf("mediaType = %s, want image/png", img.MediaType)
	}
	if string(img.Data) != string(aPng) {
		t.Errorf("fetched data mismatch")
	}
}

func TestNormalizeImageSource_URL_NoClient(t *testing.T) {
	t.Parallel()
	_, err := NormalizeImageSource("https://example.com/x.png", WithHTTPClient(nil))
	if err == nil {
		t.Fatal("expected error when HTTPClient is nil")
	}
	if !strings.Contains(err.Error(), "HTTP client") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNormalizeImageSource_URL_Non200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := NormalizeImageSource(srv.URL)
	if err == nil {
		t.Fatal("expected error for non-200")
	}
}

func TestNormalizeImageSource_LocalFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pic.png")
	if err := os.WriteFile(path, aPng, 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := NormalizeImageSource(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img.MediaType != MediaTypePNG {
		t.Errorf("mediaType = %s, want image/png", img.MediaType)
	}
	if img.Name != "pic.png" {
		t.Errorf("name = %q, want pic.png", img.Name)
	}
}

func TestNormalizeImageSource_AtFileMarker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pic.png")
	if err := os.WriteFile(path, aPng, 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := NormalizeImageSource("@" + path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img.MediaType != MediaTypePNG {
		t.Errorf("mediaType = %s, want image/png", img.MediaType)
	}
}

func TestNormalizeImageSource_LocalFilesDisabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "pic.png")
	if err := os.WriteFile(path, aPng, 0o644); err != nil {
		t.Fatal(err)
	}
	// path has no extension-implying base64 chars at boundaries, but it is a
	// path with a slash so it won't be treated as base64.
	_, err := NormalizeImageSource(path, WithAllowLocalFiles(false))
	if err == nil {
		t.Fatal("expected error when local files disabled")
	}
}

func TestNormalizeImageSource_Unrecognised(t *testing.T) {
	t.Parallel()
	_, err := NormalizeImageSource("not an image at all", WithAllowLocalFiles(false))
	if err == nil {
		t.Fatal("expected error for unrecognised source")
	}
}

func TestMediaTypeFromMIME(t *testing.T) {
	t.Parallel()
	cases := map[string]ImageMediaType{
		"image/png":  MediaTypePNG,
		"PNG":        MediaTypePNG,
		"image/jpeg": MediaTypeJPEG,
		"jpeg":       MediaTypeJPEG,
		"image/webp": MediaTypeWebP,
		"image/gif":  MediaTypeGIF,
		"text/html":  "",
		"":           "",
	}
	for in, want := range cases {
		if got := MediaTypeFromMIME(in); got != want {
			t.Errorf("MediaTypeFromMIME(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMediaTypeFromExtension(t *testing.T) {
	t.Parallel()
	cases := map[string]ImageMediaType{
		"a.png":  MediaTypePNG,
		"a.JPG":  MediaTypeJPEG,
		"a.webp": MediaTypeWebP,
		"a.gif":  MediaTypeGIF,
		"a.txt":  "",
		"a":      "",
	}
	for in, want := range cases {
		if got := MediaTypeFromExtension(in); got != want {
			t.Errorf("MediaTypeFromExtension(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksLikeBase64(t *testing.T) {
	t.Parallel()
	good := base64.StdEncoding.EncodeToString(aPng)
	bad := []string{
		"",                    // empty
		"not base64!!",        // has '!' — invalid alphabet
		"aGVsbG8",             // length not a multiple of 4
		"/etc/passwd",         // path (not multiple of 4, has '/')
		"https://example.com", // url (has ':' and '.')
	}
	if !looksLikeBase64(good) {
		t.Errorf("expected %q to look like base64", good)
	}
	for _, b := range bad {
		if looksLikeBase64(b) {
			t.Errorf("expected %q NOT to look like base64", b)
		}
	}
}
