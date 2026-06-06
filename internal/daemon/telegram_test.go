package daemon

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// telegramMockAPI captures sendMessage calls and serves a fake Telegram + hawk
// endpoint. The Telegram bot API base is hardcoded in telegram.go, so we instead
// drive handleMessage directly and observe outbound sends via a mock daemon.
func TestTelegram_HandleMessage_Authorization(t *testing.T) {
	// Mock hawk daemon /v1/chat.
	hawk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, ChatResponse{Response: "hawk-reply"})
	}))
	defer hawk.Close()

	// Mock Telegram sendMessage endpoint by intercepting via a custom transport.
	var mu sync.Mutex
	var sends []string
	tg := newTelegramGatewayFromConfig(
		TelegramConfig{Token: "tok", PairingCode: "open"},
		hawk.URL, "apikey",
	)
	tg.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/sendMessage") {
			_ = req.ParseForm()
			mu.Lock()
			sends = append(sends, req.FormValue("text"))
			mu.Unlock()
			return jsonResp(`{"ok":true}`), nil
		}
		// forward-to-hawk goes to the real mock server; let it through.
		return http.DefaultTransport.RoundTrip(req)
	})}

	mkMsg := func(text string) *TelegramMessage {
		m := &TelegramMessage{Text: text}
		m.Chat.ID = 42
		m.From.Username = "alice"
		return m
	}

	// 1. Unauthorized non-pair message -> "Unauthorized" reply, no hawk call.
	tg.handleMessage(context.Background(), mkMsg("hello"))
	// 2. Wrong pairing code -> failure reply.
	tg.handleMessage(context.Background(), mkMsg("/pair wrong"))
	// 3. Correct pairing code -> paired reply.
	tg.handleMessage(context.Background(), mkMsg("/pair open"))
	// 4. Now authorized -> hawk reply forwarded.
	tg.handleMessage(context.Background(), mkMsg("do something"))

	mu.Lock()
	defer mu.Unlock()
	if len(sends) != 4 {
		t.Fatalf("expected 4 sends, got %d: %v", len(sends), sends)
	}
	if !strings.Contains(sends[0], "Unauthorized") {
		t.Errorf("send[0]=%q want Unauthorized", sends[0])
	}
	if !strings.Contains(sends[1], "failed") {
		t.Errorf("send[1]=%q want pairing failure", sends[1])
	}
	if !strings.Contains(sends[2], "Paired") {
		t.Errorf("send[2]=%q want Paired", sends[2])
	}
	if sends[3] != "hawk-reply" {
		t.Errorf("send[3]=%q want hawk-reply", sends[3])
	}
	if !tg.auth.allowed("alice") {
		t.Errorf("alice should be allowed after pairing")
	}
}

func TestTelegramSenderID(t *testing.T) {
	withUser := &TelegramMessage{}
	withUser.From.Username = "bob"
	withUser.Chat.ID = 7
	if got := telegramSenderID(withUser); got != "bob" {
		t.Errorf("got %q want bob", got)
	}

	noUser := &TelegramMessage{}
	noUser.Chat.ID = 99
	if got := telegramSenderID(noUser); got != "99" {
		t.Errorf("got %q want 99", got)
	}
}

func TestTelegram_NilAuthAllowsAll(t *testing.T) {
	// The bare constructor leaves auth nil: legacy behaviour, all senders pass.
	hawk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, ChatResponse{Response: "ok"})
	}))
	defer hawk.Close()

	var sends []string
	tg := NewTelegramGateway("tok", hawk.URL)
	tg.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/sendMessage") {
			_ = req.ParseForm()
			sends = append(sends, req.FormValue("text"))
			return jsonResp(`{"ok":true}`), nil
		}
		return http.DefaultTransport.RoundTrip(req)
	})}

	m := &TelegramMessage{Text: "hi"}
	m.Chat.ID = 1
	tg.handleMessage(context.Background(), m)
	if len(sends) != 1 || sends[0] != "ok" {
		t.Fatalf("expected forwarded reply 'ok', got %v", sends)
	}
}

// --- helpers ---

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResp(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}
