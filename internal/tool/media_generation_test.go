package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockMediaEngine struct{}

func (mockMediaEngine) Name() string { return "mock" }

func (mockMediaEngine) GenerateImage(ctx context.Context, prompt, source string, opts MediaOptions) ([]MediaResult, error) {
	results := make([]MediaResult, opts.Count)
	for i := range results {
		results[i] = MediaResult{Kind: "image", MIME: "image/png", Data: []byte("fakepng")}
	}
	return results, nil
}

func (mockMediaEngine) GenerateVideo(ctx context.Context, prompt, source string, opts MediaOptions) ([]MediaResult, error) {
	return []MediaResult{{Kind: "video", MIME: "video/mp4", Data: []byte("fakemp4")}}, nil
}

func TestGenerateMediaImageSavesLocally(t *testing.T) {
	SetMediaEngine(mockMediaEngine{})
	t.Cleanup(func() { SetMediaEngine(nil) })

	outDir := t.TempDir()
	res, err := (GenerateMediaTool{}).Execute(context.Background(), json.RawMessage(
		`{"kind":"image","prompt":"a cat","count":2,"output_path":"`+outDir+`"}`,
	))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var resp struct {
		Engine string `json:"engine"`
		Assets []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"assets"`
	}
	if err := json.Unmarshal([]byte(res), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Engine != "mock" || len(resp.Assets) != 2 {
		t.Fatalf("resp = %+v", resp)
	}
	for _, a := range resp.Assets {
		if a.Kind != "image" || !strings.HasSuffix(a.Path, ".png") {
			t.Fatalf("asset = %+v", a)
		}
		if _, err := os.Stat(a.Path); err != nil {
			t.Fatalf("asset not persisted: %v", err)
		}
	}
}

func TestGenerateMediaVideo(t *testing.T) {
	SetMediaEngine(mockMediaEngine{})
	t.Cleanup(func() { SetMediaEngine(nil) })

	outDir := t.TempDir()
	res, err := (GenerateMediaTool{}).Execute(context.Background(), json.RawMessage(
		`{"kind":"video","prompt":"sunset timelapse","output_path":"`+outDir+`"}`,
	))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var resp struct {
		Assets []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"assets"`
	}
	if err := json.Unmarshal([]byte(res), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Assets) != 1 || resp.Assets[0].Kind != "video" || !strings.HasSuffix(resp.Assets[0].Path, ".mp4") {
		t.Fatalf("assets = %+v", resp.Assets)
	}
}

func TestGenerateMediaNoEngine(t *testing.T) {
	SetMediaEngine(nil)
	if _, err := (GenerateMediaTool{}).Execute(context.Background(),
		json.RawMessage(`{"kind":"image","prompt":"x"}`)); err == nil {
		t.Fatal("expected error when no engine installed")
	}
}

func TestGenerateMediaRequiresPrompt(t *testing.T) {
	SetMediaEngine(mockMediaEngine{})
	t.Cleanup(func() { SetMediaEngine(nil) })
	if _, err := (GenerateMediaTool{}).Execute(context.Background(),
		json.RawMessage(`{"kind":"image"}`)); err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

func TestExtensionForMIME(t *testing.T) {
	cases := []struct{ mime, kind, want string }{
		{"image/png", "image", ".png"},
		{"image/jpeg", "image", ".jpg"},
		{"video/mp4", "video", ".mp4"},
		{"video/webm", "video", ".webm"},
		{"video/quicktime", "video", ".mov"},
		{"", "video", ".mp4"},
		{"", "image", ".png"},
	}
	for _, c := range cases {
		if got := extensionForMIME(c.mime, c.kind); got != c.want {
			t.Fatalf("extensionForMIME(%q,%q) = %q, want %q", c.mime, c.kind, got, c.want)
		}
	}
}

func TestDefaultMediaDir(t *testing.T) {
	d := DefaultMediaDir()
	if d == "" || filepath.Base(d) != "generated-media" {
		t.Fatalf("DefaultMediaDir = %q", d)
	}
}
