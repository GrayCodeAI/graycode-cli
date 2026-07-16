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

func TestDiscoverHookDirs(t *testing.T) {
	dirs := DiscoverHookDirs("/tmp/proj")
	if len(dirs) < 3 {
		t.Fatalf("dirs=%v", dirs)
	}
}
