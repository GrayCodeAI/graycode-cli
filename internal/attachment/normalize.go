package attachment

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NormalizeImageError describes a failure to resolve a user-supplied image
// source (URL, data URI, raw base64, or local path) into durable bytes and a
// media type.
type NormalizeImageError struct {
	Reason string
}

func (e *NormalizeImageError) Error() string { return "normalize image: " + e.Reason }

// MediaTypeFromMIME narrows a wire MIME string to the durable raster
// vocabulary, returning "" for unsupported types. It accepts both the
// "image/png" form and the bare "png" form.
func MediaTypeFromMIME(value string) ImageMediaType {
	v := strings.ToLower(strings.TrimSpace(value))
	v = strings.TrimPrefix(v, "image/")
	for _, mt := range AllMediaTypes {
		if string(mt) == "image/"+v {
			return mt
		}
	}
	return ""
}

// MediaTypeFromExtension maps a file extension to an image media type, or ""
// if unrecognised.
func MediaTypeFromExtension(name string) ImageMediaType {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return MediaTypePNG
	case ".jpg", ".jpeg":
		return MediaTypeJPEG
	case ".webp":
		return MediaTypeWebP
	case ".gif":
		return MediaTypeGIF
	default:
		return ""
	}
}

// NormalizeImageSource resolves a user-supplied image source into bytes plus a
// media type ready for SaveImage, mirroring the source-normalization behaviour
// of the MiniMax Coding-Plan MCP server (URL / local path / data URI / raw
// base64 → canonical bytes). The recognised forms, in order:
//
//   - a "data:" URI (data:image/<fmt>;base64,<...>)
//   - an http(s) URL (fetched over the network)
//   - a raw base64 string (decoded directly)
//   - a local filesystem path (read from disk)
//
// A leading '@' on a bare value is treated as an explicit local-file marker
// and stripped, matching the MCP convention.
func NormalizeImageSource(src string, opts ...NormalizeOption) (SaveImage, error) {
	cfg := defaultNormalizeConfig()
	for _, o := range opts {
		o(&cfg)
	}

	src = strings.TrimSpace(src)

	// Explicit local-file marker: "@path".
	if strings.HasPrefix(src, "@") {
		return loadImageFile(strings.TrimPrefix(src, "@"), cfg)
	}

	// data: URI.
	if strings.HasPrefix(src, "data:") {
		return decodeDataURI(src)
	}

	// http(s) URL.
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		if cfg.HTTPClient == nil {
			return SaveImage{}, &NormalizeImageError{Reason: "image URL source requires an HTTP client (HTTPClient option)"}
		}
		return fetchImageURL(src, cfg)
	}

	// Raw base64 — only if it looks like base64 (to avoid confusing a file
	// path or plain text with an image).
	if looksLikeBase64(src) {
		data, err := base64.StdEncoding.DecodeString(src)
		if err != nil {
			return SaveImage{}, &NormalizeImageError{Reason: "source is neither a valid URL, data URI, nor base64: " + err.Error()}
		}
		return SaveImage{Data: data, MediaType: MediaTypeFromMIME(detectMIME(data))}, nil
	}

	// Fallback: local file path.
	if cfg.AllowLocalFiles {
		return loadImageFile(src, cfg)
	}
	return SaveImage{}, &NormalizeImageError{Reason: "source not recognised (and local files disabled)"}
}

// NormalizeOption customises NormalizeImageSource.
type NormalizeOption func(*normalizeConfig)

type normalizeConfig struct {
	HTTPClient      *http.Client
	AllowLocalFiles bool
}

func defaultNormalizeConfig() normalizeConfig {
	return normalizeConfig{
		HTTPClient:      &http.Client{Timeout: 30 * time.Second},
		AllowLocalFiles: true,
	}
}

// WithHTTPClient sets the HTTP client used to fetch URL sources.
func WithHTTPClient(hc *http.Client) NormalizeOption {
	return func(c *normalizeConfig) { c.HTTPClient = hc }
}

// WithAllowLocalFiles toggles reading image sources from the local filesystem.
func WithAllowLocalFiles(allow bool) NormalizeOption {
	return func(c *normalizeConfig) { c.AllowLocalFiles = allow }
}

func loadImageFile(path string, cfg normalizeConfig) (SaveImage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SaveImage{}, &NormalizeImageError{Reason: "read file: " + err.Error()}
	}
	mt := MediaTypeFromExtension(path)
	if mt == "" {
		mt = MediaTypeFromMIME(detectMIME(data))
	}
	return SaveImage{Data: data, MediaType: mt, Name: filepath.Base(path)}, nil
}

func fetchImageURL(url string, cfg normalizeConfig) (SaveImage, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return SaveImage{}, &NormalizeImageError{Reason: "build request: " + err.Error()}
	}
	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return SaveImage{}, &NormalizeImageError{Reason: "fetch URL: " + err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return SaveImage{}, &NormalizeImageError{Reason: fmt.Sprintf("fetch URL: status %d", resp.StatusCode)}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return SaveImage{}, &NormalizeImageError{Reason: "read body: " + err.Error()}
	}
	mt := MediaTypeFromMIME(resp.Header.Get("Content-Type"))
	if mt == "" {
		mt = MediaTypeFromMIME(detectMIME(data))
	}
	return SaveImage{Data: data, MediaType: mt, Name: filepath.Base(url)}, nil
}

func decodeDataURI(uri string) (SaveImage, error) {
	rest := strings.TrimPrefix(uri, "data:")
	comma := strings.Index(rest, ",")
	if comma < 0 {
		return SaveImage{}, &NormalizeImageError{Reason: "malformed data URI (missing comma)"}
	}
	meta := rest[:comma]
	b64 := rest[comma+1:]
	if !strings.Contains(meta, "base64") {
		return SaveImage{}, &NormalizeImageError{Reason: "data URI must be base64-encoded"}
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return SaveImage{}, &NormalizeImageError{Reason: "decode data URI base64: " + err.Error()}
	}
	mt := MediaTypeFromMIME(meta)
	if mt == "" {
		mt = MediaTypeFromMIME(detectMIME(data))
	}
	return SaveImage{Data: data, MediaType: mt}, nil
}

// looksLikeBase64 reports whether a source string is plausibly raw base64
// (only base64 alphabet + optional padding, and a length that decodes cleanly).
func looksLikeBase64(s string) bool {
	clean := strings.TrimRight(s, "=")
	if len(clean) == 0 || len(s)%4 != 0 {
		return false
	}
	for _, r := range clean {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '+', r == '/':
		default:
			return false
		}
	}
	return true
}

// detectMIME sniffs bytes for an image media type using the net/http detector.
func detectMIME(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return http.DetectContentType(data)
}
