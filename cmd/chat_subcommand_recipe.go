package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/GrayCodeAI/graycode-cli/internal/recipe"
)

// recipeSubcommand implements the /recipe slash command. It
// either lists available recipes (default) or runs a named recipe
// as a model prompt.
type recipeSubcommand struct{}

func (r *recipeSubcommand) Name() string        { return "recipe" }
func (r *recipeSubcommand) Aliases() []string   { return nil }
func (r *recipeSubcommand) Description() string { return "list or run a recipe" }
func (r *recipeSubcommand) Usage() string       { return "/recipe [name|list]" }
func (r *recipeSubcommand) Handle(m *chatModel, args []string, text string) (tea.Model, tea.Cmd) {
	arg := strings.TrimSpace(strings.TrimPrefix(text, "/recipe"))
	if arg == "" || arg == "list" {
		rn := recipe.NewRunner()
		recipes := rn.List()
		if len(recipes) == 0 {
			m.messages = append(m.messages, displayMsg{role: "system", content: "No recipes found in Graycode user state or .agents/recipes/"})
		} else {
			var list string
			for _, r := range recipes {
				list += fmt.Sprintf("  • %s — %s\n", r.Title, r.Description)
			}
			m.messages = append(m.messages, displayMsg{role: "system", content: "Available recipes:\n" + list})
		}
		return m, nil
	}
	rn := recipe.NewRunner()
	for _, r := range rn.List() {
		if strings.EqualFold(r.Title, arg) || strings.Contains(strings.ToLower(r.Title), strings.ToLower(arg)) {
			prompt, err := rn.Execute(context.Background(), r, nil)
			if err != nil {
				m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
				return m, nil
			}
			return m.startPromptCommand("/recipe "+r.Title, prompt)
		}
	}
	m.messages = append(m.messages, displayMsg{role: "error", content: "Recipe not found: " + arg})
	return m, nil
}

func init() {
	subcommandRegistry.Register(&recipeSubcommand{})
}
