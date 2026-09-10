package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	siteAuditDepth       int
	siteAuditChecks      string
	siteAuditFailOn      string
	siteAuditConcurrency int
)

// siteAuditCmd runs an merlin site audit against a target URL and reports the
// findings. It wires the previously-orphaned RunMerlinPipeline into the CLI.
// Named "site-audit" to avoid colliding with the existing "audit" command
// (session-pattern analysis).
var siteAuditCmd = &cobra.Command{
	Use:   "site-audit <target>",
	Short: "Run a site audit with merlin",
	Long:  "Crawl a target URL and run security/quality checks (merlin engine), reporting findings as review findings.",
	Args:  cobra.ExactArgs(1),
	RunE:  runSiteAudit,
}

func init() {
	siteAuditCmd.Flags().IntVar(&siteAuditDepth, "depth", 0, "Crawl depth (0 = default)")
	siteAuditCmd.Flags().StringVar(&siteAuditChecks, "checks", "", "Comma-separated checks to run")
	siteAuditCmd.Flags().StringVar(&siteAuditFailOn, "fail-on", "", "Fail severity threshold (low|medium|high|critical)")
	siteAuditCmd.Flags().IntVar(&siteAuditConcurrency, "concurrency", 0, "Crawl concurrency (0 = default)")
	rootCmd.AddCommand(siteAuditCmd)
}

func runSiteAudit(_ *cobra.Command, args []string) error {
	target := args[0]
	cfg := MerlinPipelineConfig{
		Target:      target,
		Depth:       siteAuditDepth,
		FailOn:      siteAuditFailOn,
		Concurrency: siteAuditConcurrency,
	}
	if strings.TrimSpace(siteAuditChecks) != "" {
		for _, c := range strings.Split(siteAuditChecks, ",") {
			if c = strings.TrimSpace(c); c != "" {
				cfg.Checks = append(cfg.Checks, c)
			}
		}
	}

	findings, reportStr, err := RunMerlinPipeline(context.Background(), cfg)
	if err != nil {
		return err
	}

	fmt.Println(reportStr)
	for _, f := range findings {
		loc := f.File
		if f.Line > 0 {
			loc = loc + ":" + itoaCLI(f.Line)
		}
		fmt.Printf("  [%s] %s %s: %s\n", f.Severity, loc, f.Concern, f.Message)
		if f.Fix != "" {
			fmt.Printf("      fix: %s\n", f.Fix)
		}
	}
	return nil
}

func itoaCLI(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
