package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/sandbox"
	"github.com/GrayCodeAI/graycode-cli/internal/terminal"
)

func resolveStore(custom *terminal.Store) *terminal.Store {
	if custom != nil {
		return custom
	}
	return terminal.DefaultStore()
}

// TerminalCreateTool spawns a persistent interactive PTY terminal.
type TerminalCreateTool struct {
	Store *terminal.Store
}

func (TerminalCreateTool) Name() string      { return "TerminalCreate" }
func (TerminalCreateTool) Aliases() []string { return []string{"terminal_create", "pty_create"} }
func (TerminalCreateTool) Description() string {
	return "Spawn a persistent interactive PTY terminal session whose state persists across tool calls."
}

func (TerminalCreateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Shell or command to run (defaults to system shell e.g. /bin/bash or powershell)",
			},
			"cwd": map[string]interface{}{
				"type":        "string",
				"description": "Working directory for the terminal session",
			},
			"rows": map[string]interface{}{
				"type":        "integer",
				"description": "Initial terminal rows (default 24)",
			},
			"cols": map[string]interface{}{
				"type":        "integer",
				"description": "Initial terminal columns (default 80)",
			},
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "Session ID establishing ownership for this terminal",
			},
		},
	}
}

func (t TerminalCreateTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Command   string `json:"command"`
		CWD       string `json:"cwd"`
		Rows      int    `json:"rows"`
		Cols      int    `json:"cols"`
		SessionID string `json:"session_id"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &p); err != nil {
			return "", fmt.Errorf("invalid parameters: %w", err)
		}
	}

	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = "default"
	}
	rows := p.Rows
	if rows <= 0 {
		rows = 24
	}
	cols := p.Cols
	if cols <= 0 {
		cols = 80
	}

	sbCfg := sandbox.SandboxConfig{}
	if sbMode := sandbox.ModeFromContext(ctx); sbMode == sandbox.ModeWorkspace {
		sbCfg.Security = sandbox.SecurityWorkspace
	} else if sbMode == sandbox.ModeStrict {
		sbCfg.Security = sandbox.SecurityStrict
	}

	term, err := resolveStore(t.Store).Create(ctx, sessionID, p.CWD, p.Command, rows, cols, sbCfg)
	if err != nil {
		return "", fmt.Errorf("failed to create terminal: %w", err)
	}

	res, _ := json.Marshal(map[string]any{
		"terminal_id": term.ID,
		"session_id":  term.SessionID,
		"cwd":         term.CWD,
		"message":     fmt.Sprintf("Terminal %s created successfully.", term.ID),
	})
	return string(res), nil
}

// TerminalSendTool writes user input to an active terminal.
type TerminalSendTool struct {
	Store *terminal.Store
}

func (TerminalSendTool) Name() string      { return "TerminalSend" }
func (TerminalSendTool) Aliases() []string { return []string{"terminal_send", "pty_send"} }
func (TerminalSendTool) Description() string {
	return "Send input characters or commands to an active persistent terminal."
}

func (TerminalSendTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"terminal_id": map[string]interface{}{
				"type":        "string",
				"description": "Branded terminal identifier (e.g. 'terminal-1')",
			},
			"input": map[string]interface{}{
				"type":        "string",
				"description": "Characters, keystrokes, or command string to send to stdin",
			},
			"send_enter": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to append a newline (Enter) at the end of input (default true)",
			},
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "Owner session ID for authorization",
			},
		},
		"required": []string{"terminal_id", "input"},
	}
}

func (t TerminalSendTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		TerminalID string `json:"terminal_id"`
		Input      string `json:"input"`
		SendEnter  *bool  `json:"send_enter"`
		SessionID  string `json:"session_id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if p.TerminalID == "" {
		return "", fmt.Errorf("terminal_id is required")
	}

	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = "default"
	}

	term, err := resolveStore(t.Store).Get(sessionID, p.TerminalID)
	if err != nil {
		return "", err
	}

	enter := true
	if p.SendEnter != nil {
		enter = *p.SendEnter
	}

	if err := term.Send(p.Input, enter); err != nil {
		return "", fmt.Errorf("failed to send input: %w", err)
	}

	res, _ := json.Marshal(map[string]any{
		"terminal_id": p.TerminalID,
		"status":      "ok",
	})
	return string(res), nil
}

// TerminalReadTool reads bounded output from an active terminal.
type TerminalReadTool struct {
	Store *terminal.Store
}

func (TerminalReadTool) Name() string      { return "TerminalRead" }
func (TerminalReadTool) Aliases() []string { return []string{"terminal_read", "pty_read"} }
func (TerminalReadTool) Description() string {
	return "Read newly emitted output from an active persistent terminal with an optional timeout."
}

func (TerminalReadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"terminal_id": map[string]interface{}{
				"type":        "string",
				"description": "Branded terminal identifier (e.g. 'terminal-1')",
			},
			"max_bytes": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum bytes to read (default 65536)",
			},
			"timeout_ms": map[string]interface{}{
				"type":        "integer",
				"description": "Milliseconds to wait for output if buffer is empty (default 500ms)",
			},
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "Owner session ID for authorization",
			},
		},
		"required": []string{"terminal_id"},
	}
}

