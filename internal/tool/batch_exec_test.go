package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBatchExecRequiresAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	_, err := (BatchExecTool{}).Execute(context.Background(), json.RawMessage(
		`{"action":"submit","prompts":["hello"]}`,
	))
	if err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("err = %v", err)
	}
}

func TestBatchExecSubmitRequiresPrompts(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	_, err := (BatchExecTool{}).Execute(context.Background(), json.RawMessage(
		`{"action":"submit","prompts":[]}`,
	))
	if err == nil || !strings.Contains(err.Error(), "at least one prompt") {
		t.Fatalf("err = %v", err)
	}
}

func TestBatchExecPollRequiresID(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	_, err := (BatchExecTool{}).Execute(context.Background(), json.RawMessage(
		`{"action":"poll"}`,
	))
	if err == nil || !strings.Contains(err.Error(), "batch_id is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestBatchExecInvalidAction(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	_, err := (BatchExecTool{}).Execute(context.Background(), json.RawMessage(
		`{"action":"nope"}`,
	))
	if err == nil || !strings.Contains(err.Error(), "unsupported action") {
		t.Fatal("expected error for unsupported action")
	}
}

func TestBatchWaitPollsUntilTerminal(t *testing.T) {
	var polls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polls++
		status := "in_progress"
		if polls >= 3 {
			status = "ended"
		}
		fmt.Fprintf(w, `{"id":"b1","status":%q}`, status)
	}))
	defer srv.Close()
	oldBase := batchBaseURL
	batchBaseURL = srv.URL
	defer func() { batchBaseURL = oldBase }()

	out, err := batchWait(context.Background(), "k", "b1", 10, 1)
	if err != nil {
		t.Fatalf("batchWait: %v", err)
	}
	if !strings.Contains(out, `"status": "ended"`) {
		t.Fatalf("out = %s", out)
	}
	if polls < 3 {
		t.Fatalf("expected >=3 polls, got %d", polls)
	}
}

func TestBatchWaitHonorsRetryAfterOn429(t *testing.T) {
	var saw429 bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !saw429 {
			saw429 = true
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"id":"b1","status":"ended"}`)
	}))
	defer srv.Close()
	oldBase := batchBaseURL
	batchBaseURL = srv.URL
	defer func() { batchBaseURL = oldBase }()

	start := time.Now()
	out, err := batchWait(context.Background(), "k", "b1", 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"status": "ended"`) {
		t.Fatal("wrong status")
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("Retry-After not honored; elapsed=%v", elapsed)
	}
}

func TestBatchWaitTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"b1","status":"in_progress"}`)
	}))
	defer srv.Close()
	oldBase := batchBaseURL
	batchBaseURL = srv.URL
	defer func() { batchBaseURL = oldBase }()

	if _, err := batchWait(context.Background(), "k", "b1", 1, 1); err == nil || !strings.Contains(err.Error(), "not terminal") {
		t.Fatalf("err = %v", err)
	}
}

func TestBatchWaitRequiresID(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "k")
	_, err := (BatchExecTool{}).Execute(context.Background(), json.RawMessage(`{"action":"wait"}`))
	if err == nil || !strings.Contains(err.Error(), "batch_id is required for wait") {
		t.Fatalf("err = %v", err)
	}
}

func TestIsBatchTerminal(t *testing.T) {
	for _, ok := range []string{"ended", "completed", "failed", "expired", "canceled", "cancelled"} {
		if !isBatchTerminal(ok) {
			t.Fatalf("%q should be terminal", ok)
		}
	}
	if isBatchTerminal("in_progress") {
		t.Fatal("in_progress should not be terminal")
	}
}

func TestBatchBackoffDelayRetryAfter(t *testing.T) {
	if d := batchBackoffDelay(0, time.Second, "60"); d < 59*time.Second {
		t.Fatalf("Retry-After not applied: %v", d)
	}
	if d := batchBackoffDelay(30, time.Second, ""); d > 31*time.Second {
		t.Fatalf("cap exceeded: %v", d)
	}
}
