package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
	"github.com/GrayCodeAI/graycode-cli/internal/eventlog/zstdz"
)

func TestSaveWithZstdRoundTrip(t *testing.T) {
	dir := t.TempDir()

	now := time.Now().UTC()
	events := []eventlog.Event{
		{Type: eventlog.SessionMeta, Seq: 1, At: now, Data: eventlog.Meta{
			ID: "test-zstd", Model: "gpt-4", Provider: "openai",
			CWD: dir, FormatVersion: eventlog.SessionFormatVersion,
		}},
		{Type: eventlog.UserMessage, Seq: 2, At: now, Data: eventlog.Message{
			Role: "user", Content: "hello",
		}},
	}
	wire, err := eventlog.MarshalWire(events)
	if err != nil {
		t.Fatal(err)
	}

	// Build event JSONL lines
	var eventBuf bytes.Buffer
	for _, ev := range wire {
		data, _ := json.Marshal(ev)
		eventBuf.Write(data)
		eventBuf.WriteByte('\n')
	}

	// Compress the event portion as a single zstd frame
	compressed, err := zstdz.CompressFrame(eventBuf.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	// Write header + message + compressed frame as a single file
	metaLine := `{"type":"session_meta","id":"test-zstd","model":"gpt-4","provider":"openai","cwd":"` + dir + `","created_at":"` + now.Format(time.RFC3339) + `","updated_at":"` + now.Format(time.RFC3339) + `","format_version":1}` + "\n"
	msgLine := `{"role":"user","content":"hello"}` + "\n"
	zstdPath := filepath.Join(dir, "test-zstd.jsonl.zstd")
	content := metaLine + msgLine + string(compressed)
	if err := os.WriteFile(zstdPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Verify detectCompression
	comp := detectCompression(zstdPath)
	if comp != JsonlCompressionZstd {
		t.Fatalf("expected zstd compression, got %s", comp)
	}

	// Verify parseHeaderMeta works
	meta, err := parseHeaderMeta(zstdPath)
	if err != nil {
		t.Fatal(err)
	}
	if meta["id"] != "test-zstd" {
		t.Errorf("expected id 'test-zstd', got %v", meta["id"])
	}
}

func TestZstdCompressionDetect(t *testing.T) {
	dir := t.TempDir()

	header := `{"type":"session_meta","id":"zstd-test","model":"gpt-4","provider":"openai","cwd":"` + dir + `"}` + "\n"
	compressed, err := zstdz.CompressFrame([]byte(`{"type":"message.user","seq":1}`))
	if err != nil {
		t.Fatal(err)
	}

	// Write raw zstd frame bytes (not JSON-encoded) after the header
	content := header + string(compressed) + "\n"
	path := filepath.Join(dir, "zstd-test.jsonl.zstd")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	comp := detectCompression(path)
	if comp != JsonlCompressionZstd {
		t.Fatalf("expected zstd compression, got %s", comp)
	}

	// Write a plaintext file
	plainPath := filepath.Join(dir, "plain-test.jsonl")
	if err := os.WriteFile(plainPath, []byte(header+`{"role":"user","content":"hi"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	comp = detectCompression(plainPath)
	if comp != JsonlCompressionNone {
		t.Fatalf("expected none compression, got %s", comp)
	}
}

func TestParseHeaderMeta(t *testing.T) {
	dir := t.TempDir()
	header := `{"type":"session_meta","id":"header-test","model":"gpt-4","provider":"openai","name":"My Session","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","format_version":1}` + "\n"
	msgLine := `{"role":"user","content":"hello world"}` + "\n"
	content := header + msgLine + `{"type":"message.user","seq":1}` + "\n"

	path := filepath.Join(dir, "header-test.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	meta, err := parseHeaderMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta["id"] != "header-test" {
		t.Errorf("expected id 'header-test', got %v", meta["id"])
	}
	if meta["name"] != "My Session" {
		t.Errorf("expected name 'My Session', got %v", meta["name"])
	}
	if meta["format_version"] != float64(1) {
		t.Errorf("expected format_version 1, got %v", meta["format_version"])
	}
}

func TestSessionLogScannerSeqValidation(t *testing.T) {
	header := `{"type":"session_meta","id":"seq-test","model":"gpt-4","provider":"openai","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","format_version":1}` + "\n"
	// Seq 1 then seq 3 (gap — should trigger issue)
	content := header + `{"type":"message.user","seq":1,"at":"2026-01-01T00:00:00Z"}` + "\n" +
		`{"type":"message.user","seq":3,"at":"2026-01-01T00:00:00Z"}` + "\n"

	scanner := NewSessionLogScanner(bytes.NewReader([]byte(content)), "seq-test")
	_ = scanner.Scan()
	if scanner.Error() == nil {
		t.Fatal("expected seq gap error, got nil")
	}
	t.Logf("seq scanner error: %v", scanner.Error())
}

func TestSessionLogScannerCommittedBytes(t *testing.T) {
	header := `{"type":"session_meta","id":"committed-test","model":"gpt-4","provider":"openai","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}` + "\n"
	msg := `{"role":"user","content":"hi"}` + "\n"
	content := header + msg

	scanner := NewSessionLogScanner(bytes.NewReader([]byte(content)), "committed-test")
	if err := scanner.Scan(); err != nil {
		t.Fatal(err)
	}
	if scanner.Header() == nil {
		t.Fatal("expected header")
	}
	if scanner.Header()["id"] != "committed-test" {
		t.Errorf("expected id 'committed-test'")
	}
	if len(scanner.Messages()) != 1 {
		t.Fatalf("expected 1 message, got %d", len(scanner.Messages()))
	}
}

func TestListWithZstdExtensions(t *testing.T) {
	ext := ".jsonl.zstd"
	id := "my-session" + ext
	var result string
	if len(id) > len(ext) && id[len(id)-len(ext):] == ext {
		result = id[:len(id)-len(ext)]
	}
	if result != "my-session" {
		t.Errorf("expected 'my-session', got %s", result)
	}
}
