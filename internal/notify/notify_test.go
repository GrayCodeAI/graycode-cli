package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		old, had := osLookup(k)
		t.Setenv(k, v)
		if k == "" {
			continue
		}
		_ = old
		_ = had
	}
}

func osLookup(key string) (string, bool) { return "", false }

func TestConfiguredNone(t *testing.T) {
	withEnv(t, map[string]string{"HAWK_NOTIFY_WEBHOOK_URL": "", "HAWK_NOTIFY_TELEGRAM_TOKEN": ""})
	if Configured() {
		t.Fatal("nothing configured")
	}
	// Send is a silent no-op.
	if err := SendCompletion(Completion{Title: "x"}); err != nil {
		t.Fatalf("unconfigured send must not error: %v", err)
	}
}

func TestSendWebhookPayload(t *testing.T) {
	var got map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("HAWK_NOTIFY_WEBHOOK_URL", srv.URL)

	if err := SendCompletion(Completion{Title: "done", Body: "built it", OK: true, Source: "hawk exec", Branch: "b1"}); err != nil {
		t.Fatalf("SendCompletion: %v", err)
	}
	if got["event"] != "agent_completion" || got["title"] != "done" || got["ok"] != true || got["branch"] != "b1" {
		t.Fatalf("payload = %+v", got)
	}
}

func TestSendWebhookErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("HAWK_NOTIFY_WEBHOOK_URL", srv.URL)
	err := SendCompletion(Completion{Title: "x"})
	if err == nil || !strings.Contains(err.Error(), "webhook status 500") {
		t.Fatalf("err = %v", err)
	}
}

func TestSendTelegram(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		body = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	// Route the telegram call at the test server by overriding via webhook-style env is not
	// possible (host is fixed), so this test only exercises renderText formatting used by it.
	c := Completion{Title: "t", Body: strings.Repeat("b", 1000), OK: true, Branch: "br"}
	text := renderText(c)
	if !strings.HasPrefix(text, "✅ t") {
		t.Fatalf("text = %q", text)
	}
	if !strings.Contains(text, "branch: br") {
		t.Fatalf("branch missing: %q", text)
	}
	mid := strings.Split(text, "\n\n")[1]
	if !strings.HasSuffix(mid, "…") {
		t.Fatal("body not truncated")
	}
	_ = body
}

func TestRenderTextFailureMarker(t *testing.T) {
	if !strings.HasPrefix(renderText(Completion{Title: "f", OK: false}), "❌") {
		t.Fatal("failure marker missing")
	}
}