func (t TerminalReadTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		TerminalID string `json:"terminal_id"`
		MaxBytes   int    `json:"max_bytes"`
		TimeoutMS  int    `json:"timeout_ms"`
		SessionID  string `json:"session_id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if p.TerminalID == "" {
		return "", fmt.Errorf("terminal_id is required")
	}

	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = "default"
	}

	term, err := resolveStore(t.Store).Get(sessionID, p.TerminalID)
	if err != nil {
		return "", err
	}

	timeout := 500 * time.Millisecond
	if p.TimeoutMS > 0 {
		timeout = time.Duration(p.TimeoutMS) * time.Millisecond
	}

	out, alive, err := term.Read(p.MaxBytes, timeout)
	if err != nil {
		return "", fmt.Errorf("failed to read terminal: %w", err)
	}

	res, _ := json.Marshal(map[string]any{
		"terminal_id": p.TerminalID,
		"output":      out,
		"alive":       alive,
	})
	return string(res), nil
}

// TerminalListTool lists active persistent terminals for the calling session.
type TerminalListTool struct {
	Store *terminal.Store
}

func (TerminalListTool) Name() string      { return "TerminalList" }
func (TerminalListTool) Aliases() []string { return []string{"terminal_list", "pty_list"} }
func (TerminalListTool) Description() string {
	return "List active persistent PTY terminals owned by the current session."
}

func (TerminalListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "Session ID to filter terminals by",
			},
		},
	}
}

func (t TerminalListTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		SessionID string `json:"session_id"`
	}
	if len(input) > 0 {
		_ = json.Unmarshal(input, &p)
	}

	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = "default"
	}

	terms := resolveStore(t.Store).List(sessionID)
	res, _ := json.Marshal(map[string]any{
		"terminals": terms,
		"count":     len(terms),
	})
	return string(res), nil
}

// TerminalResizeTool resizes an active terminal PTY window.
type TerminalResizeTool struct {
	Store *terminal.Store
}

func (TerminalResizeTool) Name() string      { return "TerminalResize" }
func (TerminalResizeTool) Aliases() []string { return []string{"terminal_resize", "pty_resize"} }
func (TerminalResizeTool) Description() string {
	return "Resize the rows and columns of an active persistent PTY terminal."
}

func (TerminalResizeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"terminal_id": map[string]interface{}{
				"type":        "string",
				"description": "Branded terminal identifier",
			},
			"rows": map[string]interface{}{
				"type":        "integer",
				"description": "New terminal row count",
			},
			"cols": map[string]interface{}{
				"type":        "integer",
				"description": "New terminal column count",
			},
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "Owner session ID for authorization",
			},
		},
		"required": []string{"terminal_id", "rows", "cols"},
	}
}

func (t TerminalResizeTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		TerminalID string `json:"terminal_id"`
		Rows       int    `json:"rows"`
		Cols       int    `json:"cols"`
		SessionID  string `json:"session_id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if p.TerminalID == "" {
		return "", fmt.Errorf("terminal_id is required")
	}

	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = "default"
	}

	term, err := resolveStore(t.Store).Get(sessionID, p.TerminalID)
	if err != nil {
		return "", err
	}

	if err := term.Resize(p.Rows, p.Cols); err != nil {
		return "", fmt.Errorf("failed to resize terminal: %w", err)
	}

	res, _ := json.Marshal(map[string]any{
		"terminal_id": p.TerminalID,
		"rows":        p.Rows,
		"cols":        p.Cols,
		"status":      "ok",
	})
	return string(res), nil
}

// TerminalKillTool terminates an active terminal and frees resources.
type TerminalKillTool struct {
	Store *terminal.Store
}

func (TerminalKillTool) Name() string      { return "TerminalKill" }
func (TerminalKillTool) Aliases() []string { return []string{"terminal_kill", "pty_kill"} }
func (TerminalKillTool) Description() string {
	return "Terminate an active persistent terminal session."
}

func (TerminalKillTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"terminal_id": map[string]interface{}{
				"type":        "string",
				"description": "Branded terminal identifier to terminate",
			},
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "Owner session ID for authorization",
			},
		},
		"required": []string{"terminal_id"},
	}
}

func (t TerminalKillTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		TerminalID string `json:"terminal_id"`
		SessionID  string `json:"session_id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if p.TerminalID == "" {
		return "", fmt.Errorf("terminal_id is required")
	}

	sessionID := strings.TrimSpace(p.SessionID)
	if sessionID == "" {
		sessionID = "default"
	}

	if err := resolveStore(t.Store).Delete(sessionID, p.TerminalID); err != nil {
		return "", err
	}

	res, _ := json.Marshal(map[string]any{
		"terminal_id": p.TerminalID,
		"status":      "killed",
	})
	return string(res), nil
}
