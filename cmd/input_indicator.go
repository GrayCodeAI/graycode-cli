package cmd

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/GrayCodeAI/hawk/internal/feature/shellmode"
)

// InputClass represents the classification of user input.
type InputClass int

const (
	InputClassNeutral InputClass = iota // empty or undetermined
	InputClassShell                     // shell command (starts with !)
	InputClassAgent                     // AI query
	InputClassSlash                     // slash command (/config, /model, etc.)
)

// InputIndicator provides real-time visual feedback on input classification.
type InputIndicator struct {
	current InputClass
}

var (
	indicatorShell   = lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Bold(true)
	indicatorAgent   = lipgloss.NewStyle().Foreground(lipgloss.Color("200")).Bold(true)
	indicatorSlash   = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
	indicatorNeutral = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

// Classify determines the input class from the current buffer text and mode.
func (ind *InputIndicator) Classify(input string, mode shellmode.Mode) InputClass {
	if input == "" {
		ind.current = InputClassNeutral
		return InputClassNeutral
	}
	trimmed := input
	for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t') {
		trimmed = trimmed[1:]
	}
	if len(trimmed) == 0 {
		ind.current = InputClassNeutral
		return InputClassNeutral
	}
	switch {
	case trimmed[0] == '!':
		ind.current = InputClassShell
	case trimmed[0] == '/':
		ind.current = InputClassSlash
	default:
		switch mode {
		case shellmode.ModeShell:
			ind.current = InputClassShell
		case shellmode.ModeAgent:
			ind.current = InputClassAgent
		default:
			cls := shellmode.ClassifyInput(trimmed)
			if cls == shellmode.ClassShell {
				ind.current = InputClassShell
			} else {
				ind.current = InputClassAgent
			}
		}
	}
	return ind.current
}

// Render returns the colored indicator character for the current classification.
func (ind *InputIndicator) Render() string {
	switch ind.current {
	case InputClassShell:
		return indicatorShell.Render("●")
	case InputClassAgent:
		return indicatorAgent.Render("●")
	case InputClassSlash:
		return indicatorSlash.Render("●")
	default:
		return indicatorNeutral.Render("○")
	}
}

// Label returns a short text label for the current classification.
func (ind *InputIndicator) Label() string {
	switch ind.current {
	case InputClassShell:
		return indicatorShell.Render("SHELL")
	case InputClassAgent:
		return indicatorAgent.Render("AGENT")
	case InputClassSlash:
		return indicatorSlash.Render("CMD")
	default:
		return indicatorNeutral.Render("...")
	}
}
