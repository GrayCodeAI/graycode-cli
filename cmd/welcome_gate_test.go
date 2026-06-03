package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestRenderWelcomeGate_HAWKBrandingAndFooter(t *testing.T) {
	m := chatModel{
		width:        100,
		height:       30,
		phase:        phaseWelcomeGate,
		welcomeCache: buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 100, nil, true),
	}
	out := m.renderWelcomeGate(100, 30)
	for _, want := range []string{welcomeToPhraseLines[0], "███████", "Press Enter", quitFooterHint, " · ", "↵"} {
		if !strings.Contains(out, want) {
			t.Fatalf("renderWelcomeGate missing %q", want)
		}
	}
	if strings.Contains(out, "pick model") || strings.Contains(out, "v0.") {
		t.Fatalf("welcome gate should not show model picker or version in footer:\n%s", out)
	}
	if strings.Contains(out, "Almost ready") || strings.Contains(out, "paste your API key") {
		t.Fatalf("welcome gate hero should not duplicate setup hints:\n%s", out)
	}
}

func TestBuildWelcomeMessage_GateOmitsSetupBanner(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 100, nil, true)
	if strings.Contains(out, "Almost ready") {
		t.Fatalf("gate welcome should not show Almost ready in hero:\n%s", out)
	}
	if !strings.Contains(out, "Skills") || !strings.Contains(out, " · ") {
		t.Fatalf("gate welcome should use chip separators:\n%s", out)
	}
	if !strings.Contains(out, welcomeToPhraseLines[0]) || !strings.Contains(out, "██   ██  █████") {
		t.Fatalf("gate welcome should show WELCOME TO block and HAWK logo:\n%s", out)
	}
}

func TestBuildWelcomeMessage_MediumGateStacksWelcomeTo(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, welcomeToPhraseMinWidth-1, nil, true)
	if !strings.Contains(out, welcomeWordLines[0]) || !strings.Contains(out, welcomeToWordLines[0]) {
		t.Fatalf("medium gate should stack WELCOME and TO blocks:\n%s", out)
	}
}

func TestBuildWelcomeMessage_NarrowGateShowsWelcomeFallback(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 50, nil, true)
	if !strings.Contains(out, "WELCOME TO") {
		t.Fatalf("narrow gate should show one-line welcome:\n%s", out)
	}
}

func TestRenderWelcomeGate_QuitHintNotTruncated(t *testing.T) {
	m := chatModel{
		width:        80,
		height:       24,
		phase:        phaseWelcomeGate,
		welcomeCache: buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 80, nil, true),
		input:        textarea.New(),
		viewport:     viewport.New(80, 8),
	}
	out := m.renderWelcomeGate(80, 24)
	if !strings.Contains(out, quitFooterHint) {
		t.Fatalf("quit hint should be fully visible, got:\n%s", out)
	}
}

func TestRenderWelcomeGate_ChromeAnchorsBottom(t *testing.T) {
	m := chatModel{
		width:        80,
		height:       30,
		phase:        phaseWelcomeGate,
		welcomeCache: buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 80, nil, true),
	}
	out := m.renderWelcomeGate(80, 30)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 0 || !strings.Contains(lines[len(lines)-1], quitFooterHint) {
		t.Fatalf("welcome gate chrome should anchor to bottom, got:\n%s", out)
	}
}

func TestEnterWorkPhase_DismissesWelcomeHeader(t *testing.T) {
	ta := textarea.New()
	ta.SetHeight(1)
	ta.SetWidth(76)
	m := chatModel{
		width:        80,
		height:       24,
		phase:        phaseWelcomeGate,
		welcomeCache: buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 80, nil, true),
		input:        ta,
		viewport:     viewport.New(80, 8),
	}
	next, _ := m.enterWorkPhase()
	if next.phase != phaseWork {
		t.Fatal("enter should move welcome gate to work phase")
	}
	if next.welcomeCache != "" || !next.welcomeDismissed {
		t.Fatalf("enter should dismiss welcome cache, dismissed=%v cache=%q", next.welcomeDismissed, next.welcomeCache)
	}
	next.rebuildWelcomeCache(false)
	if next.welcomeCache != "" {
		t.Fatalf("dismissed welcome should not rebuild in work phase:\n%s", next.welcomeCache)
	}
}

func TestRenderWelcomeGate_FitsShortTerminal(t *testing.T) {
	m := chatModel{
		width:        80,
		height:       18,
		phase:        phaseWelcomeGate,
		welcomeCache: buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 80, nil, true),
	}
	out := m.renderWelcomeGate(80, 18)
	if !strings.Contains(out, "Press Enter") {
		t.Fatal("short terminal should still show action row")
	}
}

func TestInitialUIPhase(t *testing.T) {
	if initialUIPhase(false, false) != phaseWelcomeGate {
		t.Fatal("expected welcome gate for fresh session")
	}
	if initialUIPhase(true, false) != phaseWork {
		t.Fatal("expected work when chat exists")
	}
	if initialUIPhase(false, true) != phaseWork {
		t.Fatal("expected work for one-shot prompt")
	}
}
