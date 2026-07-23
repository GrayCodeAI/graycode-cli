package cmd

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/storage"
)

// saveSession persists the current session to disk.
func (m *chatModel) saveSession() {
	raw := m.session.RawMessages()
	if len(raw) == 0 {
		return
	}
	err := session.Save(&session.Session{
		ID: m.sessionID, Model: m.session.Model(), Provider: m.session.Provider(),
		Messages: session.FromRuntimeMessages(raw), CreatedAt: time.Now(),
	})
	// On successful save, WAL is no longer needed (session file has everything)
	if err == nil && m.wal != nil {
		_ = m.wal.Remove()
		m.wal = nil
	}
}

func formatQuitResumeMessage(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return "Thank you for using Hawk!\n"
	}
	return fmt.Sprintf("Thank you for using Hawk!\n\nTo resume this session, run: hawk --resume %s\n", sessionID)
}

// handleSessionCommand dispatches session-management slash commands.
func (m *chatModel) handleSessionCommand(cmd string, parts []string, text string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "/quit", "/exit":
		m.saveSession()
		// Cancel any running /loop goroutine.
		if m.loopCancel != nil {
			m.loopCancel()
			m.loopCancel = nil
		}
		// Cancel any running /parallel agents.
		if m.parallelCancel != nil {
			m.parallelCancel()
			m.parallelCancel = nil
		}
		// Re-enable system sleep if it was prevented.
		if m.sleepCancel != nil {
			m.sleepCancel()
			m.sleepCancel = nil
		}
		// Stop file watcher if active.
		if m.watcherStop != nil {
			m.watcherStop()
		}
		// Cancel background goroutines.
		if m.bgCancel != nil {
			m.bgCancel()
		}
		ClearTabProgress()
		m.quitting = true
		return m, tea.Quit

	case "/clear":
		if m.manualCompacting {
			return m.cancelManualCompact("Compaction cancelled.")
		}
		// Cancel any running /loop goroutine.
		if m.loopCancel != nil {
			m.loopCancel()
			m.loopCancel = nil
		}
		// Cancel any running /parallel agents.
		if m.parallelCancel != nil {
			m.parallelCancel()
			m.parallelCancel = nil
		}
		m.messages = []displayMsg{{role: "system", content: "Conversation cleared."}}
		m.invalidateViewportCache()
		m.viewDirty = true
		m.autoScroll = false
		return m, nil

	case "/compact":
		if m.manualCompacting {
			return m.cancelManualCompact("Compaction cancelled.")
		}
		if m.waiting {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Wait for the current response to finish, then run /compact."})
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		}
		return m.startManualCompact()

	case "/diff":
		stat, statErr := gitOutput("diff", "--stat")
		diff, diffErr := gitOutput("diff")
		if strings.TrimSpace(diff) == "" {
			stat, statErr = gitOutput("diff", "--cached", "--stat")
			diff, diffErr = gitOutput("diff", "--cached")
		}
		// Report git errors instead of silently showing "No changes detected".
		if statErr != nil || diffErr != nil {
			errMsg := "git diff failed"
			if statErr != nil {
				errMsg = statErr.Error()
			} else if diffErr != nil {
				errMsg = diffErr.Error()
			}
			m.messages = append(m.messages, displayMsg{role: "error", content: errMsg})
			return m, nil
		}
		if strings.TrimSpace(diff) == "" {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No changes detected."})
			return m, nil
		}
		output := stat + "\n\n" + diff
		if len(output) > 10000 {
			output = stat + "\n\n(diff too large, showing stat only)"
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: output})
		return m, nil

	case "/history":
		entries, err := session.List()
		if err != nil || len(entries) == 0 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No saved sessions."})
			return m, nil
		}
		var b strings.Builder
		for _, e := range entries {
			b.WriteString(fmt.Sprintf("  %s  %s  %s\n", e.ID, e.UpdatedAt.Format("Jan 02 15:04"), e.Preview))
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
		return m, nil

	case "/recover":
		candidates := session.ScanForRecovery()
		if len(candidates) == 0 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No interrupted sessions found."})
			return m, nil
		}
		if len(parts) >= 2 {
			// Resume specific session
			s, note, err := session.ResumeSession(parts[1])
			if err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				return m, nil
			}
			m.sessionID = s.ID
			m.invalidateViewportCache()
			m.messages = []displayMsg{{role: "welcome", content: m.welcomeCache}}
			msgs := session.ToRuntimeMessages(s.Messages)
			for _, sm := range s.Messages {
				if sm.Role == "user" || sm.Role == "assistant" {
					m.messages = append(m.messages, displayMsg{role: sm.Role, content: sm.Content})
				}
			}
			m.session.LoadMessages(msgs)
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Recovered: %s\nSession %s ready (%d msgs)", note, s.ID, len(s.Messages))})
			m.viewDirty = true
			m.autoScroll = false
			return m, nil
		}
		// List candidates
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Found %d interrupted session(s):\n\n", len(candidates)))
		for i, c := range candidates {
			shortID := c.SessionID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			b.WriteString(fmt.Sprintf("%d. [%s] %s — %s (%d msgs, %s)\n",
				i+1, shortID, c.Interruption, c.CWD, c.MessageCount, formatDuration(c.Age)))
		}
		b.WriteString("\nResume with: /recover <session-id>")
		m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
		return m, nil

	case "/resume":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /resume <session-id>"})
			return m, nil
		}
		saved, err := session.Load(parts[1])
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.sessionID = saved.ID
		m.invalidateViewportCache()
		m.messages = []displayMsg{{role: "welcome", content: m.welcomeCache}}
		msgs := session.ToRuntimeMessages(saved.Messages)
		for _, sm := range saved.Messages {
			if sm.Role == "user" || sm.Role == "assistant" {
				m.messages = append(m.messages, displayMsg{role: sm.Role, content: sm.Content})
			}
		}
		m.session.LoadMessages(msgs)
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Resumed session %s", saved.ID)})
		m.viewDirty = true
		m.autoScroll = false
		return m, nil

	case "/fork":
		// If convodag is active, fork from the current head node
		if m.session.Persistence().Graph() != nil {
			headID := m.session.ConvoHead()
			if headID == "" {
				m.messages = append(m.messages, displayMsg{role: "error", content: "No conversation to fork from."})
				return m, nil
			}
			forkID, err := m.session.ForkConversation(headID)
			if err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				return m, nil
			}
			shortHead := headID
			if len(shortHead) > 8 {
				shortHead = shortHead[:8]
			}
			shortFork := forkID
			if len(shortFork) > 8 {
				shortFork = shortFork[:8]
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Forked at %s → new branch %s\nYou can now take a different approach. Use /branches to see all branches.", shortHead, shortFork)})
			return m, nil
		}
		// Fallback: legacy session fork
		atIndex := len(m.session.RawMessages()) - 1
		if len(parts) >= 2 {
			if idx, err := strconv.Atoi(parts[1]); err == nil {
				atIndex = idx
			}
		}
		if atIndex < 0 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "No messages to fork from."})
			return m, nil
		}
		forked, err := session.Fork(m.sessionID, atIndex)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Forked session %s from %s at index %d", forked.ID, m.sessionID, atIndex)})
		return m, nil

	case "/export":
		format := "md"
		if len(parts) >= 2 {
			switch strings.ToLower(parts[1]) {
			case "md", "markdown":
				format = "md"
			case "json":
				format = "json"
			case "txt", "text":
				format = "txt"
			default:
				m.messages = append(m.messages, displayMsg{
					role: "system",
					content: "Usage: /export [format]\n" +
						"  /export        — export as Markdown (default)\n" +
						"  /export md     — export as Markdown\n" +
						"  /export json   — export as structured JSON\n" +
						"  /export txt    — export as plain text",
				})
				return m, nil
			}
		}
		exportPath, err := exportSession(m, format)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Exported to: %s", exportPath)})
		}
		return m, nil

	case "/share":
		exportPath, err := writeRedactedChatMarkdownExport(m)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Session saved to: %s\nShare this file or paste its contents.", exportPath)})
		}
		return m, nil

	case "/rename":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /rename <new-session-name>"})
			return m, nil
		}
		// Sanitize: strip directory components to prevent path traversal.
		newName := filepath.Base(parts[1])
		if newName == "" || newName == "." || newName == "/" {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Invalid session name."})
			return m, nil
		}
		sessDir := storage.SessionsDir()
		oldPath := filepath.Join(sessDir, filepath.Base(m.sessionID)+".jsonl")
		newPath := filepath.Join(sessDir, newName+".jsonl")
		if err := os.Rename(oldPath, newPath); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.sessionID = newName
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Session renamed to: %s", newName)})
		}
		return m, nil

	case "/tag":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Usage: /tag <label>"})
			return m, nil
		}
		tagFile := filepath.Join(storage.SessionsDir(), filepath.Base(m.sessionID)+".tags")
		f, err := os.OpenFile(tagFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- tagFile built from internal sessions directory and session id
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			_, _ = f.WriteString(parts[1] + "\n")
			_ = f.Close()
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Tagged: %s", parts[1])})
		}
		return m, nil

	case "/search":
		if len(parts) < 2 {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /search <query>"})
			return m, nil
		}
		query := strings.TrimSpace(strings.TrimPrefix(text, "/search"))
		results, err := session.SearchSessions(query, 10)
		if err != nil || len(results) == 0 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No results found."})
			return m, nil
		}
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Search results for %q:\n", query))
		for _, r := range results {
			b.WriteString(fmt.Sprintf("  [%s] msg %d (%s): %s\n", r.SessionID, r.MsgIndex, r.Role, r.Preview))
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: b.String()})
		return m, nil

	case "/clean":
		days := 30
		if len(parts) >= 2 {
			if d, err := strconv.Atoi(parts[1]); err == nil && d > 0 {
				days = d
			}
		}
		removed, err := session.CleanOldSessions(time.Duration(days) * 24 * time.Hour)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Cleaned %d sessions older than %d days.", removed, days)})
		return m, nil

	case "/compress":
		days := 7
		if len(parts) >= 2 {
			if d, err := strconv.Atoi(parts[1]); err == nil && d > 0 {
				days = d
			}
		}
		count, err := session.CompressOldSessions(time.Duration(days) * 24 * time.Hour)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Compressed %d sessions older than %d days.", count, days)})
		return m, nil

	case "/rewind":
		if m.session.MessageCount() > 2 {
			m.session.RemoveLastExchange()
			if len(m.messages) >= 2 {
				m.messages = m.messages[:len(m.messages)-2]
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: "Rewound last exchange."})
		} else {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Nothing to rewind."})
		}
		return m, nil

	case "/drop":
		if m.waiting {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Wait for the current response to finish, then run /drop."})
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		}
		// Cancel any running /loop goroutine.
		if m.loopCancel != nil {
			m.loopCancel()
			m.loopCancel = nil
		}
		// Cancel any running /parallel agents.
		if m.parallelCancel != nil {
			m.parallelCancel()
			m.parallelCancel = nil
		}
		// Re-enable system sleep if it was prevented.
		if m.sleepCancel != nil {
			m.sleepCancel()
			m.sleepCancel = nil
		}
		// Drop the last N exchanges (user+assistant pairs) from context.
		n := 1
		if len(parts) >= 2 {
			parsed, err := strconv.Atoi(parts[1])
			if err != nil || parsed <= 0 {
				m.messages = append(m.messages, displayMsg{role: "error", content: "Usage: /drop [N] (N must be a positive integer)"})
				return m, nil
			}
			n = parsed
		}
		if m.session.MessageCount() <= 2 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "Nothing to drop."})
			return m, nil
		}
		dropped := 0
		for i := 0; i < n && m.session.MessageCount() > 2; i++ {
			m.session.RemoveLastExchange()
			if len(m.messages) >= 2 {
				m.messages = m.messages[:len(m.messages)-2]
			}
			dropped++
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Dropped %d exchange(s).", dropped)})
		m.invalidateViewportCache()
		m.viewDirty = true
		return m, nil

	case "/retry":
		if len(m.history) > 0 {
			last := m.history[len(m.history)-1]
			if m.session.MessageCount() > 2 {
				m.session.RemoveLastExchange()
				if len(m.messages) >= 2 {
					m.messages = m.messages[:len(m.messages)-2]
				}
			}
			m.messages = append(m.messages, displayMsg{role: "user", content: last})
			m.session.AddUser(last)
			m.waiting = true
			m.autoScroll = true
			m.spinnerVerb = spinnerVerbs[rand.Intn(len(spinnerVerbs))] // #nosec G404 -- non-cryptographic use (random spinner verb selection)
			m.brailleSpinner.SetLabel(m.spinnerVerb)
			m.startStream()
			return m, nil
		}
		m.messages = append(m.messages, displayMsg{role: "error", content: "No previous message to retry."})
		return m, nil

	case "/new":
		m.saveSession()
		// Cancel any running /loop goroutine.
		if m.loopCancel != nil {
			m.loopCancel()
			m.loopCancel = nil
		}
		// Cancel any running /parallel agents.
		if m.parallelCancel != nil {
			m.parallelCancel()
			m.parallelCancel = nil
		}
		// Re-enable system sleep if it was prevented.
		if m.sleepCancel != nil {
			m.sleepCancel()
			m.sleepCancel = nil
		}
		// Stop file watcher if active.
		if m.watcherStop != nil {
			m.watcherStop()
			m.watcherStop = nil
		}
		m.invalidateViewportCache()
		m.messages = []displayMsg{{role: "welcome", content: m.welcomeCache}}
		m.session.LoadMessages(nil)
		sid := genID()
		m.sessionID = sid
		if wal, err := session.NewWAL(sid); err == nil {
			m.wal = wal
		}
		m.termCtx.Reset()
		m.ghostText.Clear()
		m.messages = append(m.messages, displayMsg{role: "system", content: "New session started."})
		return m, nil

	case "/session":
		info := fmt.Sprintf("Session: %s\nModel: %s/%s\nSpec stage: %s\nMessages: %d\nTools: %d\n%s",
			m.sessionID, m.session.Provider(), m.session.Model(),
			specStageLabel(m.session), m.session.MessageCount(), len(m.registry.EyrieTools()), m.session.CostValue().Summary())
		m.messages = append(m.messages, displayMsg{role: "system", content: info})
		return m, nil

	case "/snapshot":
		return m.handleSnapshot(text)

	case "/integrity":
		saved, err := session.Load(m.sessionID)
		if err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: "Could not load current session: " + err.Error()})
			return m, nil
		}
		check := session.ValidateIntegrity(saved)
		var ib strings.Builder
		if check.Valid {
			ib.WriteString("Session integrity: VALID\n")
		} else {
			ib.WriteString("Session integrity: INVALID\n")
		}
		ib.WriteString(fmt.Sprintf("Messages: %d (user: %d, assistant: %d)\n", check.Stats.MessageCount, check.Stats.UserMessages, check.Stats.AssistantMessages))
		ib.WriteString(fmt.Sprintf("Tool uses: %d, Tool results: %d\n", check.Stats.ToolUses, check.Stats.ToolResults))
		if check.Stats.OrphanedResults > 0 {
			ib.WriteString(fmt.Sprintf("Orphaned results: %d\n", check.Stats.OrphanedResults))
		}
		for _, w := range check.Warnings {
			ib.WriteString("  warning: " + w + "\n")
		}
		for _, e := range check.Errors {
			ib.WriteString("  error: " + e + "\n")
		}
		m.messages = append(m.messages, displayMsg{role: "system", content: ib.String()})
		return m, nil
	}

	return m, nil
}
