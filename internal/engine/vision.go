package engine

import (
	"strings"

	"github.com/GrayCodeAI/hawk/internal/types"
)

// ModelSupportsVision reports whether the named model can accept image content
// blocks. eyrie carries image data on EyrieMessage.Images but does not expose a
// per-model capability flag, so hawk gates locally on the model identifier.
//
// The check is heuristic and intentionally conservative: an unknown model is
// treated as non-vision so that we never send image blocks a provider will
// reject. Known vision-capable families are matched by substring on the
// lower-cased model id (which is how eyrie/providers name them).
func ModelSupportsVision(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}

	// Explicitly text-only variants that would otherwise match a vision family.
	for _, no := range []string{
		"claude-3-haiku-20240307", // original 3.0 Haiku is text-only
	} {
		if strings.Contains(m, no) {
			return false
		}
	}

	// Vision-capable model families. Substring match keeps this resilient to
	// date suffixes and provider prefixes (e.g. "us.anthropic.claude-...").
	visionMarkers := []string{
		// Anthropic: Claude 3 / 3.5 / 3.7 / 4 family (Opus, Sonnet, Haiku 3.5+).
		"claude-3-5", "claude-3.5",
		"claude-3-7", "claude-3.7",
		"claude-3-opus", "claude-3-sonnet",
		"claude-opus", "claude-sonnet", "claude-haiku",
		"claude-4", "claude-4-5", "opus-4", "sonnet-4", "haiku-4",
		// OpenAI multimodal.
		"gpt-4o", "gpt-4.1", "gpt-4-turbo", "gpt-4-vision", "gpt-5", "o1", "o3", "o4",
		// Google Gemini (all current Gemini models are multimodal).
		"gemini",
		// Meta Llama vision variants.
		"llama-3.2", "llama-3-2", "llama-4", "vision",
		// Mistral multimodal.
		"pixtral",
		// Qwen vision.
		"qwen-vl", "qwen2-vl", "qwen2.5-vl",
	}
	for _, marker := range visionMarkers {
		if strings.Contains(m, marker) {
			return true
		}
	}
	return false
}

// SupportsVision reports whether the session's active model can accept images.
func (s *Session) SupportsVision() bool {
	return ModelSupportsVision(s.Model())
}

// AddUserWithAttachment adds a user message with an attached image, gated on the
// active model's vision capability.
//
// imageBase64 must be the raw standard-base64 encoding of the image bytes (no
// data: prefix); mediaType is the MIME type (e.g. "image/png"). If mediaType is
// empty it defaults to image/png.
//
// When the active model supports vision the image is attached as a multimodal
// block (via EyrieMessage.Images, which eyrie expands into an image content
// block). When it does not, the message degrades gracefully: only the text is
// sent, annotated with a note so the model is aware an image was dropped, and
// attached reports false. Callers can surface a warning to the user on false.
func (s *Session) AddUserWithAttachment(content, imageBase64, mediaType string) (attached bool) {
	if mediaType == "" {
		mediaType = "image/png"
	}

	if !s.SupportsVision() {
		note := content
		if note != "" {
			note += "\n\n"
		}
		note += "[image attachment omitted: the active model (" + s.Model() + ") does not support image input]"
		s.AddUser(note)
		return false
	}

	s.mu.Lock()
	s.Persistence().SetRawMessages(append(s.Persistence().RawMessages(), types.EyrieMessage{
		Role:    "user",
		Content: content,
		Images:  []string{"data:" + mediaType + ";base64," + imageBase64},
	}))
	s.mu.Unlock()

	if s.Persistence().Graph() != nil {
		parentID := ""
		if head, err := s.Persistence().Graph().Head(); err == nil && head != nil {
			parentID = head.ID
		}
		_, _ = s.Persistence().Graph().Append(parentID, "user", content+" [image attached]")
	}
	return true
}

// AddUserWithDocumentText adds a user message that incorporates extracted
// document text (e.g. from a PDF) inline as text. This is the graceful path for
// formats eyrie cannot carry as native image/document blocks: the extracted
// text is fenced and labeled so the model can reason over it.
//
// label is a short human identifier for the source (e.g. the file name); it may
// be empty. The combined message is added as a normal text user message, so it
// works regardless of model vision capability.
func (s *Session) AddUserWithDocumentText(content, label, extracted string) {
	var b strings.Builder
	if content != "" {
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	if label != "" {
		b.WriteString("[Document: ")
		b.WriteString(label)
		b.WriteString("]\n")
	} else {
		b.WriteString("[Document]\n")
	}
	b.WriteString("```\n")
	b.WriteString(extracted)
	if !strings.HasSuffix(extracted, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```")
	s.AddUser(b.String())
}
