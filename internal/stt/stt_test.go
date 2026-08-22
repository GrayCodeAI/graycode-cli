package stt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockTranscriber struct{}

func (mockTranscriber) Name() string { return "mock" }

func (mockTranscriber) Transcribe(ctx context.Context, localPath, language string) (string, error) {
	data, _ := os.ReadFile(localPath)
	return "transcribed:" + string(data), nil
}

func TestDownloadAttachmentConfinesToTempDir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake-audio-bytes"))
	}))
	defer srv.Close()

	path, err := DownloadAttachment(context.Background(), nil, srv.URL+"/file", "", "voice.ogg")
	if err != nil {
		t.Fatalf("DownloadAttachment: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(path)) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != "fake-audio-bytes" {
		t.Fatalf("content = %q", data)
	}
	if filepath.Base(path) != "voice.ogg" {
		t.Fatalf("name = %q", filepath.Base(path))
	}
	if !isPathInside(filepath.Dir(path), path) {
		t.Fatalf("path not confined: %q", path)
	}
}

func TestDownloadAttachmentRejectsNonHTTP(t *testing.T) {
	if _, err := DownloadAttachment(context.Background(), nil, "file:///etc/passwd", "", "x"); err == nil {
		t.Fatal("expected error for non-http scheme")
	}
}

func TestDownloadAttachmentRejectsTraversalName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	path, err := DownloadAttachment(context.Background(), nil, srv.URL+"/f", "", "../../evil.ogg")
	if err != nil {
		t.Fatalf("DownloadAttachment: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(path)) })

	// The cleaned name must not traverse; filepath.Base already strips the
	// leading separators, and safeFileName removes the rest.
	if strings.Contains(path, "..") {
		t.Fatalf("path leaked traversal: %q", path)
	}
	if filepath.Base(path) != "evil.ogg" {
		t.Fatalf("name = %q", filepath.Base(path))
	}
}

func TestTranscribeRequiresEngine(t *testing.T) {
	SetTranscriber(nil)
	if _, err := Transcribe(context.Background(), "/tmp/x.ogg", ""); err == nil {
		t.Fatal("expected error with no transcriber")
	}
}

func TestTranscribeWithEngine(t *testing.T) {
	SetTranscriber(mockTranscriber{})
	t.Cleanup(func() { SetTranscriber(nil) })

	dir := t.TempDir()
	path := filepath.Join(dir, "a.ogg")
	if err := os.WriteFile(path, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Transcribe(context.Background(), path, "en")
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if got != "transcribed:audio" {
		t.Fatalf("got %q", got)
	}
}

func TestExtensionForMedia(t *testing.T) {
	cases := []struct{ name, mime, want string }{
		{"voice.oga", "", ".ogg"},
		{"voice.ogg", "", ".ogg"},
		{"audio.mp3", "", ".mp3"},
		{"audio.m4a", "", ".m4a"},
		{"", "audio/ogg", ".ogg"},
		{"", "audio/mpeg", ".mp3"},
		{"", "audio/mp4", ".m4a"},
		{"", "", ".audio"},
	}
	for _, c := range cases {
		if got := ExtensionForMedia(c.name, c.mime); got != c.want {
			t.Fatalf("ExtensionForMedia(%q,%q) = %q, want %q", c.name, c.mime, got, c.want)
		}
	}
}

func TestSafeFileName(t *testing.T) {
	for input, want := range map[string]string{
		"voice.ogg":       "voice.ogg",
		"../../evil.ogg":  "evil.ogg",
		"a/b/evil.ogg":    "evil.ogg",
		"":                "attachment.ogg",
		"..":              "attachment.ogg",
		"voice\n\x00.ogg": "voice.ogg",
	} {
		if got := safeFileName(input); got != want {
			t.Fatalf("safeFileName(%q) = %q, want %q", input, got, want)
		}
	}
}
