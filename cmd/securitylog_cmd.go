package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/GrayCodeAI/hawk/internal/securitylog"
	"github.com/spf13/cobra"
)

var (
	securitylogLimit int
	securitylogJSON  bool
)

// securitylogCmd inspects the tamper-evident security event log.
var securitylogCmd = &cobra.Command{
	Use:   "securitylog",
	Short: "Inspect the tamper-evident security event log",
	Long: `Hawk records security-relevant events (permission denials, approval
denials) to an append-only, HMAC-chained log. Entries are linked so that
reordering, deletion, or alteration is detectable.

  hawk securitylog            Show a summary and recent events
  hawk securitylog show       List logged events
  hawk securitylog verify     Verify the hash chain has not been tampered with`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSecuritylogShow(cmd, 20, false)
	},
}

var securitylogShowCmd = &cobra.Command{
	Use:   "show",
	Short: "List logged security events",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSecuritylogShow(cmd, securitylogLimit, securitylogJSON)
	},
}

var securitylogVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify the event log hash chain is intact",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := securitylog.DefaultDir()
		count, err := securitylog.Verify(dir)
		if err != nil {
			return fmt.Errorf("security log verification FAILED: %w", err)
		}
		cmd.Printf("security event log OK: %d entries verified (%s)\n", count, dir)
		return nil
	},
}

func init() {
	securitylogShowCmd.Flags().IntVar(&securitylogLimit, "limit", 50, "max events to print (0 = all)")
	securitylogShowCmd.Flags().BoolVar(&securitylogJSON, "json", false, "output events as JSON")
	securitylogCmd.AddCommand(securitylogShowCmd)
	securitylogCmd.AddCommand(securitylogVerifyCmd)
	rootCmd.AddCommand(securitylogCmd)
}

func runSecuritylogShow(cmd *cobra.Command, limit int, asJSON bool) error {
	dir := securitylog.DefaultDir()
	events, err := securitylog.Entries(dir)
	if err != nil {
		return fmt.Errorf("reading security event log: %w", err)
	}

	if asJSON {
		if limit > 0 && len(events) > limit {
			events = events[len(events)-limit:]
		}
		if events == nil {
			events = []securitylog.Event{}
		}
		data, err := json.MarshalIndent(events, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling events: %w", err)
		}
		cmd.Println(string(data))
		return nil
	}

	if len(events) == 0 {
		cmd.Println("No security events recorded yet.")
		cmd.Printf("Log location: %s\n", dir)
		return nil
	}

	start := 0
	if limit > 0 && len(events) > limit {
		start = len(events) - limit
	}
	cmd.Printf("Security event log: %d event(s) at %s\n", len(events), dir)
	if start > 0 {
		cmd.Printf("Showing the most recent %d:\n", len(events)-start)
	}
	for _, ev := range events[start:] {
		cmd.Printf(
			"%s  %-8s  %-20s  %s\n",
			ev.Timestamp.Format(time.RFC3339),
			ev.Severity,
			ev.Type,
			truncateWithEllipsis(ev.Detail, 60),
		)
	}
	return nil
}
