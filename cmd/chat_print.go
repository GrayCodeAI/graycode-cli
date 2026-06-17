package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/hawk/internal/engine"
	aiwatch "github.com/GrayCodeAI/hawk/internal/engine/io"
	"github.com/GrayCodeAI/hawk/internal/engine/lifecycle"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/GrayCodeAI/hawk/internal/session"
)

// Print mode and session persistence functions extracted from chat.go

func runPrint(text string) error {
	systemPrompt, err := buildSystemPrompt()
	if err != nil {
		return err
	}

	settings, err := loadEffectiveSettings()
	if err != nil {
		return err
	}
	effectiveModel, effectiveProvider := effectiveModelAndProvider(settings)
	registry, err := defaultRegistry(settings)
	if err != nil {
		return err
	}

	sess := newHawkSession(settings, effectiveProvider, effectiveModel, systemPrompt, registry)
	sess.SetLogger(logger.New(io.Discard, logger.Error))
	if cfgErr := configureSession(sess, settings); cfgErr != nil {
		return cfgErr
	}

	reader := bufio.NewReader(os.Stdin)
	sess.PermSvc().SetPermissionFn(func(req engine.PermissionRequest) {
		_, _ = fmt.Fprintf(os.Stderr, "\nAllow %s: %s [y/N] ", req.ToolName, req.Summary)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		req.Response <- answer == "y" || answer == "yes"
	})
	sess.SetAskUserFn(func(question string) (string, error) {
		_, _ = fmt.Fprintf(os.Stderr, "\n%s\n> ", question)
		answer, _ := reader.ReadString('\n')
		return strings.TrimSpace(answer), nil
	})

	sessionID, _, err := prepareSession(sess)
	if err != nil {
		return err
	}

	sess.AddUser(text)

	// Wire timeout if --timeout flag is set
	ctx := context.Background()
	if timeout > 0 {
		cfg := lifecycle.TimeoutConfig{Total: timeout, Countdown: true}
		var cancel context.CancelFunc
		ctx, cancel = lifecycle.WithTimeout(ctx, cfg)
		defer cancel()
	}

	ch, err := sess.Stream(ctx)
	if err != nil {
		return err
	}

	var printed strings.Builder
	for ev := range ch {
		switch ev.Type {
		case "content":
			if outputFormat == "text" {
				fmt.Print(ev.Content)
			} else if outputFormat == "stream-json" {
				writePrintEvent(sessionID, "content", ev.Content, "")
			}
			printed.WriteString(ev.Content)
		case "tool_use":
			if outputFormat == "stream-json" {
				writePrintEvent(sessionID, "tool_use", "", ev.ToolName)
			} else {
				_, _ = fmt.Fprintf(os.Stderr, "\n[%s]\n", ev.ToolName)
			}
		case "tool_result":
			content := ev.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			if outputFormat == "stream-json" {
				writePrintEvent(sessionID, "tool_result", content, ev.ToolName)
			} else {
				_, _ = fmt.Fprintf(os.Stderr, "[%s] %s\n", ev.ToolName, content)
			}
		case "usage":
			if outputFormat == "stream-json" && ev.Usage != nil {
				writePrintUsageEvent(sessionID, ev.Usage)
			}
		case "error":
			if outputFormat == "stream-json" {
				writePrintResult(printed.String(), sessionID, sess, true, []string{ev.Content})
			}
			return fmt.Errorf("%s", ev.Content)
		case "done":
			switch outputFormat {
			case "text":
				if !strings.HasSuffix(printed.String(), "\n") {
					fmt.Println()
				}
			case "json":
				writePrintResult(printed.String(), sessionID, sess, false, nil)
			case "stream-json":
				writePrintResult(printed.String(), sessionID, sess, false, nil)
			}
			if !noSessionPersistence {
				saveEyrieSession(sessionID, sess)
			}
			return nil
		}
	}
	switch outputFormat {
	case "text":
		if !strings.HasSuffix(printed.String(), "\n") {
			fmt.Println()
		}
	case "json":
		writePrintResult(printed.String(), sessionID, sess, false, nil)
	case "stream-json":
		writePrintResult(printed.String(), sessionID, sess, false, nil)
	}
	if !noSessionPersistence {
		saveEyrieSession(sessionID, sess)
	}
	return nil
}

func writePrintUsageEvent(sessionID string, usage *engine.StreamUsage) {
	usageEvent := map[string]int{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
	}
	event := map[string]interface{}{
		"type":       "usage",
		"uuid":       genID(),
		"session_id": sessionID,
		"usage":      usageEvent,
	}
	if usage.CacheReadTokens > 0 {
		usageEvent["cache_read_tokens"] = usage.CacheReadTokens
	}
	if usage.CacheWriteTokens > 0 {
		usageEvent["cache_write_tokens"] = usage.CacheWriteTokens
	}
	data, _ := json.Marshal(event)
	fmt.Println(string(data))
}

