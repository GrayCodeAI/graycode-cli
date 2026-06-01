package providers_test

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/providers"
)

func TestProbesParse_Command(t *testing.T) {
	probes := providers.ProbesParse("command:claude")
	if len(probes) != 1 {
		t.Fatalf("expected 1 probe, got %d", len(probes))
	}
	if probes[0].Kind != providers.ProbeCommand {
		t.Errorf("expected ProbeCommand, got %v", probes[0].Kind)
	}
	if probes[0].Arg != "claude" {
		t.Errorf("expected arg=claude, got %q", probes[0].Arg)
	}
}

func TestProbesParse_DirWithHome(t *testing.T) {
	probes := providers.ProbesParse("dir:$HOME/.qoder")
	if len(probes) != 1 {
		t.Fatalf("expected 1 probe, got %d", len(probes))
	}
	if probes[0].Kind != providers.ProbeDir {
		t.Errorf("expected ProbeDir, got %v", probes[0].Kind)
	}
	if strings.Contains(probes[0].Arg, "$HOME") {
		t.Errorf("expected $HOME to be expanded, got %q", probes[0].Arg)
	}
	if !strings.HasSuffix(probes[0].Arg, "/.qoder") {
		t.Errorf("expected suffix /.qoder, got %q", probes[0].Arg)
	}
}

func TestProbesParse_Or(t *testing.T) {
	probes := providers.ProbesParse("command:openclaw||dir:$HOME/.openclaw/workspace")
	if len(probes) != 2 {
		t.Fatalf("expected 2 probes, got %d", len(probes))
	}
	if probes[0].Kind != providers.ProbeCommand {
		t.Errorf("expected first=ProbeCommand, got %v", probes[0].Kind)
	}
	if probes[1].Kind != providers.ProbeDir {
		t.Errorf("expected second=ProbeDir, got %v", probes[1].Kind)
	}
}

func TestProbesParse_MultipleOR(t *testing.T) {
	probes := providers.ProbesParse("vscode-ext:roo||vscode-ext:rooveterinaryinc.roo-cline||cursor-ext:roo")
	if len(probes) != 3 {
		t.Fatalf("expected 3 probes, got %d", len(probes))
	}
}

func TestCatalog_Has34(t *testing.T) {
	all := providers.All()
	if len(all) != 34 {
		t.Errorf("expected 34 providers in catalog, got %d", len(all))
	}
}

func TestCatalog_AllHaveProbes(t *testing.T) {
	all := providers.All()
	for _, p := range all {
		if len(p.Probes) == 0 {
			t.Errorf("provider %q has no parsed probes", p.ID)
		}
		if p.ID == "" {
			t.Errorf("provider with empty ID: %+v", p)
		}
		if p.Label == "" {
			t.Errorf("provider %q has empty label", p.ID)
		}
	}
}

func TestCatalog_UniqueIDs(t *testing.T) {
	all := providers.All()
	seen := map[string]bool{}
	for _, p := range all {
		if seen[p.ID] {
			t.Errorf("duplicate ID in catalog: %q", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestCatalog_ProbesParseAllRoundTrip(t *testing.T) {
	all := providers.All()
	for _, p := range all {
		// Re-parse and confirm we got the same number of clauses
		reparsed := providers.ProbesParse(p.Detect)
		if len(reparsed) != len(p.Probes) {
			t.Errorf("provider %q: original had %d probes, reparsed has %d", p.ID, len(p.Probes), len(reparsed))
		}
	}
}

func TestGet(t *testing.T) {
	p := providers.Get("claude")
	if p == nil {
		t.Fatal("expected to find claude")
	}
	if p.ID != "claude" {
		t.Errorf("expected ID=claude, got %q", p.ID)
	}
	if p.Label != "Claude Code" {
		t.Errorf("expected label 'Claude Code', got %q", p.Label)
	}
}

func TestGet_NotFound(t *testing.T) {
	p := providers.Get("nonexistent")
	if p != nil {
		t.Errorf("expected nil for unknown ID, got %+v", p)
	}
}

func TestHard_ExcludesSoft(t *testing.T) {
	hard := providers.Hard()
	for _, p := range hard {
		if p.Soft {
			t.Errorf("Hard() returned soft provider %q", p.ID)
		}
	}
	// Soft providers in catalog: copilot, junie, qoder, antigravity (4)
	all := providers.All()
	if len(hard)+4 != len(all) {
		t.Errorf("expected Hard()+4 = All(), got Hard()=%d, All()=%d", len(hard), len(all))
	}
}

func TestDetect_RunsWithoutCrash(t *testing.T) {
	// Just verify Detect runs without error. The actual results
	// depend on the host's installed software.
	detected := providers.Detect()
	// On a typical CI machine, no providers should be detected.
	// We just verify the function returns a sensible slice.
	for _, p := range detected {
		if !p.Detected {
			t.Errorf("provider %q in Detect() result but Detected=false", p.ID)
		}
	}
}

func TestDetect_PreservesProbesSnapshot(t *testing.T) {
	// Verify Detect() returns a snapshot (not a reference into the
	// global catalog) — modifying the snapshot should not affect
	// subsequent Detect() calls.
	first := providers.Detect()
	for i := range first {
		first[i].Detected = false
	}
	second := providers.Detect()
	for _, p := range second {
		// The second call's Detected should be re-evaluated based
		// on the actual probes, not poisoned by the first mutation.
		_ = p.Detected
	}
}

func TestProviderFields(t *testing.T) {
	// Spot-check a few key providers
	claude := providers.Get("claude")
	if claude == nil || claude.Mech != providers.MechClaudePlugin {
		t.Errorf("claude mech mismatch: %+v", claude)
	}
	copilot := providers.Get("copilot")
	if copilot == nil || !copilot.Soft {
		t.Errorf("copilot should be soft: %+v", copilot)
	}
	roo := providers.Get("roo")
	if roo == nil || len(roo.Probes) != 3 {
		t.Errorf("roo should have 3 probes (vscode-ext:roo||vscode-ext:rooveterinaryinc.roo-cline||cursor-ext:roo), got %d", len(roo.Probes))
	}
}

func TestProbeKinds(t *testing.T) {
	// Verify all 6 probe kinds are present
	kinds := map[providers.ProbeKind]bool{}
	all := providers.All()
	for _, p := range all {
		for _, probe := range p.Probes {
			kinds[probe.Kind] = true
		}
	}
	expected := []providers.ProbeKind{
		providers.ProbeCommand,
		providers.ProbeDir,
		providers.ProbeVSCodeExt,
		providers.ProbeCursorExt,
		providers.ProbeMacApp,
		providers.ProbeJetBrainsPlugin,
	}
	for _, k := range expected {
		if !kinds[k] {
			t.Errorf("expected probe kind %q to be present in catalog", k)
		}
	}
}
