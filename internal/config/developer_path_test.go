package config

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/provider/gateway"
	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

func TestEvaluateDeveloperPath_FreshInstall(t *testing.T) {
	isolateMilestoneTest(t)
	gateway.SetDefaultStore(emptyCredentialStore{})
	t.Cleanup(func() { gateway.SetDefaultStore(nil) })

	r := EvaluateDeveloperPath(context.Background())
	if r.Ready {
		t.Fatal("expected not ready on fresh install")
	}
	if r.ChatReady {
		t.Fatal("expected chat not ready without credentials")
	}
	if !r.SecureReady {
		for _, c := range r.Checks {
			if c.Section == "Security" && c.Status == PathFail {
				t.Fatalf("unexpected security fail: %+v", c)
			}
		}
		t.Fatal("expected secure ready on fresh install (no disk secrets)")
	}
}

func TestFormatDeveloperPathReport_ContainsSections(t *testing.T) {
	isolateMilestoneTest(t)
	gateway.SetDefaultStore(emptyCredentialStore{})
	t.Cleanup(func() { gateway.SetDefaultStore(nil) })
	out := FormatDeveloperPathReport(context.Background())
	for _, want := range []string{
		"Developer path",
		"Setup",
		"Security",
		"Sandbox",
		"Ecosystem",
		"Next:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in report, got:\n%s", want, out)
		}
	}
}

func TestProviderJSONHasSecretsOnDisk_None(t *testing.T) {
	isolateMilestoneTest(t)
	has, _ := providerJSONHasSecretsOnDisk()
	if has {
		t.Fatal("expected no secrets on missing provider.json")
	}
}

func TestPlaintextCredentialFilesPresent_None(t *testing.T) {
	isolateMilestoneTest(t)
	found, paths := plaintextCredentialFilesPresent()
	if found || len(paths) > 0 {
		t.Fatalf("expected no plaintext files, got %v", paths)
	}
}

func TestPathStatusGlyph(t *testing.T) {
	if pathStatusGlyph(PathPass) != icons.CheckBold() {
		t.Fatal("pass glyph")
	}
	if pathStatusGlyph(PathFail) != icons.CloseThick() {
		t.Fatal("fail glyph")
	}
}