func writePrintResult(result, sessionID string, sess *engine.Session, isError bool, errors []string) {
	event := map[string]interface{}{
		"type":           "result",
		"subtype":        "success",
		"is_error":       isError,
		"result":         result,
		"session_id":     sessionID,
		"uuid":           genID(),
		"total_cost_usd": sess.CostValue().Total(),
	}
	if isError {
		event["subtype"] = "error_during_execution"
		event["errors"] = errors
	}
	data, _ := json.Marshal(event)
	fmt.Println(string(data))
}

func writePrintEvent(sessionID, eventType, content, toolName string) {
	event := map[string]string{
		"type":       eventType,
		"uuid":       genID(),
		"session_id": sessionID,
	}
	if content != "" {
		event["content"] = content
	}
	if toolName != "" {
		event["tool_name"] = toolName
	}
	data, _ := json.Marshal(event)
	fmt.Println(string(data))
}

func saveEyrieSession(id string, sess *engine.Session) {
	raw := sess.RawMessages()
	if len(raw) == 0 {
		return
	}
	var msgs []session.Message
	for _, rm := range raw {
		sm := session.Message{Role: rm.Role, Content: rm.Content}
		sm.ToolUse = append(sm.ToolUse, rm.ToolUse...)
		if len(rm.ToolResults) > 0 {
			sm.ToolResults = make([]session.ToolResult, len(rm.ToolResults))
			copy(sm.ToolResults, rm.ToolResults)
		}
		msgs = append(msgs, sm)
	}
	_ = session.Save(&session.Session{
		ID:        id,
		Model:     sess.Model(),
		Provider:  sess.Provider(),
		Messages:  msgs,
		CreatedAt: time.Now(),
	})
}

func toEyrieMessages(saved []session.Message) []client.EyrieMessage {
	msgs := make([]client.EyrieMessage, 0, len(saved))
	for _, sm := range saved {
		em := client.EyrieMessage{Role: sm.Role, Content: sm.Content}
		em.ToolUse = append(em.ToolUse, sm.ToolUse...)
		if len(sm.ToolResults) > 0 {
			em.ToolResults = make([]client.ToolResult, len(sm.ToolResults))
			copy(em.ToolResults, sm.ToolResults)
		}
		msgs = append(msgs, em)
	}
	return msgs
}

// runRepl starts an interactive REPL mode for multi-turn conversation without TUI.
func runRepl() error {
	fmt.Fprintln(os.Stderr, "Hawk REPL — type 'exit' or 'quit' to leave, 'help' for commands")
	fmt.Fprintln(os.Stderr)

	systemPrompt, err := buildSystemPrompt()
	if err != nil {
		return err
	}

	settings, err := loadEffectiveSettings()
	if err != nil {
		return err
	}

	effectiveModel, effectiveProvider := effectiveModelAndProvider(settings)
	registry, err := defaultRegistry(settings)
	if err != nil {
		return err
	}

	sess := newHawkSession(settings, effectiveProvider, effectiveModel, systemPrompt, registry)
	sess.SetLogger(logger.New(io.Discard, logger.Error))
	if cfgErr := configureSession(sess, settings); cfgErr != nil {
		return cfgErr
	}

	reader := bufio.NewReader(os.Stdin)
	sess.PermSvc().SetPermissionFn(func(req engine.PermissionRequest) {
		_, _ = fmt.Fprintf(os.Stderr, "\nAllow %s: %s [y/N] ", req.ToolName, req.Summary)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		req.Response <- answer == "y" || answer == "yes"
	})
	sess.SetAskUserFn(func(question string) (string, error) {
		_, _ = fmt.Fprintf(os.Stderr, "\n%s\n> ", question)
		answer, _ := reader.ReadString('\n')
		return strings.TrimSpace(answer), nil
	})

	sessionID, _, err := prepareSession(sess)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if timeout > 0 {
		cfg := lifecycle.TimeoutConfig{Total: timeout, Countdown: true}
		var cancel context.CancelFunc
		ctx, cancel = lifecycle.WithTimeout(ctx, cfg)
		defer cancel()
	}

	for {
		_, _ = fmt.Fprint(os.Stderr, "\n> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(os.Stderr, "")
				return nil
			}
			return err
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			fmt.Fprintln(os.Stderr, "Goodbye!")
			return nil
		}
		if input == "help" {
			fmt.Fprintln(os.Stderr, "Commands:")
			fmt.Fprintln(os.Stderr, "  exit, quit  - Leave REPL")
			fmt.Fprintln(os.Stderr, "  help        - Show this help")
			fmt.Fprintln(os.Stderr, "  /tools      - List available tools")
			fmt.Fprintln(os.Stderr, "  /models     - List available models")
			fmt.Fprintln(os.Stderr, "  /session    - Show session info")
			fmt.Fprintln(os.Stderr, "")
			continue
		}

		sess.AddUser(input)

		ch, err := sess.Stream(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		var printed strings.Builder
		for ev := range ch {
			switch ev.Type {
			case "content":
				if outputFormat == "text" {
					fmt.Print(ev.Content)
				} else if outputFormat == "stream-json" {
					writePrintEvent(sessionID, "content", ev.Content, "")
				}
				printed.WriteString(ev.Content)
			case "tool_use":
				if outputFormat == "stream-json" {
					writePrintEvent(sessionID, "tool_use", "", ev.ToolName)
				} else {
					_, _ = fmt.Fprintf(os.Stderr, "\n[%s]\n", ev.ToolName)
				}
			case "tool_result":
				content := ev.Content
				if len(content) > 500 {
					content = content[:500] + "..."
				}
				if outputFormat == "stream-json" {
					writePrintEvent(sessionID, "tool_result", content, ev.ToolName)
				} else {
					_, _ = fmt.Fprintf(os.Stderr, "[%s] %s\n", ev.ToolName, content)
				}
			case "usage":
				if outputFormat == "stream-json" && ev.Usage != nil {
					writePrintUsageEvent(sessionID, ev.Usage)
				}
			case "error":
				if outputFormat == "stream-json" {
					writePrintResult(printed.String(), sessionID, sess, true, []string{ev.Content})
				}
				fmt.Fprintf(os.Stderr, "Error: %s\n", ev.Content)
			case "done":
				switch outputFormat {
				case "text":
					if !strings.HasSuffix(printed.String(), "\n") {
						fmt.Println()
					}
				case "json":
					writePrintResult(printed.String(), sessionID, sess, false, nil)
				case "stream-json":
					writePrintResult(printed.String(), sessionID, sess, false, nil)
				}
				if !noSessionPersistence {
					saveEyrieSession(sessionID, sess)
				}
			}
		}
	}
}

