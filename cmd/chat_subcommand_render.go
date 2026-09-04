package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

// renderSubcommand implements the /render slash command. It
// converts the current session (or a file path argument) to CXML
// and copies it to the clipboard.
type renderSubcommand struct{}

func (r *renderSubcommand) Name() string      { return "render" }
func (r *renderSubcommand) Aliases() []string { return nil }
func (r *renderSubcommand) Description() string {
	return "convert session to CXML and copy to clipboard"
}
func (r *renderSubcommand) Usage() string { return "/render [path]" }
func (r *renderSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	renderPath := ""
	if len(args) >= 1 {
		renderPath = strings.TrimSpace(strings.TrimPrefix(text, "/render"))
	}
	cxml, stats, err := renderCXML(renderPath)
	if err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		return m, nil
	}
	result := copyToClipboard(cxml)
	if result.FallbackPath == "" {
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s CXML copied to clipboard.\n%s", icons.FileDocument(), stats)})
	} else {
		m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Clipboard unavailable — saved CXML to %s\n%s", result.FallbackPath, stats)})
	}
	return m, nil
}

func init() {
	subcommandRegistry.Register(&renderSubcommand{})
}
