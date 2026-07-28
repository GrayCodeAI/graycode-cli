package cmd

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/GrayCodeAI/hawk/internal/harness"
)

type harnessSubcommand struct{}

func (h *harnessSubcommand) Name() string      { return "harness" }
func (h *harnessSubcommand) Aliases() []string { return []string{"harness-review"} }
func (h *harnessSubcommand) Description() string {
	return "review workspace AI agent harness and generate evaluation report"
}
func (h *harnessSubcommand) Usage() string { return "[review]" }
func (h *harnessSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}

	report, err := harness.EvaluateWorkspace(context.Background(), wd, harness.EvaluateOptions{TargetPath: wd})
	if err != nil {
		prompt := fmt.Sprintf("Audit the project's AI coding workflow and harness. Evaluation failed: %v", err)
		return m.startPromptCommand("/harness", prompt)
	}

	prompt := fmt.Sprintf("I ran a Hawk Agent Harness Review on this repository (%s).\n\nOverall Score: %d/100 (%s)\nPrioritized Findings (%d):\n\n%s\n\nPlease help me address the highest priority findings to improve our AI coding harness.",
		report.TargetPath, report.OverallScore, report.OverallStatus, len(report.Findings), harness.RenderMarkdown(report))

	return m.startPromptCommand("/harness", prompt)
}

func init() {
	subcommandRegistry.Register(&harnessSubcommand{})
}
