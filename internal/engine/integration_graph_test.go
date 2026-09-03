package engine

import (
	"strings"
	"testing"
)

func TestPostResponseReportsTokOnlyRedactions(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())
	pipeline := NewIntegrationPipeline()
	secret := "github_pat_abcdefghijklmnopqrstuvwxyz1234567890"

	result := pipeline.PostResponse("token: "+secret, nil)
	if result.SecretMatches != 1 {
		t.Fatalf("SecretMatches = %d, want 1", result.SecretMatches)
	}
	if result.SecretTypes["GitHub Fine-grained Token"] != 1 {
		t.Fatalf("SecretTypes = %#v", result.SecretTypes)
	}
	if strings.Contains(result.FormattedResponse, secret) {
		t.Fatal("formatted response retained Shrike-detected secret")
	}
}
