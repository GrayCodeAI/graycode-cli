package engine

import (
	"strings"
	"testing"
)

func TestModelSupportsVision(t *testing.T) {
	t.Parallel()
	tests := []struct {
		model string
		want  bool
	}{
		// Anthropic vision-capable.
		{"claude-3-5-sonnet-20241022", true},
		{"claude-3.5-sonnet", true},
		{"claude-3-7-sonnet", true},
		{"claude-opus-4-8", true},
		{"us.anthropic.claude-sonnet-4-20250514", true},
		{"claude-3-opus-20240229", true},
		// Anthropic text-only original Haiku.
		{"claude-3-haiku-20240307", false},
		// OpenAI multimodal.
		{"gpt-4o", true},
		{"gpt-4o-mini", true},
		{"gpt-4.1", true},
		{"o3", true},
		// OpenAI text-only legacy.
		{"gpt-3.5-turbo", false},
		// Google.
		{"gemini-1.5-pro", true},
		{"gemini-2.0-flash", true},
		// Meta.
		{"llama-3.2-90b-vision", true},
		{"llama-3.1-8b", false},
		// Mistral / Qwen.
		{"pixtral-12b", true},
		{"qwen2.5-vl-7b", true},
		{"mistral-large", false},
		// Edge cases.
		{"", false},
		{"   ", false},
		{"some-unknown-model", false},
	}
	for _, tc := range tests {
		if got := ModelSupportsVision(tc.model); got != tc.want {
			t.Errorf("ModelSupportsVision(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

func TestAddUserWithAttachment_VisionModel(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := NewSession("anthropic", "claude-3-5-sonnet-20241022", "sys", nil)
	s.SetTestClient(mc)

	attached := s.AddUserWithAttachment("describe this", "QUJD", "image/png")
	if !attached {
		t.Fatal("expected attached=true for vision-capable model")
	}

	msgs := s.Persistence().RawMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if len(msgs[0].Images) != 1 {
		t.Fatalf("expected 1 image block, got %d", len(msgs[0].Images))
	}
	wantImg := "data:image/png;base64,QUJD"
	if msgs[0].Images[0] != wantImg {
		t.Errorf("image block = %q, want %q", msgs[0].Images[0], wantImg)
	}
	if msgs[0].Content != "describe this" {
		t.Errorf("content = %q, want 'describe this'", msgs[0].Content)
	}
}

func TestAddUserWithAttachment_DefaultMediaType(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := NewSession("anthropic", "claude-opus-4-8", "sys", nil)
	s.SetTestClient(mc)

	if !s.AddUserWithAttachment("hi", "ZZZ", "") {
		t.Fatal("expected attached=true")
	}
	img := s.Persistence().RawMessages()[0].Images[0]
	if !strings.HasPrefix(img, "data:image/png;base64,") {
		t.Errorf("expected default image/png media type, got %q", img)
	}
}

func TestAddUserWithAttachment_NonVisionModelDegrades(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := NewSession("openai", "gpt-3.5-turbo", "sys", nil)
	s.SetTestClient(mc)

	attached := s.AddUserWithAttachment("look at this", "QUJD", "image/png")
	if attached {
		t.Fatal("expected attached=false for non-vision model")
	}

	msgs := s.Persistence().RawMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if len(msgs[0].Images) != 0 {
		t.Errorf("expected no image blocks on degraded message, got %d", len(msgs[0].Images))
	}
	if !strings.Contains(msgs[0].Content, "look at this") {
		t.Errorf("degraded content should retain original text, got %q", msgs[0].Content)
	}
	if !strings.Contains(strings.ToLower(msgs[0].Content), "does not support image") {
		t.Errorf("degraded content should note missing vision support, got %q", msgs[0].Content)
	}
}

func TestSupportsVision(t *testing.T) {
	t.Parallel()
	vis := NewSession("anthropic", "claude-3-5-sonnet", "", nil)
	if !vis.SupportsVision() {
		t.Error("expected SupportsVision()=true for claude-3-5-sonnet")
	}
	non := NewSession("openai", "gpt-3.5-turbo", "", nil)
	if non.SupportsVision() {
		t.Error("expected SupportsVision()=false for gpt-3.5-turbo")
	}
}

func TestAddUserWithDocumentText(t *testing.T) {
	t.Parallel()
	mc := newMockClient()
	s := newMockSession(mc)

	s.AddUserWithDocumentText("summarize", "report.pdf", "line one\nline two")

	msgs := s.Persistence().RawMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	c := msgs[0].Content
	for _, want := range []string{"summarize", "report.pdf", "line one", "line two", "```"} {
		if !strings.Contains(c, want) {
			t.Errorf("document message missing %q; got:\n%s", want, c)
		}
	}
}
