package cmd

import (
	"context"
	"errors"
	"image/color"
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/provider/gateway"
	"github.com/charmbracelet/x/ansi"
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
	got := ansi.Strip(formatConnectionContextLabel(m, "131k"))
	// The label is per-part colored with ANSI escapes. In a TTY test the
	// escapes are present; in a non-TTY test lipgloss strips them. We
	// just assert the plain-text segments are present either way.
	for _, want := range []string{"0k", "/131k", "ctx", "(0%)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected segment %q in %q", want, got)
		}
	}

	sess := engine.NewSession("", "", "", nil)
	sess.AddUser(strings.Repeat("a", 4000))
	m.session = sess
	got = ansi.Strip(formatConnectionContextLabel(m, "131k"))
	if !strings.Contains(got, "/131k ctx") || !strings.Contains(got, "%") {
		t.Fatalf("expected used/total ctx label, got %q", got)
	}
}

func TestContextPercentColor(t *testing.T) {
	// The percent-color helper is the single source of truth for the
	// "0k/262k ctx (0%)" traffic-light coloring. Assert each threshold.
	cases := []struct {
		pct  int
		want color.Color
	}{
		{0, doneGreen},
		{50, doneGreen},
		{79, doneGreen},
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
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	if err := store.Set(ctx, gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890"); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.SetActiveProvider(ctx, "openrouter")
	_ = hawkconfig.SetActiveModel(ctx, "moonshotai/kimi-k2.6")
	hawkconfig.RefreshConfigCredSnapshot(ctx)

	sess := engine.NewSession("openrouter", "moonshotai/kimi-k2.6", "", nil)

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
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	if err := store.Set(ctx, gateway.AccountForEnv("OPENROUTER_API_KEY"), "sk-or-test-key-1234567890"); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.ClearActiveSelection(ctx)
	_ = hawkconfig.SetActiveProvider(ctx, "openrouter")
	hawkconfig.RefreshConfigCredSnapshot(ctx)

	m := chatModel{session: &engine.Session{}}
	got := m.chatConnectionStatus()
	if got != "OpenRouter · pick model" {
		t.Fatalf("status = %q", got)
	}
}

func TestChatConnectionStatus_NoGatewayNoModel(t *testing.T) {
	hawkconfig.InvalidateConfigUICache()
	isolateCredentialHome(t)
	store := &gateway.MapStore{}
	gateway.SetDefaultStore(store)
	t.Cleanup(func() {
		gateway.SetDefaultStore(nil)
		hawkconfig.InvalidateConfigUICache()
	})

	ctx := context.Background()
	if err := store.Set(ctx, gateway.AccountForEnv("ANTHROPIC_API_KEY"), "sk-ant-test-key-long-enough"); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	hawkconfig.InvalidateConfigUICache()
	_ = hawkconfig.ClearActiveSelection(ctx)
	hawkconfig.RefreshConfigCredSnapshot(ctx)

	m := chatModel{session: &engine.Session{}}
	got := m.chatConnectionStatus()
	if got != "pick model" {
		t.Fatalf("expected pick model when no explicit selection is saved, got %q", got)
	}
}

func TestWelcomeDockerRunning_States(t *testing.T) {
	m := chatModel{containerEnabled: false}
	if m.welcomeDockerRunning() != nil {
		t.Fatal("expected nil when container mode disabled")
	}

	m.containerEnabled = true
	m.containerStatus = "checking docker…"
	if m.welcomeDockerRunning() != nil {
		t.Fatal("expected nil while container status is still checking")
	}

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

func TestStartupWarmMsg_RefreshesFooterCache(t *testing.T) {
	m := chatModel{}
	nextModel, _ := m.Update(startupWarmMsg{
		statusLeftKey:    "/tmp/project",
		statusLeftVal:    "~/project",
		statusLeftBranch: "main",
		connStatusVal:    "OpenRouter · gpt-4",
		connStatusKey:    "cache-key",
		welcomeSetup:     hawkconfig.SetupState{NeedsSetup: true},
		welcomeAgentsOK:  true,
	})
	next := nextModel.(chatModel)
	if next.statusLeftVal != "~/project" {
		t.Fatalf("statusLeftVal = %q, want %q", next.statusLeftVal, "~/project")
	}
	if next.statusLeftBranch != "main" {
		t.Fatalf("statusLeftBranch = %q, want %q", next.statusLeftBranch, "main")
	}
	if next.connStatusVal != "OpenRouter · gpt-4" {
		t.Fatalf("connStatusVal = %q, want %q", next.connStatusVal, "OpenRouter · gpt-4")
	}
	if !next.welcomeSetupState.NeedsSetup {
		t.Fatal("welcome setup snapshot should refresh from startup warm msg")
	}
	if !next.welcomeAgentsOK {
		t.Fatal("welcome agents snapshot should refresh from startup warm msg")
	}
}

func TestBuildWelcomeMessage_IncludesDockerWhenEnabled(t *testing.T) {
	running := true
	msg := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 80, 24, &running)
	if !strings.Contains(msg, "Container") {
		t.Fatalf("expected container execution badge in welcome, got:\n%s", msg)
	}
}

func TestBuildWelcomeMessage_OmitsDockerWhenDisabled(t *testing.T) {
	msg := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 80, 24, nil)
	if !strings.Contains(msg, "Container Starting") || strings.Contains(msg, "HOST") {
		t.Fatalf("expected mandatory container startup badge, got:\n%s", msg)
	}
}

func TestContainerFooterLeft_HostModeCopy(t *testing.T) {
	sess := &engine.Session{}
	bold, dim := containerFooterLeft(chatModel{session: sess, containerEnabled: false})
	if !strings.Contains(bold, "Docker:") {
		t.Fatalf("bold = %q, want Docker label", bold)
	}
	if !strings.Contains(dim, "required") || !strings.Contains(dim, "locked") {
		t.Fatalf("dim = %q, want fail-closed Docker hint", dim)
	}
}

func TestNormalizeModelDisplayName_ShortensSlug(t *testing.T) {
	got := normalizeModelDisplayName("openrouter/free", "openrouter/free")
	if got != "free" {
		t.Fatalf("expected free, got %q", got)
	}
}

func TestTrimRepeatedGatewayPrefix(t *testing.T) {
	tests := []struct {
		name    string
		gateway string
		model   string
		want    string
	}{
		{name: "colon", gateway: "Poolside", model: "Poolside: Laguna XS-2.1", want: "Laguna XS-2.1"},
		{name: "case insensitive", gateway: "Poolside", model: "poolside · Laguna S-2.1", want: "Laguna S-2.1"},
		{name: "em dash", gateway: "OpenRouter", model: "OpenRouter — Claude", want: "Claude"},
		{name: "unrelated", gateway: "Poolside", model: "Laguna XS-2.1", want: "Laguna XS-2.1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := trimRepeatedGatewayPrefix(tc.gateway, tc.model); got != tc.want {
				t.Fatalf("trimRepeatedGatewayPrefix(%q, %q) = %q, want %q", tc.gateway, tc.model, got, tc.want)
			}
		})
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
	msg := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 80, 24, nil)
	if strings.Contains(msg, "vdev") {
		t.Fatal("welcome should not show vdev; DisplayVersion should read VERSION file or dev")
	}
	if !strings.Contains(msg, "v") {
		t.Fatal("expected version line in welcome")
	}
}
