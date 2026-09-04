package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

// startSubcommand is the first-run / guided success path.
type startSubcommand struct{}

func (c *startSubcommand) Name() string      { return "start" }
func (c *startSubcommand) Aliases() []string { return []string{"onboard", "quickstart"} }
func (c *startSubcommand) Description() string {
	return "guided setup: trust, mode, branch, first task"
}
func (c *startSubcommand) Usage() string { return "/start [trust] [branch]" }

func (c *startSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	autoTrust := false
	autoBranch := false
	for _, a := range args {
		switch strings.ToLower(a) {
		case "trust":
			autoTrust = true
		case "branch", "agent-branch":
			autoBranch = true
		}
	}

	var b strings.Builder
	b.WriteString("## Graycode quick start\n\n")

	// 1. Model / session
	if m.session != nil {
		b.WriteString(fmt.Sprintf("1. **Session** %s · %s/%s\n",
			m.sessionID, m.session.Provider(), m.session.Model()))
	} else {
		b.WriteString("1. **Session** missing — restart chat.\n")
	}

	// 2. Trust
	tr := engine.ProjectTrust("")
	if autoTrust && tr.Enforced && !tr.Trusted {
		if err := engine.TrustProject("", "onboarding /start trust"); err == nil {
			tr = engine.ProjectTrust("")
			b.WriteString("2. **Trust** — trusted this folder for project automation.\n")
		} else {
			b.WriteString(fmt.Sprintf("2. **Trust** — failed: %v\n", err))
		}
	} else if tr.Blocked {
		b.WriteString("2. **Trust** — project is NOT trusted (hooks/MCP blocked).\n")
		b.WriteString("   → `/trust add` or `/start trust`\n")
	} else {
		b.WriteString(fmt.Sprintf("2. **Trust** — %s\n", tr.String()))
	}

	// 3. Work mode
	if m.session != nil {
		_ = m.session.SetWorkMode(engine.WorkModeAct)
		b.WriteString(fmt.Sprintf("3. **Work mode** → %s (use `/mode plan` to research first)\n", m.session.WorkMode()))
		b.WriteString(fmt.Sprintf("4. **Isolation** → %s (`/isolation workspace` for safer shell)\n", m.session.Isolation().String()))
	}

	// 5. Git branch
	gi := engine.InspectGitBranch("")
	if autoBranch && gi.HasRepo && gi.OnDefault {
		name, err := engine.EnsureAgentBranch("")
		if err != nil {
			b.WriteString(fmt.Sprintf("5. **Git** — could not create agent branch: %v\n", err))
		} else {
			b.WriteString(fmt.Sprintf("5. **Git** — created and checked out `%s`\n", name))
			_, _ = m.refreshStatusBarLeft(true)
		}
	} else if advice := engine.GitSafetyAdvice(gi); advice != "" {
		b.WriteString(fmt.Sprintf("5. **Git** — %s\n", advice))
		if gi.OnDefault {
			b.WriteString("   → `/branch-agent` or `/start branch`\n")
		}
	} else {
		b.WriteString("5. **Git** — not a git repo (ok for scratch work)\n")
	}

	// 6. First tasks
	b.WriteString("\n### Try one of these\n")
	b.WriteString("- *Explain the project layout*\n")
	b.WriteString("- *Run the test suite and fix failures*\n")
	b.WriteString("- *`/mode plan` then: plan a small safe refactor*\n")
	b.WriteString("\n### Power shortcuts\n")
	b.WriteString("`/mode plan|act|review` · `/isolation` · `/trust` · `/cost` · `/help`\n")

	m.messages = append(m.messages, displayMsg{role: "system", content: strings.TrimSpace(b.String())})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&startSubcommand{})
}
