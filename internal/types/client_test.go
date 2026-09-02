package types

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GrayCodeAI/eagle/llm"
)

func TestContentPartJSONContract(t *testing.T) {
	in := EyrieMessage{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "text", Text: "hello"},
			{Type: "image_url", ImageURL: &ImageURLPart{URL: "data:image/png;base64,abc", Detail: "high"}},
			{Type: "input_audio", InputAudio: &InputAudioPart{Data: "audio", Format: "wav"}},
		},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got EyrieMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got.ContentParts) != 3 || got.ContentParts[0].Text != "hello" {
		t.Fatalf("ContentParts = %#v", got.ContentParts)
	}
	if got.ContentParts[1].ImageURL == nil || got.ContentParts[1].ImageURL.Detail != "high" {
		t.Fatalf("image part = %#v", got.ContentParts[1])
	}
	if got.ContentParts[2].InputAudio == nil || got.ContentParts[2].InputAudio.Format != "wav" {
		t.Fatalf("audio part = %#v", got.ContentParts[2])
	}
}

func TestDefaultContinuationConfig(t *testing.T) {
	got := DefaultContinuationConfig()
	if got.MaxContinuations != 3 || got.MaxTotalTokens != 32000 {
		t.Fatalf("DefaultContinuationConfig() = %#v", got)
	}
}

func TestStreamResultCloseIsOptionalAndIdempotentCompatible(t *testing.T) {
	closed := 0
	result := llm.NewStreamResult(nil, "request-1", func() { closed++ })
	if result.RequestID != "request-1" {
		t.Fatalf("RequestID = %q", result.RequestID)
	}
	result.Close()
	if closed != 1 {
		t.Fatalf("close calls = %d, want 1", closed)
	}
	(*StreamResult)(nil).Close()
	llm.NewStreamResult(nil, "", context.CancelFunc(nil)).Close()
}
