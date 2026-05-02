package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/hawk/engine"
	"github.com/GrayCodeAI/hawk/logger"
	"github.com/GrayCodeAI/hawk/session"
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

	sess := engine.NewSession(effectiveProvider, effectiveModel, systemPrompt, registry)
	sess.SetLogger(logger.New(io.Discard, logger.Error))
	if err := configureSession(sess, settings); err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	sess.PermissionFn = func(req engine.PermissionRequest) {
		fmt.Fprintf(os.Stderr, "\nAllow %s: %s [y/N] ", req.ToolName, req.Summary)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		req.Response <- answer == "y" || answer == "yes"
	}
	sess.AskUserFn = func(question string) (string, error) {
		fmt.Fprintf(os.Stderr, "\n%s\n> ", question)
		answer, _ := reader.ReadString('\n')
		return strings.TrimSpace(answer), nil
	}

	sessionID, _, err := prepareSession(sess)
	if err != nil {
		return err
	}

	sess.AddUser(text)

	// Wire timeout if --timeout flag is set
	ctx := context.Background()
	if timeout > 0 {
		cfg := engine.TimeoutConfig{Total: timeout, Countdown: true}
		var cancel context.CancelFunc
		ctx, cancel = engine.WithTimeout(ctx, cfg)
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
				fmt.Fprintf(os.Stderr, "\n[%s]\n", ev.ToolName)
			}
		case "tool_result":
			content := ev.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			if outputFormat == "stream-json" {
				writePrintEvent(sessionID, "tool_result", content, ev.ToolName)
			} else {
				fmt.Fprintf(os.Stderr, "[%s] %s\n", ev.ToolName, content)
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
	event := map[string]interface{}{
		"type":       "usage",
		"uuid":       genID(),
		"session_id": sessionID,
		"usage": map[string]int{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
		},
	}
	if usage.CacheReadTokens > 0 {
		event["usage"].(map[string]int)["cache_read_tokens"] = usage.CacheReadTokens
	}
	if usage.CacheWriteTokens > 0 {
		event["usage"].(map[string]int)["cache_write_tokens"] = usage.CacheWriteTokens
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
		"total_cost_usd": sess.Cost.Total(),
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
		for _, tc := range rm.ToolUse {
			sm.ToolUse = append(sm.ToolUse, session.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
		}
		if rm.ToolResult != nil {
			sm.ToolResult = &session.ToolResult{ToolUseID: rm.ToolResult.ToolUseID, Content: rm.ToolResult.Content, IsError: rm.ToolResult.IsError}
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
		for _, tc := range sm.ToolUse {
			em.ToolUse = append(em.ToolUse, client.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
		}
		if sm.ToolResult != nil {
			em.ToolResult = &client.ToolResult{ToolUseID: sm.ToolResult.ToolUseID, Content: sm.ToolResult.Content, IsError: sm.ToolResult.IsError}
		}
		msgs = append(msgs, em)
	}
	return msgs
}
