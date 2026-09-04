package cmd

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

// handleImageCommand implements the /image slash command:
//
//	/image <path> [prompt...]
//
// It reads an image (or PDF) file, builds a multimodal user message gated on the
// active model's vision capability, and starts a turn. For PDFs the text is
// extracted and injected inline (graycode-router has no native document block). Without a
// vision-capable model the image degrades to a text-only note.
func (m *chatModel) handleImageCommand(parts []string, text string) (tea.Model, tea.Cmd) {
	if len(parts) < 2 {
		m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /image <path> [prompt]\nAttach an image (png/jpg/gif/webp/bmp) or PDF to your next message."})
		return m, nil
	}

	// First field after /image is the path; the rest (if any) is the prompt.
	rest := strings.TrimSpace(strings.TrimPrefix(text, parts[0]))
	path := parts[1]
	prompt := strings.TrimSpace(strings.TrimPrefix(rest, parts[1]))
	// Allow @-mention and quoting of the path argument.
	path = strings.Trim(strings.TrimPrefix(path, "@"), "\"'")

	if err := m.ensureSessionReadyForChat(); err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		m.viewDirty = true
		m.updateViewportContent()
		return m, nil
	}

	switch {
	case IsImageFile(path):
		att, err := ReadImageFile(path)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		display := prompt
		if display == "" {
			display = FormatImageMessage("", path)
		} else {
			display = FormatImageMessage(prompt, path)
		}
		m.messages = append(m.messages, displayMsg{role: "user", content: display})
		if m.session.AddUserWithAttachment(prompt, att.Base64, att.MIMEType) {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Attached image: %s", icons.Image(), filepath.Base(path))})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Model %q has no vision support — image %s sent as text-only note.", icons.Alert(), m.session.Model(), filepath.Base(path))})
		}
	case IsPDFFile(path):
		extracted, err := ReadPDFText(path)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		if strings.TrimSpace(extracted) == "" {
			m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("No extractable text found in PDF: %s", filepath.Base(path))})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "user", content: fmt.Sprintf("%s\n%s [PDF: %s]", prompt, icons.FileDocument(), filepath.Base(path))})
		m.session.AddUserWithDocumentText(prompt, filepath.Base(path), extracted)
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Extracted text from PDF: %s", icons.FileDocument(), filepath.Base(path))})
	default:
		m.messages = append(m.messages, displayMsg{role: "error", content: fmt.Sprintf("Unsupported attachment: %s (supported: png, jpg, jpeg, gif, webp, bmp, pdf)", filepath.Base(path))})
		return m, nil
	}

	m.waiting = true
	m.autoScroll = true
	m.viewDirty = true
	m.spinnerVerb = spinnerVerbs[rand.Intn(len(spinnerVerbs))] // #nosec G404 -- non-cryptographic use (random spinner verb selection)
	m.brailleSpinner.SetLabel(m.spinnerVerb)
	m.turnInputTokens = 0
	m.turnOutputTokens = 0
	m.turnEstimatedOutputRunes = 0
	m.partial.Reset()
	m.startStream()
	return m, nil
}
