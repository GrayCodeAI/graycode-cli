package engine

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// Persona represents a specialized agent persona for party mode.
type Persona struct {
	Code  string
	Name  string
	Title string
	Icon  string
	Style string // how this persona communicates
}

// BuiltinPersonas are the default personas available in party mode.
var BuiltinPersonas = []Persona{
	{Code: "architect", Name: "Winston", Title: "System Architect", Icon: "BUILD:" + "️", Style: "Measured, trade-offs over verdicts, boring technology for stability"},
	{Code: "developer", Name: "Amelia", Title: "Senior Engineer", Icon: "DEV:", Style: "Precise, test-first, commit-message brevity, every statement citable"},
	{Code: "reviewer", Name: "Marcus", Title: "Code Reviewer", Icon: icons.Magnify(), Style: "Adversarial, must find issues, no rubber-stamping"},
	{Code: "pm", Name: "John", Title: "Product Manager", Icon: "LIST:", Style: "User value first, asks why, short questions sharp follow-ups"},
	{Code: "security", Name: "Kai", Title: "Security Engineer", Icon: "SHIELD:" + "️", Style: "Paranoid, assumes breach, checks every input/output boundary"},
	{Code: "devops", Name: "Riley", Title: "DevOps Engineer", Icon: icons.Cog() + "️", Style: "Automation-first, infrastructure as code, observability obsessed"},
}

// PartySession manages a multi-persona discussion.
type PartySession struct {
	Topic    string
	Personas []Persona
	Turns    []PartyTurn
}

// PartyTurn is a single contribution from a persona.
type PartyTurn struct {
	Persona Persona
	Content string
}

// NewPartySession creates a party mode session with selected personas.
func NewPartySession(topic string, personaCodes []string) *PartySession {
	var selected []Persona
	for _, code := range personaCodes {
		for _, p := range BuiltinPersonas {
			if p.Code == code {
				selected = append(selected, p)
				break
			}
		}
	}
	if len(selected) == 0 {
		selected = BuiltinPersonas[:3] // default: architect, developer, reviewer
	}
	return &PartySession{Topic: topic, Personas: selected}
}

// GeneratePrompt creates the system prompt for a party mode round.
func (ps *PartySession) GeneratePrompt(roundNum int) string {
	var personas []string
	for _, p := range ps.Personas {
		personas = append(personas, fmt.Sprintf("- %s %s (%s): %s", p.Icon, p.Name, p.Title, p.Style))
	}

	return fmt.Sprintf(`You are facilitating a PARTY MODE discussion. Multiple expert personas discuss a topic, each from their perspective.

TOPIC: %s

PERSONAS IN THIS SESSION:
%s

RULES:
- Each persona speaks in character (1-3 sentences each)
- They respond to each other, not just the topic
- Disagreement is encouraged — it surfaces better solutions
- After all personas speak, synthesize: what do they agree on? Where do they disagree? What's the recommended path?

ROUND %d — Each persona gives their take:`, ps.Topic, strings.Join(personas, "\n"), roundNum)
}

// FormatTurn renders a persona's contribution.
func FormatPartyTurn(p Persona, content string) string {
	return fmt.Sprintf("%s **%s** (%s):\n%s", p.Icon, p.Name, p.Title, content)
}

// ListPersonas returns a formatted list of available personas.
func ListPersonas() string {
	var lines []string
	for _, p := range BuiltinPersonas {
		lines = append(lines, fmt.Sprintf("  %s %-12s — %s (%s)", p.Icon, p.Code, p.Name, p.Title))
	}
	return strings.Join(lines, "\n")
}
