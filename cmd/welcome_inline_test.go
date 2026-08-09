package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

type welcomeMCPStub struct {
	name   string
	server string
}

// TestWelcomeScreenReadableIcons renders the full welcome in Nerd mode and
// verifies that operational status uses font-independent, visible chips.
// PUA glyph metrics vary by terminal font, so welcome status must not depend
// on a tiny fallback glyph being legible.
func TestWelcomeScreenReadableIcons(t *testing.T) {
	icons.SetMode(icons.ModeNerd)
	defer icons.SetMode(icons.ModeASCII)

	running := true
	stopped := false
	states := []*bool{nil, &running, &stopped}
	for i, docker := range states {
		out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 100, 24, docker)
		for _, chip := range []string{"[!]", "[*]", "[net]", "[ok]", "[.]"} {
			if !strings.Contains(out, chip) {
				t.Fatalf("state %d: welcome output missing visible status chip %q:\n%s", i, chip, out)
			}
		}
		seen := make(map[rune]struct{})
		for _, r := range out {
			if r < 0xE000 || r > 0xF8FF {
				continue
			}
			seen[r] = struct{}{}
		}
		if len(seen) != 0 {
			t.Fatalf("state %d: welcome screen should not depend on PUA glyphs:\n%s", i, out)
		}
	}
}

func (s welcomeMCPStub) Name() string                       { return s.name }
func (s welcomeMCPStub) Description() string                { return "test tool" }
func (s welcomeMCPStub) Parameters() map[string]interface{} { return nil }
func (s welcomeMCPStub) Execute(context.Context, json.RawMessage) (string, error) {
	return "", nil
}
func (s welcomeMCPStub) MCPServerName() string { return s.server }

func TestBuildWelcomeMessage_InlineShowsSetupGuidance(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 100, 24, nil)
	if !strings.Contains(out, "v") {
		t.Fatalf("inline welcome should show version, got:\n%s", out)
	}
	if strings.Contains(out, "WELCOME TO") {
		t.Fatalf("inline welcome should not render the old gate banner, got:\n%s", out)
	}
}

func TestBuildWelcomeMessage_InlineShowsGuidance(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 100, 24, nil)
	for _, want := range []string{"Container Starting", "Skills (0)", "AGENTS.md", "MCPs (0)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("minimal welcome missing %q in:\n%s", want, out)
		}
	}
	for _, wantIcon := range []string{welcomeDisplayIcon("robot"), welcomeDisplayIcon("network")} {
		if !strings.Contains(out, wantIcon) {
			t.Fatalf("minimal welcome missing semantic icon %q in:\n%s", wantIcon, out)
		}
	}
	for mode, rowIcons := range map[string][]string{
		"active": {welcomeDisplayIcon("bolt"), welcomeDisplayIcon("robot"), welcomeDisplayIcon("network")},
		"nerd":   {welcomeDisplayIcon("bolt"), welcomeDisplayIcon("robot"), welcomeDisplayIcon("network")},
		"ascii":  {welcomeDisplayIcon("bolt"), welcomeDisplayIcon("robot"), welcomeDisplayIcon("network")},
	} {
		seenIcons := make(map[string]struct{}, len(rowIcons))
		for _, icon := range rowIcons {
			if _, exists := seenIcons[icon]; exists {
				t.Fatalf("%s welcome-row icon %q is reused; Skills, AGENTS.md, and MCPs must be unique", mode, icon)
			}
			seenIcons[icon] = struct{}{}
		}
	}
	for concept, footerIcon := range map[string]string{
		"tokens":   icons.Database(),
		"cost":     icons.Ruby(),
		"duration": icons.ClockOutline(),
		"branch":   icons.Branch(),
		"docker":   icons.Container(),
	} {
		if icons.Network() == footerIcon {
			t.Fatalf("MCP and footer %s must use distinct icons", concept)
		}
	}
	for _, noise := range []string{"TIP:", "ctrl+N", "/help", "/config", "Esc to dismiss"} {
		if strings.Contains(out, noise) {
			t.Fatalf("minimal welcome should omit %q, got:\n%s", noise, out)
		}
	}
}

func TestBuildWelcomeMessage_ShortTerminalUsesCompactCopy(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 72, 20, nil)
	if strings.Contains(out, "PgUp/Dn scroll chat") || strings.Contains(out, "for new session") {
		t.Fatalf("compact welcome should drop verbose descriptions, got:\n%s", out)
	}
	if !strings.Contains(out, "v") || !strings.Contains(out, "Container Starting") {
		t.Fatalf("compact welcome should keep version and execution mode, got:\n%s", out)
	}
}

func TestBuildWelcomeMessage_WideTerminalUsesHawkWordmark(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 120, 40, nil)
	for _, want := range []string{
		"___     ___    _________",
		"(\\.|\\/|./)",
		"|0\\/0|",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("wide welcome missing hawk wordmark line %q in:\n%s", want, out)
		}
	}
}

func TestBuildWelcomeMessage_HawkWordmarkBlinks(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, true, 120, 40, nil)
	if !strings.Contains(out, "|-\\/-|") {
		t.Fatalf("blinking welcome should close the hawk's eyes, got:\n%s", out)
	}
}

