package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/charmbracelet/lipgloss"
)

func TestRenderChatConnectionStatus_ColorParts(t *testing.T) {
	rendered, vis := renderChatConnectionStatus("OpenRouter", "laguna-m.1:free", "0k/131k ctx (0%)", 0)
	if vis <= 0 {
		t.Fatalf("expected positive visible width, got %d", vis)
	}
	if !strings.Contains(rendered, "OpenRouter") || !strings.Contains(rendered, "laguna-m.1:free") || !strings.Contains(rendered, "0k/131k ctx (0%)") {
		t.Fatalf("missing status parts in %q", rendered)
	}
}

func TestParseContextWindowLabel(t *testing.T) {
	cases := map[string]int{
		"131k": 131000,
		"1m":   1_000_000,
		"1.0m": 1_000_000,
		"400k": 400000,
		"—":    0,
		"":     0,
	}
	for label, want := range cases {
		if got := parseContextWindowLabel(label); got != want {
			t.Fatalf("parseContextWindowLabel(%q) = %d, want %d", label, got, want)
		}
	}
}

func TestFormatConnectionContextLabel(t *testing.T) {
	m := chatModel{session: &engine.Session{}}
	got := formatConnectionContextLabel(m, "131k")
	// The label is per-part colored with ANSI escapes. In a TTY test the
	// escapes are present; in a non-TTY test lipgloss strips them. We
	// just assert the plain-text segments are present either way.
	for _, want := range []string{"0k", "/131k", "ctx", "(0%)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected segment %q in %q", want, got)
		}
	}

	sess := &engine.Session{}
	sess.AddUser(strings.Repeat("a", 4000))
	m.session = sess
	got = formatConnectionContextLabel(m, "131k")
	if !strings.Contains(got, "/131k ctx") || !strings.Contains(got, "%") {
		t.Fatalf("expected used/total ctx label, got %q", got)
	}
}

func TestContextPercentColor(t *testing.T) {
	// The percent-color helper is the single source of truth for the
	// "0k/262k ctx (0%)" traffic-light coloring. Assert each threshold.
	cases := []struct {
		pct  int
		want lipgloss.Color
	}{
		{0, successTeal},
		{50, successTeal},
		{79, successTeal},
		{80, warnAmber},
		{94, warnAmber},
		{95, errorCoral},
		{100, errorCoral},
	}
	for _, tc := range cases {
		if got := contextPercentColor(tc.pct); got != tc.want {
			t.Errorf("contextPercentColor(%d) = %v, want %v", tc.pct, got, tc.want)
		}
	}
}

func TestFormatContextUsedLabel(t *testing.T) {
	if got := formatContextUsedLabel(0); got != "0k" {
		t.Fatalf("got %q", got)
	}
	if got := formatContextUsedLabel(5000); got != "5k" {
		t.Fatalf("got %q", got)
	}
}

func TestChatConnectionStatus_WithModel(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.SetActiveProvider(ctx, "openrouter")
	_ = hawkconfig.SetActiveModel(ctx, "moonshotai/kimi-k2.6")

	sess := &engine.Session{}
	sess.SetProvider("openrouter")
	sess.SetModel("moonshotai/kimi-k2.6")

	m := chatModel{session: sess}
	got := m.chatConnectionStatus()
	if !strings.Contains(got, "OpenRouter · ") {
		t.Fatalf("expected gateway prefix, got %q", got)
	}
	if !strings.Contains(got, "kimi-k2.6") {
		t.Fatalf("expected model name, got %q", got)
	}
	if strings.Contains(got, "moonshotai/kimi") {
		t.Fatalf("should not show owner slug as gateway label, got %q", got)
	}
}

func TestChatConnectionStatus_KeyNoModel(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, credentials.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890")
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.ClearActiveSelection(ctx)
	_ = hawkconfig.SetActiveProvider(ctx, "openrouter")

	m := chatModel{session: &engine.Session{}}
	got := m.chatConnectionStatus()
	if got != "OpenRouter · pick model" {
		t.Fatalf("status = %q", got)
	}
}

func TestChatConnectionStatus_NoGatewayNoModel(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() {
		credentials.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	_ = store.Set(ctx, credentials.AccountForEnv("ANTHROPIC_API_KEY"), "sk-ant-test-key-long-enough")
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.ClearActiveSelection(ctx)

	m := chatModel{session: &engine.Session{}}
	got := m.chatConnectionStatus()
	if got != "pick model" {
		t.Fatalf("status = %q", got)
	}
}

func TestWelcomeDockerRunning_States(t *testing.T) {
	m := chatModel{containerEnabled: false}
	if m.welcomeDockerRunning() != nil {
		t.Fatal("expected nil when container mode disabled")
	}

	m.containerEnabled = true
	m.containerReady = true
	running := m.welcomeDockerRunning()
	if running == nil || !*running {
		t.Fatalf("expected running=true when container ready, got %v", running)
	}

	m.containerReady = false
	m.containerErr = errors.New("docker not running")
	stopped := m.welcomeDockerRunning()
	if stopped == nil || *stopped {
		t.Fatalf("expected running=false when container errored, got %v", stopped)
	}
}

func TestBuildWelcomeMessage_IncludesDockerWhenEnabled(t *testing.T) {
	running := true
	msg := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 80, &running)
	if !strings.Contains(msg, "Docker") {
		t.Fatalf("expected Docker indicator in welcome, got snippet without it")
	}
}

func TestBuildWelcomeMessage_OmitsDockerWhenDisabled(t *testing.T) {
	msg := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 80, nil)
	if strings.Contains(msg, "Docker") {
		t.Fatal("expected no Docker indicator when container mode disabled")
	}
}

func TestNormalizeModelDisplayName_ShortensSlug(t *testing.T) {
	got := normalizeModelDisplayName("openrouter/free", "openrouter/free")
	if got != "free" {
		t.Fatalf("expected free, got %q", got)
	}
}

func TestWelcomeHeader_CompactAfterChat(t *testing.T) {
	m := chatModel{
		welcomeCache: "BIG LOGO",
		messages:     []displayMsg{{role: "user", content: "Hi"}},
	}
	got := m.welcomeHeader()
	if strings.Contains(got, "BIG LOGO") {
		t.Fatal("expected compact header after chat, got full welcome")
	}
	if !strings.Contains(got, "/welcome") {
		t.Fatalf("expected compact hint, got %q", got)
	}
}

func TestShowWelcomeBanner_WithMessages(t *testing.T) {
	m := chatModel{
		welcomeCache: "welcome",
		messages: []displayMsg{
			{role: "user", content: "Hi"},
			{role: "assistant", content: "Hello"},
		},
	}
	if !m.showWelcomeBanner() {
		t.Fatal("welcome banner should stay visible after chat starts")
	}
}

func TestBuildWelcomeMessage_UsesDisplayVersion(t *testing.T) {
	SetVersion("dev")
	msg := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, false, 80, nil)
	if strings.Contains(msg, "vdev") {
		t.Fatal("welcome should not show vdev; DisplayVersion should read VERSION file or dev")
	}
	if !strings.Contains(msg, "v") {
		t.Fatal("expected version line in welcome")
	}
}
