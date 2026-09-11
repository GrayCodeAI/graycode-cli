package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	contracts "github.com/GrayCodeAI/eyrie/tools"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/testutil"
)

// saveTestSession isolates session storage to a temp dir and persists a
// session with the given messages, returning its ID.
func saveTestSession(t *testing.T, id string, messages []session.Message) {
	t.Helper()
	t.Setenv("HAWK_STATE_DIR", t.TempDir())

	sess := &session.Session{
		ID:       id,
		Model:    "test-model",
		Provider: "test-provider",
		Name:     "test-session",
		Messages: messages,
	}
	if err := session.Save(sess); err != nil {
		t.Fatalf("session.Save: %v", err)
	}
}

func TestDaemon_GetSession_Found(t *testing.T) {
	saveTestSession(t, "sess-detail", []session.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello", ToolUse: []contracts.ToolCall{{ID: "t1", Name: "Read"}}},
	})

	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, err := http.Get("http://" + addr + "/v1/sessions/sess-detail")
	if err != nil {
		t.Fatalf("GET /v1/sessions/sess-detail failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var detail SessionDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.ID != "sess-detail" {
		t.Errorf("ID = %q, want %q", detail.ID, "sess-detail")
	}
	if detail.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2", detail.MessageCount)
	}
	if detail.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", detail.ToolCalls)
	}
	if detail.Model != "test-model" {
		t.Errorf("Model = %q, want %q", detail.Model, "test-model")
	}
}

func TestDaemon_GetMessages_Pagination(t *testing.T) {
	msgs := make([]session.Message, 0, 5)
	for i := 0; i < 5; i++ {
		msgs = append(msgs, session.Message{Role: "user", Content: "msg"})
	}
	saveTestSession(t, "sess-messages", msgs)

	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, err := http.Get("http://" + addr + "/v1/sessions/sess-messages/messages?offset=2&limit=2")
	if err != nil {
		t.Fatalf("GET messages failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var page PaginatedResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if page.Total != 5 {
		t.Errorf("Total = %d, want 5", page.Total)
	}
	if page.Offset != 2 {
		t.Errorf("Offset = %d, want 2", page.Offset)
	}
	if page.Limit != 2 {
		t.Errorf("Limit = %d, want 2", page.Limit)
	}
	if !page.HasMore {
		t.Error("HasMore = false, want true (5 total, offset 2, limit 2 leaves 1 more)")
	}
	data, ok := page.Data.([]interface{})
	if !ok {
		t.Fatalf("Data is %T, want []interface{}", page.Data)
	}
	if len(data) != 2 {
		t.Errorf("len(Data) = %d, want 2", len(data))
	}
}

func TestDaemon_GetMessages_NoPaginationParams(t *testing.T) {
	saveTestSession(t, "sess-default-page", []session.Message{
		{Role: "user", Content: "hi"},
	})

	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, err := http.Get("http://" + addr + "/v1/sessions/sess-default-page/messages")
	if err != nil {
		t.Fatalf("GET messages failed: %v", err)
	}
	defer resp.Body.Close()

	var page PaginatedResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if page.Offset != 0 {
		t.Errorf("default Offset = %d, want 0", page.Offset)
	}
	if page.Limit != 50 {
		t.Errorf("default Limit = %d, want 50", page.Limit)
	}
	if page.HasMore {
		t.Error("HasMore = true, want false (only 1 message)")
	}
}

func TestDaemon_GetMessages_MissingSession(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, err := http.Get("http://" + addr + "/v1/sessions/does-not-exist/messages")
	if err != nil {
		t.Fatalf("GET messages failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDaemon_DeleteSession_Success(t *testing.T) {
	saveTestSession(t, "sess-to-delete", []session.Message{{Role: "user", Content: "hi"}})

	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	req, _ := http.NewRequest(http.MethodDelete, "http://"+addr+"/v1/sessions/sess-to-delete", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	// Deleting again should now 404 — the file is gone.
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("second DELETE failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("second delete: expected 404, got %d", resp2.StatusCode)
	}
}

func TestDaemon_DeleteSession_InvalidID(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	// A single path segment with a disallowed character (not a slash, which
	// the router would treat as a segment boundary rather than passing
	// through to the id-charset validation in handleDeleteSession).
	req, _ := http.NewRequest(http.MethodDelete, "http://"+addr+"/v1/sessions/bad%24id", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for id with disallowed characters, got %d", resp.StatusCode)
	}
}

func TestDaemon_DeleteSession_NotFound(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	req, _ := http.NewRequest(http.MethodDelete, "http://"+addr+"/v1/sessions/never-existed", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}
