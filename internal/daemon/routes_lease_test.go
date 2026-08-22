package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/testutil"
)

func httpDo(t *testing.T, method, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestAcquireAndReleaseLease(t *testing.T) {
	t.Setenv("HAWK_STATE_DIR", t.TempDir())
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	// Acquire a lease through the real mux (so path routing sets {id}).
	resp := httpDo(t, http.MethodPost, "http://"+addr+"/v1/sessions/lease-test/lease")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("acquire status = %d, want 200", resp.StatusCode)
	}
	var acquired map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&acquired); err != nil {
		t.Fatal(err)
	}
	fence := acquired["fence"]
	if fence == "" {
		t.Fatal("acquire must return a fence")
	}

	// Releasing with the wrong fence must be rejected (single owner).
	bad := httpDo(t, http.MethodDelete, "http://"+addr+"/v1/sessions/lease-test/lease?fence=wrong")
	bad.Body.Close()
	if bad.StatusCode != http.StatusConflict {
		t.Fatalf("release with wrong fence = %d, want 409", bad.StatusCode)
	}

	// Releasing with the correct fence succeeds and clears ownership.
	ok := httpDo(t, http.MethodDelete, "http://"+addr+"/v1/sessions/lease-test/lease?fence="+fence)
	ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("release with correct fence = %d, want 200", ok.StatusCode)
	}
	if got := session.FenceOf("lease-test"); got != "" {
		t.Fatalf("fence should be cleared after release, got %q", got)
	}
}
