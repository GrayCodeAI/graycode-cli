package config

import (
	"context"

	eyrieengine "github.com/GrayCodeAI/eyrie/engine"
)

type (
	EngineModel            = eyrieengine.Model
	EnginePreflight        = eyrieengine.PreflightReport
	EnginePreflightOptions = eyrieengine.PreflightOptions
)

func CredentialStoreName() string { return eyrieengine.SecretStoreName() }

func CredentialStorageStatus(ctx context.Context) eyrieengine.CredentialStorageReport {
	return eyrieengine.CredentialStorage(ctx)
}

func MigrateLegacyCredentials(ctx context.Context) (int, error) {
	return eyrieengine.MigrateLegacyCredentials(ctx)
}

func EnginePreflightReport(ctx context.Context) EnginePreflight {
	return EnginePreflightReportWithOptions(ctx, EnginePreflightOptions{})
}

func EnginePreflightReportWithOptions(ctx context.Context, opts EnginePreflightOptions) EnginePreflight {
	engine, err := newEyrieEngine()
	if err != nil {
		return EnginePreflight{Checks: []eyrieengine.PreflightCheck{{
			Name: "engine", Status: eyrieengine.CheckFail, Detail: err.Error(),
		}}}
	}
	return engine.PreflightWithOptions(ctx, opts)
}

// EnginePreflightReportWithSettings runs preflight against one invocation's
// effective settings. Custom gateways never escape into package or process
// state, so concurrent commands can safely use different settings files.
func EnginePreflightReportWithSettings(ctx context.Context, settings Settings, opts EnginePreflightOptions) EnginePreflight {
	engine, err := NewEyrieEngineForSettings(settings)
	if err != nil {
		return EnginePreflight{Checks: []eyrieengine.PreflightCheck{{
			Name: "engine", Status: eyrieengine.CheckFail, Detail: err.Error(),
		}}}
	}
	return engine.PreflightWithOptions(ctx, opts)
}

func FormatEnginePreflight(report EnginePreflight) string {
	return eyrieengine.FormatPreflight(report)
}

func EngineGatewayRegion(providerID string) (string, bool) {
	engine, err := newEyrieEngine()
	if err != nil {
		return "", false
	}
	return engine.GatewayRegion(providerID)
}

func SetEngineGatewayRegion(ctx context.Context, providerID, region string) error {
	engine, err := newEyrieEngine()
	if err != nil {
		return err
	}
	return engine.SetGatewayRegion(ctx, providerID, region)
}

func CanonicalModelID(ctx context.Context, modelID string) string {
	engine, err := newEyrieEngine()
	if err != nil {
		return modelID
	}
	return engine.CanonicalModel(ctx, modelID)
}

func HasCredentialEnv(ctx context.Context, envVar string) bool {
	engine, err := newEyrieEngine()
	return err == nil && engine.HasCredentialEnv(ctx, envVar)
}

func CredentialGuidance(providerID, secret string) string {
	return eyrieengine.CredentialGuidance(providerID, secret)
}

func ProviderStateSecurityStatus() eyrieengine.ProviderStateSecurity {
	engine, err := newEyrieEngine()
	if err != nil {
		return eyrieengine.ProviderStateSecurity{Error: err.Error(), Detail: "Eyrie engine initialization failed"}
	}
	return engine.ProviderStateSecurityStatus()
}

func MigrateEngineProviderSecrets() error {
	engine, err := newEyrieEngine()
	if err != nil {
		return err
	}
	return engine.MigrateProviderSecrets()
}

func EngineDeploymentSummary(ctx context.Context, model string) (eyrieengine.DeploymentSummary, error) {
	engine, err := newEyrieEngine()
	if err != nil {
		return eyrieengine.DeploymentSummary{}, err
	}
	return engine.DeploymentSummary(ctx, model)
}

// newEyrieEngine is Hawk's default composition root for Eyrie's stable host
// facade. Command paths that support --settings must use
// NewEyrieEngineForSettings instead of relying on this global-settings default.
func newEyrieEngine() (*eyrieengine.Engine, error) {
	return newEyrieEngineForCustomProviders(LoadGlobalSettings().CustomProviders)
}

// ListEngineModels returns model-picker rows through Eyrie's stable facade.
func ListEngineModels(ctx context.Context, providerID string, refresh bool) ([]EngineModel, error) {
	engine, err := newEyrieEngine()
	if err != nil {
		return nil, err
	}
	return engine.ListModels(ctx, providerID, refresh)
}

func ListLiveEngineModels(ctx context.Context, providerID string) ([]EngineModel, error) {
	engine, err := newEyrieEngine()
	if err != nil {
		return nil, err
	}
	return engine.ListLiveModels(ctx, providerID)
}

func ListEngineModelsWithSettings(ctx context.Context, settings Settings, providerID string, refresh bool) ([]EngineModel, error) {
	engine, err := NewEyrieEngineForSettings(settings)
	if err != nil {
		return nil, err
	}
	return engine.ListModels(ctx, providerID, refresh)
}

func ListLiveEngineModelsWithSettings(ctx context.Context, settings Settings, providerID string) ([]EngineModel, error) {
	engine, err := NewEyrieEngineForSettings(settings)
	if err != nil {
		return nil, err
	}
	return engine.ListLiveModels(ctx, providerID)
}

func ListPublicEngineModels(ctx context.Context, providerID string) ([]EngineModel, error) {
	engine, err := newEyrieEngine()
	if err != nil {
		return nil, err
	}
	return engine.ListPublicModels(ctx, providerID)
}

func NewEyrieEngine() (*eyrieengine.Engine, error) { return newEyrieEngine() }

// NewEyrieEngineForSettings composes a fresh Eyrie engine for exactly one
// effective Hawk settings snapshot. It performs no package-global registration
// and does not mutate provider environment variables.
func NewEyrieEngineForSettings(settings Settings) (*eyrieengine.Engine, error) {
	return newEyrieEngineForCustomProviders(settings.CustomProviders)
}

func newEyrieEngineForCustomProviders(providers []CustomProviderConfig) (*eyrieengine.Engine, error) {
	return eyrieengine.New(eyrieengine.Options{CustomGateways: customGatewaysFromSettings(providers)})
}

func customGatewaysFromSettings(providers []CustomProviderConfig) []eyrieengine.CustomGateway {
	gateways := make([]eyrieengine.CustomGateway, 0, len(providers))
	for _, provider := range providers {
		if provider.Name == "" && provider.BaseURL == "" {
			continue
		}
		gateways = append(gateways, eyrieengine.CustomGateway{
			ID: provider.Name, BaseURL: provider.BaseURL,
			CredentialEnv: provider.APIKeyEnv, DefaultModel: provider.Model,
		})
	}
	return gateways
}
