package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/spf13/cobra"
)

// sessionShareCmd shares a session. It uploads the export to a configured
// hosted endpoint (HAWK_SHARE_URL) when one is set, else falls back to the
// local content-derived deeplink + export path.
var sessionShareCmd = &cobra.Command{
	Use:   "share [session-id]",
	Short: "Share a session (hosted URL or local deeplink)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSessionShare,
}

func runSessionShare(_ *cobra.Command, args []string) error {
	var s *session.Session
	var err error
	if len(args) > 0 {
		s, err = session.Load(args[0])
	} else {
		s, err = session.LoadLatest()
	}
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	data, err := session.Export(s, "json", true)
	if err != nil {
		return fmt.Errorf("export session: %w", err)
	}

	host := strings.TrimSpace(os.Getenv("HAWK_SHARE_URL"))
	if host != "" {
		url, err := uploadShare(context.Background(), host, s.ID, data)
		if err != nil {
			return fmt.Errorf("upload share: %w", err)
		}
		fmt.Printf("Shared: %s\n", url)
		return nil
	}

	// Fallback: local deeplink + export path.
	link := session.ShareLinkForID(s.ID)
	if link == "" {
		return fmt.Errorf("could not generate a share link for session %q", s.ID)
	}
	fmt.Printf("Share deeplink: %s\n", link)
	fmt.Printf("(Set HAWK_SHARE_URL to a Hawk Cloud share endpoint for a hosted URL.)\n")
	return nil
}

// uploadShare POSTs the export to the hosted endpoint and returns the share URL.
func uploadShare(ctx context.Context, host, id string, data []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(host, "/")+"/v1/shares", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("share endpoint returned %s", resp.Status)
	}
	return strings.TrimRight(host, "/") + "/share/" + id, nil
}

func init() {
	rootCmd.AddCommand(sessionShareCmd)
}
