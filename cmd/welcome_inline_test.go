package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

type welcomeMCPStub struct {
	name   string
	server string
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
	for _, want := range []string{"CONTAINER · STARTING", "Skills (0)", "AGENTS.md", "MCPs (0)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("minimal welcome missing %q in:\n%s", want, out)
		}
	}
	for _, wantIcon := range []string{icons.Robot(), icons.Network()} {
		if !strings.Contains(out, wantIcon) {
			t.Fatalf("minimal welcome missing semantic icon %q in:\n%s", wantIcon, out)
		}
	}
	for mode, rowIcons := range map[string][]string{
		"active": {icons.Bolt(), icons.Robot(), icons.Network()},
		"nerd":   {icons.Nerd("bolt"), icons.Nerd("robot"), icons.Nerd("network")},
		"ascii":  {icons.ASCII("bolt"), icons.ASCII("robot"), icons.ASCII("network")},
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
	if !strings.Contains(out, "v") || !strings.Contains(out, "CONTAINER · STARTING") {
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

func TestWelcomeModeBadge_IdentifiesExecutionEnvironment(t *testing.T) {
	running := true
	stopped := false
	for _, tc := range []struct {
		name   string
		docker *bool
		want   string
	}{
		{name: "starting", want: "CONTAINER · STARTING"},
		{name: "container", docker: &running, want: "CONTAINER · DOCKER · ISOLATED"},
		{name: "required", docker: &stopped, want: "CONTAINER · DOCKER REQUIRED"},
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
			want: []string{"Skills (0)</active> <none>", "AGENTS.md <none>", "MCPs (0)</active> <none>"},
		},
		{
			name:        "active counts",
			skillsCount: 4,
			agentsOK:    true,
			mcpCount:    1,
			want:        []string{"Skills (4)</active> <ready>", "AGENTS.md <ready>", "MCPs (1)</active> <ready>"},
		},
		{
			name:        "mixed state",
			skillsCount: 2,
			mcpCount:    3,
			want:        []string{"Skills (2)</active> <ready>", "AGENTS.md <none>", "MCPs (3)</active> <ready>"},
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
