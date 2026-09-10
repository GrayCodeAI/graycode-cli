package engine

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/token"
)

func TestPostResponseReportsTokOnlyRedactions(t *testing.T) {
	if !token.ShrikeAvailable() {
		t.Skip("shrike engine is the build-harness stub; skipping engine-dependent test")
	}
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
