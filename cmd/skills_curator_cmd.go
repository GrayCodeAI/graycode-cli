package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/GrayCodeAI/hawk/internal/intelligence/skillcurator"
	"github.com/GrayCodeAI/hawk/internal/storage"
	"github.com/spf13/cobra"
)

// skillsCuratorCmd exposes the background skill curator (adopted from Hermes
// Agent) as a CLI surface: review/archive/pin/unpin over agent-created skills
// in ~/.hawk/skills.
var skillsCuratorCmd = &cobra.Command{
	Use:   "curator [command]",
	Short: "Skill lifecycle curation (status, run, pin, unpin, archive)",
	Long: `Maintain the agent-created skill collection.

  status            List skills with lifecycle status and usage
  run               Run the inactivity review now (archives cold skills)
  pin <name>        Pin a skill (auto-transitions skip pinned skills)
  unpin <name>      Remove a pin
  archive <name>    Move a skill to .archive/ (recoverable, never deleted)

The review is inactivity-triggered: only agent-created skills that have been
used before and have gone cold are archived; installed third-party skills,
never-used skills, and pinned skills are left alone.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		c, err := newCurator()
		if err != nil {
			return err
		}
		switch args[0] {
		case "status", "list":
			skills, err := c.List()
			if err != nil {
				return err
			}
			if len(skills) == 0 {
				fmt.Println("No curated skills found.")
				return nil
			}
			for _, s := range skills {
				last := "-"
				if !s.LastUsed.IsZero() {
					last = s.LastUsed.Format("2006-01-02")
				}
				fmt.Printf("%-24s %-9s uses=%-4d last=%s\n", s.Name, s.Status, s.UseCount, last)
			}
			return nil
		case "run":
			archived, err := c.ForceReview()
			if err != nil {
				return err
			}
			if len(archived) == 0 {
				fmt.Println("Review complete: nothing to archive.")
				return nil
			}
			fmt.Printf("Archived %d cold skill(s):\n", len(archived))
			for _, n := range archived {
				fmt.Printf("  - %s (recoverable from .archive/)\n", n)
			}
			return nil
		case "pin":
			return requireArg(args, func(name string) error { return c.Pin(name) })
		case "unpin":
			return requireArg(args, func(name string) error { return c.Unpin(name) })
		case "archive":
			return requireArg(args, func(name string) error { return c.Archive(name, "archived via CLI") })
		default:
			return fmt.Errorf("unknown curator command %q (use status, run, pin, unpin, archive)", args[0])
		}
	},
}

func requireArg(args []string, fn func(string) error) error {
	if len(args) < 2 {
		return fmt.Errorf("%s requires a skill name", args[0])
	}
	return fn(args[1])
}

func newCurator() (*skillcurator.Curator, error) {
	dir := filepath.Join(storage.StateDir(), "skills")
	return skillcurator.New(skillcurator.Config{
		SkillsDir: dir,
		StateFile: filepath.Join(dir, ".curator_state.json"),
	})
}

func init() {
	skillsCmd.AddCommand(skillsCuratorCmd)
}
