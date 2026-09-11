package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/executiongraph"
	cloud "github.com/GrayCodeAI/hawk/internal/platform/cloud"
	"github.com/spf13/cobra"
)

func newCloudGraphCmd() *cobra.Command {
	graphCmd := &cobra.Command{
		Use:   "graph",
		Short: "Manage explicit portable graph synchronization",
		Args:  cobra.NoArgs,
	}

	var repositoryID string
	var swiftCheckpointIDs []string
	var missionDir string
	syncCmd := &cobra.Command{
		Use:   "sync [session-id]",
		Short: "Upload a privacy-normalized execution graph to Hawk Cloud",
		Long: `Build the same read-only execution graph as "hawk graph export", hash
cloud-sensitive metadata, enforce Hawk Cloud's upload bounds, and upload it
with a deterministic idempotency key. Sync completed session snapshots: graph
facts are immutable after acceptance. This is explicit and opt-in; local
execution never depends on cloud synchronization.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prog := NewCLIProgress("Graph sync", []string{"Building execution graph", "Uploading to Hawk Cloud"})
			defer prog.Abort()
			prog.StartStep(0)
			var export executiongraph.Export
			var err error
			if strings.TrimSpace(missionDir) != "" {
				if len(args) != 0 {
					return fmt.Errorf("session-id cannot be used with --mission-dir")
				}
				export, err = loadMissionGraphExport(missionDir)
			} else {
				export, err = buildExecutionGraphExport(
					args,
					repositoryID,
					swiftCheckpointIDs,
					time.Time{},
				)
			}
			if err != nil {
				return err
			}
			client, cfg, err := cloud.LoadClient()
			if err != nil || !client.Enabled() {
				return fmt.Errorf("hawk cloud is not connected")
			}
			prepared, err := cloud.PrepareGraph(export)
			if err != nil {
				return fmt.Errorf("prepare graph for Hawk Cloud: %w", err)
			}
			prog.CompleteStep(0)
			prog.StartStep(1)
			result, err := client.SyncGraph(cmd.Context(), cloud.GraphSyncRequest{
				SyncID:    prepared.SyncID,
				ProjectID: cfg.ProjectID,
				Graph:     prepared.Graph,
			})
			if err != nil {
				return err
			}
			prog.CompleteStep(1)
			prog.Done()
			status := "accepted"
			if result.Duplicate {
				status = "already synchronized"
			}
			cmd.Printf("%s\n",
				auditTint("Graph "+status+": ", doneGreen)+
					auditTint(fmt.Sprintf("%d facts", prepared.Facts), textPrimary)+
					auditTint(fmt.Sprintf(" (digest %s).", result.GraphDigest), textMuted))
			return nil
		},
	}
	syncCmd.Flags().StringVar(
		&repositoryID,
		"repository",
		"",
		"Repository scope override (defaults to the saved session directory name)",
	)
	syncCmd.Flags().StringArrayVar(
		&swiftCheckpointIDs,
		"swift-checkpoint",
		nil,
		"Link a Swift checkpoint ID; may be repeated",
	)
	syncCmd.Flags().StringVar(
		&missionDir,
		"mission-dir",
		"",
		"Sync mission-graph.json from this mission directory instead of a session",
	)
	graphCmd.AddCommand(syncCmd)
	return graphCmd
}

func init() {
	cloudCmd.AddCommand(newCloudGraphCmd())
}
