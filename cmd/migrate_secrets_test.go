package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/observability/logger"
)

func TestLogMigrateProviderSecretsError_Nil_NoOutput(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(&buf, logger.Debug)

	logMigrateProviderSecretsError(l, nil)

	if buf.Len() != 0 {
		t.Errorf("expected no output for nil error, got: %q", buf.String())
	}
}

func TestLogMigrateProviderSecretsError_LogsWarn(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(&buf, logger.Debug)

	logMigrateProviderSecretsError(l, errors.New("read provider.json: permission denied"))

	out := buf.String()
	if !strings.Contains(out, "WARN") {
		t.Errorf("expected WARN level, got: %q", out)
	}
	if !strings.Contains(out, "provider secret migration failed") {
		t.Errorf("expected message about migration failure, got: %q", out)
	}
	if !strings.Contains(out, "permission denied") {
		t.Errorf("expected error message in log, got: %q", out)
	}
}

func TestLogMigrateProviderSecretsError_IncludesRemediationHint(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(&buf, logger.Debug)

	logMigrateProviderSecretsError(l, errors.New("boom"))

	out := buf.String()
	if !strings.Contains(out, "graycode /config") {
		t.Errorf("expected remediation hint mentioning `graycode /config`, got: %q", out)
	}
	if !strings.Contains(out, "keychain") {
		t.Errorf("expected remediation hint mentioning keychain, got: %q", out)
	}
}

func TestLogMigrateProviderSecretsError_RespectsLogLevel(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(&buf, logger.Error) // WARN < ERROR is filtered

	logMigrateProviderSecretsError(l, errors.New("boom"))

	if buf.Len() != 0 {
		t.Errorf("WARN should be filtered at Error level, got: %q", buf.String())
	}
}
