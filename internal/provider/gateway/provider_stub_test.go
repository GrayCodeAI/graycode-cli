package gateway

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
	graycoderouterengine "github.com/GrayCodeAI/graycode-router/engine"
)

// stubProvider proves the Provider interface is swappable: a test can inject a
// deterministic fake and drive the ChatClient adapter without constructing any
// GraycodeRouter engine. This is the swappable-Engine guarantee Phase 3 delivers.
type stubProvider struct {
	resp *graycoderouterengine.GenerateResponse
	err  error
}

func (s *stubProvider) Resolve(context.Context, graycoderouterengine.SelectionRequest) (graycoderouterengine.Route, error) {
	return graycoderouterengine.Route{}, nil
}

func (s *stubProvider) Generate(context.Context, graycoderouterengine.GenerateRequest) (*graycoderouterengine.GenerateResponse, error) {
	return s.resp, s.err
}

func (s *stubProvider) Stream(context.Context, graycoderouterengine.GenerateRequest) (graycoderouterengine.EventStreamer, error) {
	return nil, nil
}

func (s *stubProvider) ListModels(context.Context, string, bool) ([]graycoderouterengine.Model, error) {
	return nil, nil
}

func (s *stubProvider) ListLiveModels(context.Context, string) ([]graycoderouterengine.Model, error) {
	return nil, nil
}

func (s *stubProvider) ListPublicModels(context.Context, string) ([]graycoderouterengine.Model, error) {
	return nil, nil
}

func (s *stubProvider) ModelInfo(context.Context, string) (graycoderouterengine.Model, bool, error) {
	return graycoderouterengine.Model{}, false, nil
}
func (s *stubProvider) ModelProviders(context.Context) ([]string, error)    { return nil, nil }
func (s *stubProvider) DefaultModel(context.Context, string, string) string { return "" }
func (s *stubProvider) PreferredModel(context.Context, string, graycoderouterengine.ModelClass, string) string {
	return ""
}

func (s *stubProvider) PreferredModels(context.Context, string, graycoderouterengine.ModelClass, int) []string {
	return nil
}

func (s *stubProvider) ModelClassOf(context.Context, string) graycoderouterengine.ModelClass {
	return ModelClassEconomical
}
func (s *stubProvider) ProviderForModel(context.Context, string) string { return "" }
func (s *stubProvider) PrimaryModel(context.Context) string             { return "" }
func (s *stubProvider) ModelNames(context.Context) []string             { return nil }
func (s *stubProvider) StatePaths() graycoderouterengine.StatePaths {
	return graycoderouterengine.StatePaths{}
}
func (s *stubProvider) DefaultProviderFilter(context.Context) string { return "" }
func (s *stubProvider) Catalog(context.Context) (graycoderouterengine.CatalogSnapshot, error) {
	return graycoderouterengine.CatalogSnapshot{}, nil
}

func (s *stubProvider) RefreshCatalog(context.Context, string) (graycoderouterengine.CatalogSnapshot, error) {
	return graycoderouterengine.CatalogSnapshot{}, nil
}

func (s *stubProvider) ApplyCredentials(context.Context, string) (graycoderouterengine.CatalogSnapshot, error) {
	return graycoderouterengine.CatalogSnapshot{}, nil
}

func (s *stubProvider) SaveCredential(context.Context, string, string) (graycoderouterengine.CredentialStatus, error) {
	return graycoderouterengine.CredentialStatus{}, nil
}
func (s *stubProvider) RemoveCredential(context.Context, string) error { return nil }
func (s *stubProvider) CredentialStatus(context.Context, string) (graycoderouterengine.CredentialStatus, error) {
	return graycoderouterengine.CredentialStatus{}, nil
}
func (s *stubProvider) SaveCredentialEnv(context.Context, string, string) error { return nil }
func (s *stubProvider) HasCredentialEnv(context.Context, string) bool           { return false }
func (s *stubProvider) CredentialEnvKeys(string) []string                       { return nil }
func (s *stubProvider) ResolveCredential(context.Context, string) graycoderouterengine.CredentialResolution {
	return graycoderouterengine.CredentialResolution{}
}

func (s *stubProvider) CredentialProviders(context.Context) []graycoderouterengine.CredentialProvider {
	return nil
}

func (s *stubProvider) GatewayDefinitions() []graycoderouterengine.Gateway { return nil }

func (s *stubProvider) Gateways(context.Context) []graycoderouterengine.Gateway { return nil }
func (s *stubProvider) GatewayRegion(string) (string, bool)                     { return "", false }
func (s *stubProvider) SetGatewayRegion(context.Context, string, string) error  { return nil }
func (s *stubProvider) GatewayForModel(context.Context, string) string          { return "" }
func (s *stubProvider) CanonicalModel(context.Context, string) string           { return "" }
func (s *stubProvider) ApplyGatewayEnvironment(context.Context, string)         {}
func (s *stubProvider) DeploymentRoutingEnabled(*bool) bool                     { return false }
func (s *stubProvider) DeploymentStatus(context.Context, string) (string, error) {
	return "", nil
}

func (s *stubProvider) DeploymentSummary(context.Context, string) (graycoderouterengine.DeploymentSummary, error) {
	return graycoderouterengine.DeploymentSummary{}, nil
}
func (s *stubProvider) RoutingPreview(context.Context, string) (string, error) { return "", nil }
func (s *stubProvider) CatalogHealth(context.Context) graycoderouterengine.CatalogHealth {
	return graycoderouterengine.CatalogHealth{}
}

func (s *stubProvider) Preflight(context.Context) graycoderouterengine.PreflightReport {
	return graycoderouterengine.PreflightReport{}
}

func (s *stubProvider) PreflightWithOptions(context.Context, graycoderouterengine.PreflightOptions) graycoderouterengine.PreflightReport {
	return graycoderouterengine.PreflightReport{}
}

func (s *stubProvider) ActiveSelection(context.Context) graycoderouterengine.Route {
	return graycoderouterengine.Route{}
}

func (s *stubProvider) EffectiveSelection(context.Context, graycoderouterengine.SelectionOptions) graycoderouterengine.Selection {
	return graycoderouterengine.Selection{}
}
func (s *stubProvider) SetActiveProvider(context.Context, string) error    { return nil }
func (s *stubProvider) SetActiveModel(context.Context, string) error       { return nil }
func (s *stubProvider) SetSelection(context.Context, string, string) error { return nil }
func (s *stubProvider) ClearSelection(context.Context) error               { return nil }
func (s *stubProvider) ProviderStateSecurityStatus() graycoderouterengine.ProviderStateSecurity {
	return graycoderouterengine.ProviderStateSecurity{}
}
func (s *stubProvider) MigrateProviderSecrets() error                                 { return nil }
func (s *stubProvider) MigrateProviderSecretsContext(context.Context) error           { return nil }
func (s *stubProvider) SupportsNativeCompaction(context.Context, string, string) bool { return false }

func (s *stubProvider) CompactNative(context.Context, graycoderouterengine.NativeCompactionRequest) (string, error) {
	return "", nil
}

func TestStubProviderDrivesChatClient(t *testing.T) {
	stub := &stubProvider{resp: &graycoderouterengine.GenerateResponse{Content: "from stub", FinishReason: "end_turn"}}
	gw := &Gateway{Generator: stub}
	client := gw.ChatClient()

	got, err := client.Chat(context.Background(), []types.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, types.ChatOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Content != "from stub" {
		t.Fatalf("stub content not propagated: %+v", got)
	}
	if !client.ManagesResilience() {
		t.Fatal("expected resilience managed flag")
	}
}

var _ Provider = (*stubProvider)(nil)