func TestEyeBlinkTick_CyclesEyeFrameStates(t *testing.T) {
	m := chatModel{input: textarea.New(), width: 100, height: 40}
	m.rebuildWelcomeCache()
	next, cmd := m.Update(eyeBlinkTickMsg{})
	nextModel := next.(chatModel)
	if nextModel.eyeFrame != 1 {
		t.Fatalf("eyeBlinkTickMsg eyeFrame = %d, want 1", nextModel.eyeFrame)
	}
	if cmd == nil {
		t.Fatal("eyeBlinkTickMsg should return next commands")
	}

	next2, _ := nextModel.Update(eyeFrameNextMsg{frame: 2})
	nextModel2 := next2.(chatModel)
	if nextModel2.eyeFrame != 2 {
		t.Fatalf("eyeFrameNextMsg frame 2 eyeFrame = %d, want 2", nextModel2.eyeFrame)
	}

	next3, _ := nextModel2.Update(eyeFrameNextMsg{frame: 3})
	nextModel3 := next3.(chatModel)
	if nextModel3.eyeFrame != 3 {
		t.Fatalf("eyeFrameNextMsg frame 3 eyeFrame = %d, want 3", nextModel3.eyeFrame)
	}

	next4, _ := nextModel3.Update(eyeFrameNextMsg{frame: 0})
	nextModel4 := next4.(chatModel)
	if nextModel4.eyeFrame != 0 {
		t.Fatalf("eyeFrameNextMsg frame 0 eyeFrame = %d, want 0", nextModel4.eyeFrame)
	}
}

func TestWelcomeMessage_OneLineGapBeforeStatusLine(t *testing.T) {
	out := buildWelcomeMessage(nil, "", nil, nil, hawkconfig.Settings{}, 0, false, 120, 40, nil)
	lines := strings.Split(out, "\n")
	artBottomIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "\\/") && !strings.Contains(line, "Container") {
			artBottomIdx = i
		}
	}
	if artBottomIdx == -1 {
		t.Fatalf("could not find bottom line of ASCII art in:\n%s", out)
	}
	if artBottomIdx+1 >= len(lines) || strings.TrimSpace(lines[artBottomIdx+1]) != "" {
		t.Fatalf("expected blank line (gap) immediately after ASCII art bottom line, got %q in:\n%s", lines[artBottomIdx+1], out)
	}
	if artBottomIdx+2 >= len(lines) || !strings.Contains(lines[artBottomIdx+2], "Container") {
		t.Fatalf("expected status line after gap, got %q in:\n%s", lines[artBottomIdx+2], out)
	}
}

func TestWelcomeModeBadge_IdentifiesExecutionEnvironment(t *testing.T) {
	running := true
	stopped := false
	for _, tc := range []struct {
		name   string
		docker *bool
		want   string
	}{
		{name: "starting", want: "Container Starting"},
		{name: "container", docker: &running, want: "Container"},
		{name: "required", docker: &stopped, want: "Container Required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := welcomeModeBadge(tc.docker); !strings.Contains(got, tc.want) {
				t.Fatalf("welcomeModeBadge() = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

func TestWelcomeIndicatorRow_UsesSemanticStatesAndCounts(t *testing.T) {
	tests := []struct {
		name        string
		skillsCount int
		agentsOK    bool
		mcpCount    int
		want        []string
	}{
		{
			name: "nothing configured",
			want: []string{"Skills (0)</active> <none>", "AGENTS.md</active> <none>", "MCPs (0)</active> <none>"},
		},
		{
			name:        "active counts",
			skillsCount: 4,
			agentsOK:    true,
			mcpCount:    1,
			want:        []string{"Skills (4)</active> <ready>", "AGENTS.md</active> <ready>", "MCPs (1)</active> <ready>"},
		},
		{
			name:        "mixed state",
			skillsCount: 2,
			mcpCount:    3,
			want:        []string{"Skills (2)</active> <ready>", "AGENTS.md</active> <none>", "MCPs (3)</active> <ready>"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := welcomeIndicatorRow(tc.skillsCount, tc.agentsOK, tc.mcpCount, "<active>", "<idle>", "</active>", "<ready>", "<none>")
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("welcomeIndicatorRow() missing %q in %q", want, got)
				}
			}
		})
	}

	active := welcomeIndicatorRow(1, true, 1, "<active>", "<idle>", "</active>", "<ready>", "<none>")
	if strings.Count(active, ansiBold) != 3 {
		t.Fatalf("welcomeIndicatorRow() should bold all three semantic chips, got %q", active)
	}
}

func TestConnectedMCPCount_CountsDistinctUsableServers(t *testing.T) {
	registry := tool.NewRegistry(
		welcomeMCPStub{name: "alpha-one", server: "alpha"},
		welcomeMCPStub{name: "alpha-two", server: "alpha"},
		welcomeMCPStub{name: "beta-one", server: "beta"},
		welcomeMCPStub{name: "not-mcp"},
	)

	if got := connectedMCPCount(registry); got != 2 {
		t.Fatalf("connectedMCPCount() = %d, want 2 distinct connected servers", got)
	}
	if got := connectedMCPCount(nil); got != 0 {
		t.Fatalf("connectedMCPCount(nil) = %d, want 0", got)
	}
}
