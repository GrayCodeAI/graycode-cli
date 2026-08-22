package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusEndpointReturnsRedactedSnapshot(t *testing.T) {
	s := New(DefaultConfig(), nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	s.handleStatus(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body struct {
		SchemaVersion string `json:"schema_version"`
		Permission    struct {
			SecretRedacted bool `json:"secret_values_redacted"`
		} `json:"permission"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.SchemaVersion != "1" {
		t.Fatalf("schema_version = %q, want 1", body.SchemaVersion)
	}
	if !body.Permission.SecretRedacted {
		t.Fatal("status must mark sensitive values as redacted")
	}
}