// watchIgnoreDirs are directory names skipped when scanning for AI directives.
var watchIgnoreDirs = []string{".git", "node_modules", "vendor", "__pycache__", ".hawk"}

// runWatch watches the working directory for AI!/AI? comment directives and
// dispatches a targeted LLM edit for each one as files change (Aider-style
// "watch files" flow). For AI! directives the agent acts immediately; for AI?
// directives it answers the embedded question. After a successful dispatch the AI
// comment token is stripped from the file.
//
// If an initialPrompt is supplied it is run once up front (preserving the prior
// behaviour of seeding the session). Detection prefers the event-driven fsnotify
// backend (io.AIWatcher.StartFsnotify); if that backend is unavailable on the
// platform it falls back to a 2s polling loop.
func runWatch(initialPrompt string) error {
	// Optional initial run to seed context, matching the prior behaviour.
	if strings.TrimSpace(initialPrompt) != "" {
		if err := runPrint(initialPrompt); err != nil {
			fmt.Fprintf(os.Stderr, "Initial run failed: %v\n", err)
		}
	}

	root := "."
	fmt.Fprintln(os.Stderr, "\n[Watching for AI!/AI? comment directives — press Ctrl+C to stop]")

	// Process any directives already present before the first change event.
	if n := processAIDirectives(root, watchIgnoreDirs); n > 0 {
		fmt.Fprintf(os.Stderr, "[%s] processed %d AI directive(s)\n", time.Now().Format("15:04:05"), n)
	}

	// Prefer the fsnotify event-driven backend. The AI!/AI? directive grammar
	// differs from AIWatcher's ai: grammar, so we hook OnChange (fires once per
	// debounced filesystem-change burst) and run our own directive sweep — rather
	// than relying on OnComment, which only matches ai: comments.
	watcher := aiwatch.NewAIWatcher(root, nil)
	watcher.OnChange = func() {
		if n := processAIDirectives(root, watchIgnoreDirs); n > 0 {
			fmt.Fprintf(os.Stderr, "[%s] processed %d AI directive(s)\n", time.Now().Format("15:04:05"), n)
		}
	}

	ctx := context.Background()
	if err := watcher.StartFsnotify(ctx); err != nil {
		// fsnotify unavailable — fall back to the polling backstop.
		fmt.Fprintf(os.Stderr, "[watch] fsnotify unavailable (%v), using polling fallback\n", err)
		return runWatchPolling(root)
	}
	return nil
}

// runWatchPolling is the polling backstop used when the fsnotify backend is not
// available. It re-scans the tree every 2 seconds and processes any directives
// found in files modified since the last sweep.
func runWatchPolling(root string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastMod := getLastModTime(root)
	for range ticker.C {
		currentMod := getLastModTime(root)
		if currentMod.After(lastMod) {
			lastMod = currentMod
			if n := processAIDirectives(root, watchIgnoreDirs); n > 0 {
				fmt.Fprintf(os.Stderr, "[%s] processed %d AI directive(s)\n", time.Now().Format("15:04:05"), n)
			}
		}
	}
	return nil
}

func getLastModTime(root string) time.Time {
	var latest time.Time
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	return latest
}
