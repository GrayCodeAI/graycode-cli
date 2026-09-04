// Package stt provides pluggable speech-to-text transcription for messaging
// gateways (e.g. Telegram voice notes), adopted from grok-cli's audio-input
// flow. A Transcriber backs the actual cloud STT call; the package handles the
// safe plumbing around it: downloading an attachment to a temp file with a
// path-traversal guard and mapping media types to extensions, so the same
// transcription pipeline works regardless of which STT backend is installed.
package stt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Transcriber turns audio bytes into text. graycode does not bundle an STT
// provider; a host wires one in via SetTranscriber. A nil transcriber makes
// Transcription disabled and returns a clear error.
type Transcriber interface {
	// Name identifies the STT backend for provenance/logging.
	Name() string
	// Transcribe converts audio data (already on disk at localPath) to text.
	// language is optional ("", "en", etc.).
	Transcribe(ctx context.Context, localPath, language string) (string, error)
}

var transcriber Transcriber

// SetTranscriber installs the speech-to-text backend.
func SetTranscriber(t Transcriber) { transcriber = t }

// Enabled reports whether an STT backend is installed.
func Enabled() bool { return transcriber != nil }

// TranscribeResult is the outcome of transcribing a downloaded attachment.
type TranscribeResult struct {
	Text       string
	Language   string
	DurationMS int64
}

// DownloadAttachment downloads a Telegram (or generic) hosted file to a
// per-request temp directory and returns the local path. The token is used for
// Telegram's file endpoint; for other hosts set downloadToken to "".
//
// The returned path always lives inside the created temp dir, which defeats
// path-traversal attempts in the file path or name.
func DownloadAttachment(ctx context.Context, client *http.Client, downloadURL, downloadToken, suggestedName string) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	u, err := url.Parse(downloadURL)
	if err != nil {
		return "", fmt.Errorf("stt: parse download url: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("stt: unsupported download scheme %q", u.Scheme)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("stt: build download request: %w", err)
	}
	if downloadToken != "" {
		req.Header.Set("Authorization", "Bearer "+downloadToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("stt: download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("stt: download status %d", resp.StatusCode)
	}

	// Create a dedicated temp dir and confine the file to it.
	dir, err := os.MkdirTemp("", "graycode-stt-*")
	if err != nil {
		return "", fmt.Errorf("stt: create temp dir: %w", err)
	}
	name := safeFileName(suggestedName)
	path := filepath.Join(dir, name)
	if !isPathInside(dir, path) {
		// Defensive: MkdirTemp guarantees this, but never trust a caller's name.
		return "", fmt.Errorf("stt: suggested name escapes temp dir")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("stt: create temp file: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 50<<20)); err != nil { // 50 MB cap
		return "", fmt.Errorf("stt: write attachment: %w", err)
	}
	return path, nil
}

// Transcribe runs the installed transcriber over the audio file at localPath.
// It returns the transcript, or a clear error when no STT backend is installed.
func Transcribe(ctx context.Context, localPath, language string) (string, error) {
	if transcriber == nil {
		return "", fmt.Errorf("stt: no transcriber installed — configure a speech-to-text backend first")
	}
	return transcriber.Transcribe(ctx, localPath, language)
}

// isPathInside reports whether child is lexically inside parent after cleaning.
func isPathInside(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// safeFileName strips path separators and control characters from a suggested
// attachment name so it can never traverse out of the temp dir.
func safeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." || name == string(filepath.Separator) {
		return "attachment.ogg"
	}
	var b strings.Builder
	for _, r := range name {
		if r == '/' || r == '\\' || r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	cleaned := b.String()
	if cleaned == "" {
		return "attachment.ogg"
	}
	return cleaned
}

// ExtensionForMedia infers a file extension from a Telegram media path/name or
// MIME type. Telegram voice notes are .ogg (Opus); audio attachments vary.
func ExtensionForMedia(nameOrPath, mime string) string {
	base := strings.ToLower(filepath.Base(nameOrPath))
	switch {
	case strings.HasSuffix(base, ".oga"), strings.HasSuffix(base, ".ogg"):
		return ".ogg"
	case strings.HasSuffix(base, ".mp3"):
		return ".mp3"
	case strings.HasSuffix(base, ".m4a"):
		return ".m4a"
	case strings.HasSuffix(base, ".wav"):
		return ".wav"
	case strings.HasSuffix(base, ".flac"):
		return ".flac"
	case strings.HasSuffix(base, ".aac"):
		return ".aac"
	}
	switch strings.ToLower(mime) {
	case "audio/ogg", "application/ogg", "audio/opus":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4", "audio/x-m4a":
		return ".m4a"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/flac":
		return ".flac"
	case "audio/aac":
		return ".aac"
	}
	return ".audio"
}
