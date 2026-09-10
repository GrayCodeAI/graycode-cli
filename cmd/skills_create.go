package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/plugin"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
	"github.com/spf13/cobra"
)

// skillsCreateCmd proposes and persists a new skill from a description. It
// surfaces the auto-skill creation loop (Hermes-style) in the CLI: the model
// generates a SKILL.md, the name is extracted, and the skill is saved to user
// state.
var skillsCreateCmd = &cobra.Command{
	Use:   "create <description>",
	Short: "Create a new skill from a description (LLM-generated)",
	Long:  "Ask the configured model to author a SKILL.md for the given description and save it to user state.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runSkillsCreate,
}

func runSkillsCreate(_ *cobra.Command, args []string) error {
	desc := strings.Join(args, " ")
	settings, err := loadEffectiveSettings()
	if err != nil {
		return err
	}
	model, provider := effectiveModelAndProvider(settings)
	sess := newGraycodeSession(settings, provider, model, "You are a skill author.", tool.NewRegistry())

	prompt := plugin.BuildNewSkillPrompt(desc)
	resp, err := sess.Chat(context.Background(), []types.GraycodeRouterMessage{
		{Role: "user", Content: prompt},
	}, types.ChatOptions{Model: model, MaxTokens: 4096})
	if err != nil {
		return fmt.Errorf("generate skill: %w", err)
	}
	content := strings.TrimSpace(resp.Content)
	if content == "" {
		return fmt.Errorf("model returned an empty skill definition")
	}
	name := plugin.ExtractSkillName(content)
	if name == "" {
		return fmt.Errorf("could not extract a skill name from the generated SKILL.md")
	}
	path, err := plugin.SaveNewSkill(name, content)
	if err != nil {
		return err
	}
	fmt.Printf("Created skill %q at %s\n", name, path)
	return nil
}

func init() {
	skillsCmd.AddCommand(skillsCreateCmd)
}
