package eyrieclient

import (
	"context"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
)

func PlatformSecretStoreName() string {
	return credentials.PlatformSecretStoreName()
}

func HasSecret(ctx context.Context, key string) bool {
	return credentials.HasSecret(ctx, key)
}

func LookupSecret(ctx context.Context, envKey string) string {
	return credentials.LookupSecret(ctx, envKey)
}

func DeleteSecret(ctx context.Context, envKey string) error {
	return credentials.DeleteSecret(ctx, envKey)
}

type StorageReport = credentials.StorageReport

func FormatStorageReport(r StorageReport) string {
	return credentials.FormatStorageReport(r)
}

func StorageReportFor(ctx context.Context) StorageReport {
	return credentials.StorageReportFor(ctx)
}

func KeychainWriteAvailable(ctx context.Context) (bool, string) {
	return credentials.KeychainWriteAvailable(ctx)
}

func MigrateLegacyEnvFile(ctx context.Context) (int, error) {
	return credentials.MigrateLegacyEnvFile(ctx)
}

func ValidateCredentialSecret(envKey, secret string) error {
	return eyriecfg.ValidateCredentialSecret(envKey, secret)
}
