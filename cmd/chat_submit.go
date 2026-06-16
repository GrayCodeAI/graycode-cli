package cmd

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/feature/shellmode"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// submitUserMessage handles Enter on a non-empty prompt (slash commands, shell, or agent turn).
func (m chatModel) submitUserMessage() (chatModel, tea.Cmd) {
	if m.containerEnabled && !m.containerReady {
		m.messages = append(m.messages, displayMsg{role: "system", content: "Waiting for container — agent tools are disabled until the sandbox is ready."})
		m.viewDirty = true
		m.updateViewportContent()
		return m, nil
	}
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil
	}
	if sugs := m.slashSuggestionsFor(text); len(sugs) > 0 {
		if m.slashSel < 0 || m.slashSel >= len(sugs) {
			m.slashSel = 0
		}
		m.input.SetValue(applySlashSuggestion(sugs[m.slashSel]))
		m.input.CursorEnd()
		return m, nil
	}
	m.history = append(m.history, text)
	m.historyIdx = len(m.history)
	m.historyDraft = ""
	m.input.Reset()
	if strings.HasPrefix(text, "/") {
		result, cmd := m.handleCommand(text)
		if cm, ok := result.(chatModel); ok {
			m = cm
		}
		m.viewDirty = true
		m.updateViewportContent()
		return m, cmd
	}
	if strings.HasPrefix(text, "!") {
		m.termCtx.MarkCommand(text[1:])
		result, cmd := m.handleShellEscape(text[1:])
		if cm, ok := result.(chatModel); ok {
			return cm, cmd
		}
		return m, cmd
	}
	classification := m.modeManager.ClassifyWithMode(text)
	if classification == shellmode.ClassShell && !strings.HasPrefix(text, "!") {
		m.termCtx.MarkCommand(text)
		result, cmd := m.handleShellEscape(text)
		if cm, ok := result.(chatModel); ok {
			return cm, cmd
		}
		return m, cmd
	}
	if setup := hawkconfig.EvaluateSetupCached(context.Background()); setup.NeedsSetup {
		hint := setup.Hint
		if hint == "" {
			hint = "Complete setup in /config (keychain + model)."
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: hint})
		m.viewDirty = true
		m.updateViewportContent()
		return m, nil
	}
	if err := m.ensureSessionReadyForChat(); err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		m.viewDirty = true
		m.updateViewportContent()
		return m, nil
	}
	text = m.handleMentions(text)
	userDisplay := text
	text = m.termCtx.BuildContext(text)
	scale := engine.ClassifyScale(text)
	behavior := engine.GetBehavior(scale)
	_ = behavior
	if lessons := m.selfImprover.ForPrompt(5); lessons != "" {
		m.session.AppendSystemContext(lessons)
	}
	if soul := m.codingSoul.ForPrompt(); soul != "" {
		m.session.AppendSystemContext(soul)
	}
	cwd, _ := os.Getwd()
	if hints := m.hintsLoader.LoadHints(cwd); hints != "" {
		m.session.AppendSystemContext(hints)
	}
	m.messages = append(m.messages, displayMsg{role: "user", content: userDisplay})

	if imgPath := extractImagePath(text); imgPath != "" {
		if att, err := ReadImageFile(imgPath); err == nil {
			if m.session.AddUserWithAttachment(text, att.Base64, att.MIMEType) {
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Attached image: %s", icons.Image(), filepath.Base(imgPath))})
			} else {
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Model %q has no vision support — image %s sent as text-only note.", icons.Alert(), m.session.Model(), filepath.Base(imgPath))})
			}
		} else {
			m.session.AddUser(text)
		}
	} else if pdfPath := extractPDFPath(text); pdfPath != "" {
		if extracted, err := ReadPDFText(pdfPath); err == nil && strings.TrimSpace(extracted) != "" {
			m.session.AddUserWithDocumentText(text, filepath.Base(pdfPath), extracted)
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Extracted text from PDF: %s", icons.FileDocument(), filepath.Base(pdfPath))})
		} else {
			m.session.AddUser(text)
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Could not extract text from PDF: %s", icons.Alert(), filepath.Base(pdfPath))})
		}
	} else {
		m.session.AddUser(text)
	}
	if m.wal != nil {
		_ = m.wal.Append(session.Message{Role: "user", Content: text})
	}
	m.turnSawThinking = false
	m.turnHadAssistantOutput = false
	m.turnHadToolActivity = false
	m.waiting = true
	m.autoScroll = true
	m.viewDirty = true
	m.spinnerVerb = spinnerVerbs[rand.Intn(len(spinnerVerbs))]
	m.brailleSpinner.SetLabel(m.spinnerVerb)
	m.turnInputTokens = 0
	m.turnOutputTokens = 0
	m.startedAt = time.Time{}
	m.partial.Reset()
	m.startStream()
	return m, nil
}
