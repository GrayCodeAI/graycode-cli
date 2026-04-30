package tool

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestSendMessageTool(t *testing.T) {
	input, _ := json.Marshal(map[string]string{
		"to":      "worker-1",
		"message": "please start on task_1",
		"summary": "task assignment",
	})
	result, err := (SendMessageTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Message string `json:"message"`
		SentAt  string `json:"sentAt"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Message != "please start on task_1" {
		t.Fatalf("expected message content, got %q", resp.Message)
	}
	if resp.SentAt == "" {
		t.Fatal("expected sentAt timestamp")
	}
}

func TestSendMessageTool_MissingTo(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"message": "hello"})
	_, err := (SendMessageTool{}).Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing 'to'")
	}
}

func TestSendMessageTool_MissingMessage(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"to": "user"})
	_, err := (SendMessageTool{}).Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing 'message'")
	}
}

func TestMailbox(t *testing.T) {
	mb := &Mailbox{}
	mb.Send(MessageRouting{Sender: "a", Target: "b", Content: "hello"})
	mb.Send(MessageRouting{Sender: "a", Target: "*", Content: "broadcast"})
	mb.Send(MessageRouting{Sender: "a", Target: "c", Content: "not for b"})

	msgs := mb.ReadFor("b")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages for 'b', got %d", len(msgs))
	}
}

func TestSleepTool(t *testing.T) {
	input, _ := json.Marshal(map[string]any{"duration_ms": 50})
	start := time.Now()
	result, err := (SleepTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Fatalf("sleep was too short: %v", elapsed)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestSleepTool_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	input, _ := json.Marshal(map[string]any{"duration_ms": 60000})

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := (SleepTool{}).Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Sleep interrupted" {
		t.Fatalf("expected 'Sleep interrupted', got %q", result)
	}
}

func TestSleepTool_MaxCap(t *testing.T) {
	input, _ := json.Marshal(map[string]any{"duration_ms": 999999})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := (SleepTool{}).Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Sleep interrupted" {
		t.Fatalf("expected interruption due to context timeout, got %q", result)
	}
}
