package gateway

import (
	"context"
	"testing"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// stubProvider proves the Provider interface is swappable: a test can inject a
// deterministic fake and drive the ChatClient adapter without constructing any
// Eyrie engine. This is the swappable-Engine guarantee Phase 3 delivers.
type stubProvider struct {
	resp *eyrieengine.GenerateResponse
	err  error
}

func (s *stubProvider) Resolve(context.Context, eyrieengine.SelectionRequest) (eyrieengine.Route, error) {
	return eyrieengine.Route{}, nil
}

func (s *stubProvider) Generate(context.Context, eyrieengine.GenerateRequest) (*eyrieengine.GenerateResponse, error) {
	return s.resp, s.err
}

func (s *stubProvider) Stream(context.Context, eyrieengine.GenerateRequest) (eyrieengine.EventStreamer, error) {
	return nil, nil
}

func (s *stubProvider) ListModels(context.Context, string, bool) ([]eyrieengine.Model, error) {
	return nil, nil
}

func (s *stubProvider) ListLiveModels(context.Context, string) ([]eyrieengine.Model, error) {
	return nil, nil
}

func (s *stubProvider) ListPublicModels(context.Context, string) ([]eyrieengine.Model, error) {
	return nil, nil
}

func (s *stubProvider) ModelInfo(context.Context, string) (eyrieengine.Model, bool, error) {
	return eyrieengine.Model{}, false, nil
}
func (s *stubProvider) ModelProviders(context.Context) ([]string, error)    { return nil, nil }
func (s *stubProvider) DefaultModel(context.Context, string, string) string { return "" }
func (s *stubProvider) PreferredModel(context.Context, string, eyrieengine.ModelClass, string) string {
	return ""
}

func (s *stubProvider) PreferredModels(context.Context, string, eyrieengine.ModelClass, int) []string {
	return nil
}

func (s *stubProvider) ModelClassOf(context.Context, string) eyrieengine.ModelClass {
	return ModelClassEconomical
}
func (s *stubProvider) ProviderForModel(context.Context, string) string { return "" }
func (s *stubProvider) PrimaryModel(context.Context) string             { return "" }
func (s *stubProvider) ModelNames(context.Context) []string             { return nil }
func (s *stubProvider) StatePaths() eyrieengine.StatePaths              { return eyrieengine.StatePaths{} }
func (s *stubProvider) DefaultProviderFilter(context.Context) string    { return "" }
func (s *stubProvider) Catalog(context.Context) (eyrieengine.CatalogSnapshot, error) {
	return eyrieengine.CatalogSnapshot{}, nil
}

func (s *stubProvider) RefreshCatalog(context.Context, string) (eyrieengine.CatalogSnapshot, error) {
	return eyrieengine.CatalogSnapshot{}, nil
}

func (s *stubProvider) ApplyCredentials(context.Context, string) (eyrieengine.CatalogSnapshot, error) {
	return eyrieengine.CatalogSnapshot{}, nil
}

func (s *stubProvider) SaveCredential(context.Context, string, string) (eyrieengine.CredentialStatus, error) {
	return eyrieengine.CredentialStatus{}, nil
}
func (s *stubProvider) RemoveCredential(context.Context, string) error { return nil }
func (s *stubProvider) CredentialStatus(context.Context, string) (eyrieengine.CredentialStatus, error) {
	return eyrieengine.CredentialStatus{}, nil
}
func (s *stubProvider) SaveCredentialEnv(context.Context, string, string) error { return nil }
func (s *stubProvider) HasCredentialEnv(context.Context, string) bool           { return false }
func (s *stubProvider) CredentialEnvKeys(string) []string                       { return nil }
func (s *stubProvider) ResolveCredential(context.Context, string) eyrieengine.CredentialResolution {
	return eyrieengine.CredentialResolution{}
}

func (s *stubProvider) CredentialProviders(context.Context) []eyrieengine.CredentialProvider {
	return nil
}
func (s *stubProvider) GatewayDefinitions() []eyrieengine.Gateway              { return nil }
func (s *stubProvider) Gateways(context.Context) []eyrieengine.Gateway         { return nil }
func (s *stubProvider) GatewayRegion(string) (string, bool)                    { return "", false }
func (s *stubProvider) SetGatewayRegion(context.Context, string, string) error { return nil }
func (s *stubProvider) GatewayForModel(context.Context, string) string         { return "" }
func (s *stubProvider) CanonicalModel(context.Context, string) string          { return "" }
func (s *stubProvider) ApplyGatewayEnvironment(context.Context, string)        {}
func (s *stubProvider) DeploymentRoutingEnabled(*bool) bool                    { return false }
func (s *stubProvider) DeploymentStatus(context.Context, string) (string, error) {
	return "", nil
}

func (s *stubProvider) DeploymentSummary(context.Context, string) (eyrieengine.DeploymentSummary, error) {
	return eyrieengine.DeploymentSummary{}, nil
}
func (s *stubProvider) RoutingPreview(context.Context, string) (string, error) { return "", nil }
func (s *stubProvider) CatalogHealth(context.Context) eyrieengine.CatalogHealth {
	return eyrieengine.CatalogHealth{}
}

func (s *stubProvider) Preflight(context.Context) eyrieengine.PreflightReport {
	return eyrieengine.PreflightReport{}
}

func (s *stubProvider) PreflightWithOptions(context.Context, eyrieengine.PreflightOptions) eyrieengine.PreflightReport {
	return eyrieengine.PreflightReport{}
}

func (s *stubProvider) ActiveSelection(context.Context) eyrieengine.Route { return eyrieengine.Route{} }

func (s *stubProvider) EffectiveSelection(context.Context, eyrieengine.SelectionOptions) eyrieengine.Selection {
	return eyrieengine.Selection{}
}
func (s *stubProvider) SetActiveProvider(context.Context, string) error    { return nil }
func (s *stubProvider) SetActiveModel(context.Context, string) error       { return nil }
func (s *stubProvider) SetSelection(context.Context, string, string) error { return nil }
func (s *stubProvider) ClearSelection(context.Context) error               { return nil }
func (s *stubProvider) ProviderStateSecurityStatus() eyrieengine.ProviderStateSecurity {
	return eyrieengine.ProviderStateSecurity{}
}
func (s *stubProvider) MigrateProviderSecrets() error                                 { return nil }
func (s *stubProvider) MigrateProviderSecretsContext(context.Context) error           { return nil }
func (s *stubProvider) SupportsNativeCompaction(context.Context, string, string) bool { return false }

func (s *stubProvider) CompactNative(context.Context, eyrieengine.NativeCompactionRequest) (string, error) {
	return "", nil
}

func TestStubProviderDrivesChatClient(t *testing.T) {
	stub := &stubProvider{resp: &eyrieengine.GenerateResponse{Content: "from stub", FinishReason: "end_turn"}}
	gw := &Gateway{Generator: stub}
	client := gw.ChatClient()

	got, err := client.Chat(context.Background(), []types.EyrieMessage{{Role: "user", Content: "hi"}}, types.ChatOptions{})
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
