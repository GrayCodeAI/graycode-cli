package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	feedbackLocal    bool
	feedbackCategory string
)

// FeedbackReport is the structured report written to file or used in issue URL.
type FeedbackReport struct {
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
	Model     string `json:"model"`
	Provider  string `json:"provider"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Category  string `json:"category"`
	Body      string `json:"body"`
	SessionID string `json:"session_id,omitempty"`
}

var feedbackCmd = &cobra.Command{
	Use:   "feedback [message]",
	Short: "Submit feedback about hawk",
	Long: `Capture feedback about your hawk experience. By default, opens a
GitHub issue template URL in your browser. Use --local to save feedback
to ~/.hawk/feedback/ for later submission.

Categories: bug, feature, ux, performance, other

Examples:
  hawk feedback "The completion is slow"
  hawk feedback --category bug "Crash when using /compact"
  hawk feedback --local "Wish it could do X"`,
	Args: cobra.MinimumNArgs(0),
	RunE: runFeedback,
}

func init() {
	feedbackCmd.Flags().BoolVar(&feedbackLocal, "local", false, "save feedback locally instead of opening browser")
	feedbackCmd.Flags().StringVar(&feedbackCategory, "category", "other", "feedback category: bug, feature, ux, performance, other")
	rootCmd.AddCommand(feedbackCmd)
}

func runFeedback(_ *cobra.Command, args []string) error {
	body := strings.Join(args, " ")
	if body == "" {
		fmt.Println("Enter your feedback (press Ctrl+D when done):")
		data, err := readFeedbackStdin()
		if err != nil {
			return err
		}
		body = string(data)
	}

	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("feedback body cannot be empty")
	}

	report := FeedbackReport{
		Timestamp: time.Now().Format(time.RFC3339),
		Version:   version,
		Model:     model,
		Provider:  provider,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Category:  feedbackCategory,
		Body:      body,
	}

	if feedbackLocal {
		return saveFeedbackLocal(report)
	}
	return openFeedbackIssue(report)
}

func saveFeedbackLocal(report FeedbackReport) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	dir := filepath.Join(home, ".hawk", "feedback")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create feedback directory: %w", err)
	}

	filename := fmt.Sprintf("feedback-%s.json", time.Now().Format("20060102-150405"))
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write feedback: %w", err)
	}

	fmt.Printf("Feedback saved to %s\n", path)
	return nil
}

func openFeedbackIssue(report FeedbackReport) error {
	title := fmt.Sprintf("[%s] %s", report.Category, truncateFeedback(report.Body, 60))

	var bodyBuilder strings.Builder
	bodyBuilder.WriteString("## Feedback\n\n")
	bodyBuilder.WriteString(report.Body)
	bodyBuilder.WriteString("\n\n## Environment\n\n")
	bodyBuilder.WriteString(fmt.Sprintf("- **Version:** %s\n", report.Version))
	bodyBuilder.WriteString(fmt.Sprintf("- **OS:** %s/%s\n", report.OS, report.Arch))
	if report.Model != "" {
		bodyBuilder.WriteString(fmt.Sprintf("- **Model:** %s\n", report.Model))
	}
	if report.Provider != "" {
		bodyBuilder.WriteString(fmt.Sprintf("- **Provider:** %s\n", report.Provider))
	}
	bodyBuilder.WriteString(fmt.Sprintf("- **Category:** %s\n", report.Category))
	bodyBuilder.WriteString(fmt.Sprintf("- **Timestamp:** %s\n", report.Timestamp))

	issueURL := fmt.Sprintf(
		"https://github.com/GrayCodeAI/hawk/issues/new?title=%s&body=%s&labels=%s",
		url.QueryEscape(title),
		url.QueryEscape(bodyBuilder.String()),
		url.QueryEscape(report.Category),
	)

	if err := openBrowser(issueURL); err != nil {
		// Fallback: print the URL.
		fmt.Printf("Could not open browser. Please visit:\n%s\n", issueURL)
		return nil
	}

	fmt.Println("Opened feedback issue in your browser.")
	return nil
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}

func truncateFeedback(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func readFeedbackStdin() ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}
