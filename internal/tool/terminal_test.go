package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/terminal"
)

func TestTerminalTools_FullLifecycle(t *testing.T) {
	store := terminal.NewStore()

	createTool := TerminalCreateTool{Store: store}
	sendTool := TerminalSendTool{Store: store}
	readTool := TerminalReadTool{Store: store}
	listTool := TerminalListTool{Store: store}
	resizeTool := TerminalResizeTool{Store: store}
	killTool := TerminalKillTool{Store: store}

	ctx := context.Background()

	// 1. Create Terminal
	createInput, _ := json.Marshal(map[string]any{
		"session_id": "test-session",
		"command":    "cat",
		"rows":       24,
		"cols":       80,
	})
	createRes, err := createTool.Execute(ctx, createInput)
	if err != nil {
		t.Fatalf("TerminalCreateTool failed: %v", err)
	}

	var created struct {
		TerminalID string `json:"terminal_id"`
	}
	if err := json.Unmarshal([]byte(createRes), &created); err != nil {
		t.Fatalf("unmarshal createRes failed: %v", err)
	}
	if created.TerminalID == "" {
		t.Fatal("expected non-empty terminal ID")
	}

	defer func() {
		_, _ = killTool.Execute(ctx, []byte(`{"terminal_id":"`+created.TerminalID+`","session_id":"test-session"}`))
	}()

	// 2. List Terminals
	listRes, err := listTool.Execute(ctx, []byte(`{"session_id":"test-session"}`))
	if err != nil {
		t.Fatalf("TerminalListTool failed: %v", err)
	}
	var listed struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal([]byte(listRes), &listed)
	if listed.Count != 1 {
		t.Errorf("expected 1 terminal in list, got %d", listed.Count)
	}

	// 3. Resize Terminal
	resizeInput, _ := json.Marshal(map[string]any{
		"session_id":  "test-session",
		"terminal_id": created.TerminalID,
		"rows":        30,
		"cols":        100,
	})
	if _, err := resizeTool.Execute(ctx, resizeInput); err != nil {
		t.Fatalf("TerminalResizeTool failed: %v", err)
	}

	// 4. Send Input
	sendInput, _ := json.Marshal(map[string]any{
		"session_id":  "test-session",
		"terminal_id": created.TerminalID,
		"input":       "echo_tool_test",
	})
	if _, err := sendTool.Execute(ctx, sendInput); err != nil {
		t.Fatalf("TerminalSendTool failed: %v", err)
	}

	// 5. Read Output
	readInput, _ := json.Marshal(map[string]any{
		"session_id":  "test-session",
		"terminal_id": created.TerminalID,
		"timeout_ms":  1000,
	})
	time.Sleep(50 * time.Millisecond)
	readRes, err := readTool.Execute(ctx, readInput)
	if err != nil {
		t.Fatalf("TerminalReadTool failed: %v", err)
	}
	if !strings.Contains(readRes, "echo_tool_test") {
		t.Errorf("expected readRes to contain echo_tool_test, got %s", readRes)
	}

	// 6. Kill Terminal
	killInput, _ := json.Marshal(map[string]any{
		"session_id":  "test-session",
		"terminal_id": created.TerminalID,
	})
	killRes, err := killTool.Execute(ctx, killInput)
	if err != nil {
		t.Fatalf("TerminalKillTool failed: %v", err)
	}
	if !strings.Contains(killRes, "killed") {
		t.Errorf("expected killed status, got %s", killRes)
	}
}

func TestTerminalTools_CrossSessionRejection(t *testing.T) {
	store := terminal.NewStore()

	createTool := TerminalCreateTool{Store: store}
	sendTool := TerminalSendTool{Store: store}

	ctx := context.Background()

	createInput, _ := json.Marshal(map[string]any{
		"session_id": "session-1",
		"command":    "cat",
	})
	createRes, err := createTool.Execute(ctx, createInput)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	var created struct {
		TerminalID string `json:"terminal_id"`
	}
	_ = json.Unmarshal([]byte(createRes), &created)

	// Send from session-2 must fail
	sendInput, _ := json.Marshal(map[string]any{
		"session_id":  "session-2",
		"terminal_id": created.TerminalID,
		"input":       "unauthorized input",
	})
	_, err = sendTool.Execute(ctx, sendInput)
	if err == nil {
		t.Fatal("expected unauthorized session error, got nil")
	}
}
