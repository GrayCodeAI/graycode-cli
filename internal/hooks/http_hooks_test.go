package hooks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPDecisionHookDeny(t *testing.T) {
	ResetDecisionHooks()
	t.Cleanup(ResetDecisionHooks)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"action":  "deny",
			"reason":  "remote policy",
			"message": "no writes",
		})
	}))
	t.Cleanup(srv.Close)

	RegisterHTTPDecisionHook(HTTPHook{
		URL:      srv.URL,
		Events:   []string{"pre_tool"},
		Priority: 1,
	})

	d := ExecuteDecisionHooks("pre_tool", map[string]interface{}{"tool": "Write"})
	if d == nil || d.Action != ActionDeny {
		t.Fatalf("decision=%+v", d)
	}
	if d.Message != "no writes" {
		t.Fatalf("message=%q", d.Message)
	}
}

func TestHTTPDecisionHookUnreachableDeniesByDefault(t *testing.T) {
	ResetDecisionHooks()
	t.Cleanup(ResetDecisionHooks)

	// A hook pointing at a closed server: connection refused.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	RegisterHTTPDecisionHook(HTTPHook{
		Name:     "dead-hook",
		URL:      url,
		Events:   []string{"pre_tool"},
		Priority: 1,
	})

	d := ExecuteDecisionHooks("pre_tool", map[string]interface{}{"tool": "Bash"})
	if d == nil || d.Action != ActionDeny {
		t.Fatalf("expected deny (fail-closed) for unreachable hook, got %+v", d)
	}
}

func TestHTTPDecisionHookFailOpenExplicit(t *testing.T) {
	ResetDecisionHooks()
	t.Cleanup(ResetDecisionHooks)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	RegisterHTTPDecisionHook(HTTPHook{
		Name:     "dead-hook",
		URL:      url,
		Events:   []string{"pre_tool"},
		Priority: 1,
		FailOpen: true,
	})

	d := ExecuteDecisionHooks("pre_tool", map[string]interface{}{"tool": "Bash"})
	if d != nil {
		t.Fatalf("expected nil (explicit fail-open) for unreachable hook, got %+v", d)
	}
}

func TestHTTPDecisionHookBadStatusDeniesByDefault(t *testing.T) {
	ResetDecisionHooks()
	t.Cleanup(ResetDecisionHooks)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	RegisterHTTPDecisionHook(HTTPHook{
		Name:     "500-hook",
		URL:      srv.URL,
		Events:   []string{"pre_tool"},
		Priority: 1,
	})

	d := ExecuteDecisionHooks("pre_tool", map[string]interface{}{"tool": "Bash"})
	if d == nil || d.Action != ActionDeny {
		t.Fatalf("expected deny (fail-closed) for 500, got %+v", d)
	}
}

func TestDiscoverHookDirs(t *testing.T) {
	dirs := DiscoverHookDirs("/tmp/proj")
	if len(dirs) < 3 {
		t.Fatalf("dirs=%v", dirs)
	}
}
