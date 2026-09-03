package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
	"github.com/spf13/cobra"
)

var (
	learnWhat     string
	learnWhy      string
	learnLesson   string
	learnCategory string
	learnLimit    int
	learnAll      bool
)

// learnCmd manages the cross-session lesson store.
var learnCmd = &cobra.Command{
	Use:   "learn",
	Short: "Manage lessons learned across sessions",
	Long: `Graycode persists lessons from failures (and manual entries) so future
sessions avoid repeating them. Lessons are injected into the system prompt.

  graycode learn                      List recent lessons
  graycode learn add                  Add a lesson manually
  graycode learn prompt <context>     Print the lesson-extraction prompt for a context
  graycode learn clear                Remove all lessons`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLearnList(cmd)
	},
}

var learnAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a lesson manually",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(learnWhat) == "" || strings.TrimSpace(learnLesson) == "" {
			return fmt.Errorf("--what and --lesson are required")
		}
		if learnCategory == "" {
			learnCategory = "manual"
		}
		si := engine.NewSelfImprover()
		si.Learn(strings.TrimSpace(learnWhat), strings.TrimSpace(learnWhy), strings.TrimSpace(learnLesson), strings.TrimSpace(learnCategory))
		cmd.Printf("lesson added (category: %s)\n", learnCategory)
		return nil
	},
}

var learnPromptCmd = &cobra.Command{
	Use:   "prompt <context>",
	Short: "Print the lesson-extraction prompt for a failure context",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println(engine.LearnPrompt(strings.Join(args, " ")))
		return nil
	},
}

var learnClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove all lessons",
	RunE: func(cmd *cobra.Command, args []string) error {
		si := engine.NewSelfImprover()
		n := len(si.Lessons(""))
		if n == 0 {
			cmd.Println("no lessons to clear")
			return nil
		}
		si.Clear()
		cmd.Printf("cleared %d lesson(s)\n", n)
		return nil
	},
}

func init() {
	learnAddCmd.Flags().StringVar(&learnWhat, "what", "", "what went wrong")
	learnAddCmd.Flags().StringVar(&learnWhy, "why", "", "root cause")
	learnAddCmd.Flags().StringVar(&learnLesson, "lesson", "", "what to do differently")
	learnAddCmd.Flags().StringVar(&learnCategory, "category", "manual", "code, test, design, communication, manual")
	learnCmd.Flags().IntVar(&learnLimit, "limit", 20, "max lessons to print (0 = all)")
	learnCmd.Flags().BoolVar(&learnAll, "all", false, "include all fields (also shows the why)")
	learnCmd.AddCommand(learnAddCmd)
	learnCmd.AddCommand(learnPromptCmd)
	learnCmd.AddCommand(learnClearCmd)
	rootCmd.AddCommand(learnCmd)
}

func runLearnList(cmd *cobra.Command) error {
	si := engine.NewSelfImprover()
	lessons := si.Lessons("")
	if len(lessons) == 0 {
		cmd.Println("No lessons yet. Add one with: graycode learn add --what ... --lesson ...")
		return nil
	}

	// Count by category.
	cats := map[string]int{}
	for _, e := range lessons {
		cats[e.Category]++
	}
	var catSummary []string
	for cat, count := range cats {
		catSummary = append(catSummary, fmt.Sprintf("%s (%d)", cat, count))
	}
	cmd.Printf("Lesson store: %d lesson(s) — %s\n", len(lessons), strings.Join(catSummary, ", "))

	start := 0
	if learnLimit > 0 && len(lessons) > learnLimit {
		start = len(lessons) - learnLimit
	}
	cmd.Println()
	for _, e := range lessons[start:] {
		cmd.Printf("[%s] %s\n", e.Category, e.What)
		cmd.Printf("    lesson: %s\n", e.Lesson)
		if learnAll && e.Why != "" {
			cmd.Printf("    why:    %s\n", e.Why)
		}
		cmd.Printf("    learned: %s\n", e.Timestamp.Format(time.RFC3339))
	}
	return nil
}
