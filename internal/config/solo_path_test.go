package config

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestEvaluateSoloPath_FreshInstall(t *testing.T) {
	isolateMilestoneTest(t)
	credentials.SetDefaultStore(emptyCredentialStore{})
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	r := EvaluateSoloPath(context.Background())
	if r.Ready {
		t.Fatal("expected not ready on fresh install")
	}
	if r.ChatReady {
		t.Fatal("expected chat not ready without credentials")
	}
	if !r.SecureReady {
		for _, c := range r.Checks {
			if c.Section == "Security" && c.Status == SoloFail {
				t.Fatalf("unexpected security fail: %+v", c)
			}
		}
		t.Fatal("expected secure ready on fresh install (no disk secrets)")
	}
}

func TestFormatSoloPathReport_ContainsSections(t *testing.T) {
	isolateMilestoneTest(t)
	out := FormatSoloPathReport(context.Background())
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

func TestLegacyCredentialFilesPresent_None(t *testing.T) {
	isolateMilestoneTest(t)
	found, paths := legacyCredentialFilesPresent()
	if found || len(paths) > 0 {
		t.Fatalf("expected no legacy files, got %v", paths)
	}
}

func TestSoloStatusGlyph(t *testing.T) {
	if soloStatusGlyph(SoloPass) != "✓" {
		t.Fatal("pass glyph")
	}
	if soloStatusGlyph(SoloFail) != "✗" {
		t.Fatal("fail glyph")
	}
}
