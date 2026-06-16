package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

var reviewStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show review queue summary",
	RunE:  runReviewStatus,
}

var reviewShowCmd = &cobra.Command{
	Use:   "show [sha|id]",
	Short: "Display review findings for a commit",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runReviewShow,
}

var reviewCloseCmd = &cobra.Command{
	Use:   "close <id|sha>",
	Short: "Close a review (mark as resolved)",
	Args:  cobra.ExactArgs(1),
	RunE:  runReviewClose,
}

var reviewListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all reviews",
	RunE:  runReviewList,
}

var (
	reviewShowFormat string
	reviewListLimit  int
)

func init() {
	reviewShowCmd.Flags().StringVar(&reviewShowFormat, "format", "terminal", "Output format: terminal, json")
	reviewListCmd.Flags().IntVarP(&reviewListLimit, "limit", "n", 20, "Max reviews to show")
	reviewCmd.AddCommand(reviewStatusCmd)
	reviewCmd.AddCommand(reviewShowCmd)
	reviewCmd.AddCommand(reviewCloseCmd)
	reviewCmd.AddCommand(reviewListCmd)
}

func runReviewStatus(_ *cobra.Command, _ []string) error {
	projectDir, _ := os.Getwd()
	store, err := OpenReviewStore(projectDir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = store.Close() }()

	summary, err := store.Summary()
	if err != nil {
		return err
	}

	total := 0
	for _, v := range summary {
		total += v
	}
	if total == 0 {
		fmt.Println("No reviews yet. Run 'hawk review init' to start.")
		return nil
	}

	open := summary[ReviewStatusOpen]
	passed := summary[ReviewStatusPassed]
	fixed := summary[ReviewStatusFixed]
	failed := summary[ReviewStatusFailed]

	fmt.Printf("Reviews: %d total", total)
	if open > 0 {
		fmt.Printf(" · %d open", open)
	}
	if passed > 0 {
		fmt.Printf(" · %d passed", passed)
	}
	if fixed > 0 {
		fmt.Printf(" · %d fixed", fixed)
	}
	if failed > 0 {
		fmt.Printf(" · %d failed", failed)
	}
	fmt.Println()

	// Show open reviews briefly.
	if open > 0 {
		reviews, _ := store.ListOpen()
		fmt.Println()
		for _, r := range reviews {
			fmt.Printf("  #%d %s [%s] %d findings\n", r.ID, r.SHA[:8], r.MaxSeverity, len(r.Findings))
		}
	}
	return nil
}

func runReviewShow(_ *cobra.Command, args []string) error {
	projectDir, _ := os.Getwd()
	store, err := OpenReviewStore(projectDir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = store.Close() }()

	var review *ReviewRecord
	if len(args) == 0 {
		// Show latest open review.
		reviews, _ := store.ListOpen()
		if len(reviews) == 0 {
			fmt.Println("No open reviews.")
			return nil
		}
		review = reviews[0]
	} else {
		review, err = resolveReview(store, args[0])
		if err != nil {
			return err
		}
	}

	if reviewShowFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(review)
	}

	printReviewDetail(review)
	return nil
}

func runReviewClose(_ *cobra.Command, args []string) error {
	projectDir, _ := os.Getwd()
	store, err := OpenReviewStore(projectDir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = store.Close() }()

	review, err := resolveReview(store, args[0])
	if err != nil {
		return err
	}

	if err := store.SetStatus(review.ID, ReviewStatusClosed); err != nil {
		return err
	}
	fmt.Printf("%s Closed review #%d (%s)\n", icons.CheckBold(), review.ID, review.SHA[:8])
	return nil
}

func runReviewList(_ *cobra.Command, _ []string) error {
	projectDir, _ := os.Getwd()
	store, err := OpenReviewStore(projectDir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = store.Close() }()

	reviews, err := store.ListAll(reviewListLimit)
	if err != nil {
		return err
	}
	if len(reviews) == 0 {
		fmt.Println("No reviews yet.")
		return nil
	}

	for _, r := range reviews {
		icon := statusIcon(r.Status)
		findings := ""
		if len(r.Findings) > 0 {
			findings = fmt.Sprintf(" %d findings [%s]", len(r.Findings), r.MaxSeverity)
		}
		fmt.Printf("%s #%-3d %s %s%s  %s\n", icon, r.ID, r.SHA[:8], r.Status, findings, r.CreatedAt.Format("Jan 02 15:04"))
	}
	return nil
}

// resolveReview finds a review by numeric ID or SHA prefix.
func resolveReview(store *ReviewStore, ref string) (*ReviewRecord, error) {
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		r, err := store.Get(id)
		if err != nil {
			return nil, fmt.Errorf("review #%d not found", id)
		}
		return r, nil
	}
	r, err := store.GetBySHA(ref)
	if err != nil {
		return nil, fmt.Errorf("no review found for %s", ref)
	}
	return r, nil
}

func printReviewDetail(r *ReviewRecord) {
	header := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)

	fmt.Printf("%s Review #%d — %s\n", statusIcon(r.Status), r.ID, r.SHA[:8])
	fmt.Printf("%s\n", dim.Render(fmt.Sprintf("Status: %s · Created: %s · Tokens: %d", r.Status, r.CreatedAt.Format("2006-01-02 15:04"), r.TokensUsed)))
	fmt.Println()

	if len(r.Findings) == 0 {
		fmt.Println(header.Render("No findings — clean commit " + icons.CheckBold()))
		return
	}

	fmt.Println(header.Render(fmt.Sprintf("%d Findings:", len(r.Findings))))
	fmt.Println()

	for i, f := range r.Findings {
		sev := severityStyle(f.Severity.String())
		fmt.Printf("  %d. %s %s:%d\n", i+1, sev, f.File, f.Line)
		fmt.Printf("     %s\n", f.Message)
		if f.Fix != "" {
			fmt.Printf("     %s %s\n", dim.Render("Fix:"), f.Fix)
		}
		fmt.Println()
	}
}

func statusIcon(s ReviewStatus) string {
	switch s {
	case ReviewStatusPassed:
		return icons.CheckBold() + " "
	case ReviewStatusOpen:
		return icons.Alert() + " "
	case ReviewStatusFixed:
		return icons.CheckBold() + " "
	case ReviewStatusClosed:
		return icons.CloseThick() + " "
	case ReviewStatusFailed:
		return icons.CloseThick() + " "
	case ReviewStatusRunning:
		return icons.RotateVariant() + " "
	default:
		return "."
	}
}

func severityStyle(sev string) string {
	switch strings.ToLower(sev) {
	case "critical":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render("[CRITICAL]")
	case "high":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true).Render("[HIGH]")
	case "medium":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("[MEDIUM]")
	case "low":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("[LOW]")
	default:
		return lipgloss.NewStyle().Faint(true).Render("[INFO]")
	}
}
