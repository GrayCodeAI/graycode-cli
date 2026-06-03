package cmd

import (
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestProviderPickerNotice_GenericOpenAICompatibleKey(t *testing.T) {
	opts := []hawkconfig.CredentialProviderOption{
		{ProviderID: "openai", DisplayName: "OpenAI"},
		{ProviderID: "opencodego", DisplayName: "OpenCode Go"},
	}

	notice := providerPickerNotice("sk-test-key-that-is-generic", opts, false)
	if !strings.Contains(notice, "Generic OpenAI-compatible key") {
		t.Fatalf("notice = %q", notice)
	}

	help := configProviderPickerHelp(len(opts), opts)
	if strings.Contains(help, "★ = suggested") {
		t.Fatalf("generic key help should not mention suggestions: %q", help)
	}
	if !strings.Contains(help, "choose gateway manually") {
		t.Fatalf("help = %q", help)
	}
}

func TestProviderPickerNotice_InferredKey(t *testing.T) {
	opts := []hawkconfig.CredentialProviderOption{
		{ProviderID: "openrouter", DisplayName: "OpenRouter", Inferred: true},
	}

	notice := providerPickerNotice("sk-or-test-key", opts, false)
	if !strings.Contains(notice, "suggested") {
		t.Fatalf("notice = %q", notice)
	}

	help := configProviderPickerHelp(len(opts), opts)
	if !strings.Contains(help, "★ = suggested") {
		t.Fatalf("help = %q", help)
	}
}
