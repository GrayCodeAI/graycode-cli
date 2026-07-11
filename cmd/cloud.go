package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	cloud "github.com/GrayCodeAI/hawk/internal/platform/cloud"
	"github.com/spf13/cobra"
)

var cloudCmd = &cobra.Command{Use: "cloud", Short: "Manage optional Hawk Cloud synchronization"}

var cloudConnectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect this Hawk device to Hawk Cloud",
	RunE: func(cmd *cobra.Command, _ []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint")
		deviceID, _ := cmd.Flags().GetString("device-id")
		projectID, _ := cmd.Flags().GetString("project-id")
		token, _ := cmd.Flags().GetString("token")
		if endpoint == "" || deviceID == "" || projectID == "" || token == "" {
			return fmt.Errorf("endpoint, device-id, project-id, and token are required")
		}
		if err := cloud.SaveDeviceConfig(cloud.DeviceConfig{Endpoint: endpoint, DeviceID: deviceID, ProjectID: projectID}, token); err != nil {
			return err
		}
		cmd.Println("Hawk Cloud connected. Usage synchronization is opt-in and fail-open.")
		return nil
	},
}

var cloudLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Sign in to Hawk Cloud in a browser",
	RunE: func(cmd *cobra.Command, _ []string) error {
		endpoint, _ := cmd.Flags().GetString("endpoint")
		label, _ := cmd.Flags().GetString("label")
		if endpoint == "" {
			endpoint = os.Getenv("HAWK_CLOUD_URL")
		}
		if endpoint == "" {
			return fmt.Errorf("hawk cloud endpoint is required (use --endpoint or HAWK_CLOUD_URL)")
		}
		if label == "" {
			label, _ = os.Hostname()
		}
		client := cloud.New(cloud.Config{Endpoint: endpoint})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		start, err := client.StartDeviceLogin(ctx, label, runtime.GOOS, version)
		if err != nil {
			return err
		}
		cmd.Printf("Open %s and enter code %s\n", start.VerificationURI, start.UserCode)
		if err := openBrowser(start.VerificationURI + "?code=" + start.UserCode); err != nil {
			cmd.Printf("Could not open the browser automatically: %v\n", err)
		}
		interval := time.Duration(start.Interval) * time.Second
		if interval < time.Second {
			interval = 5 * time.Second
		}
		for {
			poll, pollErr := client.PollDeviceLogin(ctx, start.DeviceCode)
			if pollErr != nil {
				return pollErr
			}
			switch poll.Status {
			case "pending":
				select {
				case <-ctx.Done():
					return fmt.Errorf("waiting for browser approval: %w", ctx.Err())
				case <-time.After(interval):
				}
			case "approved":
				if poll.Token == "" || poll.DeviceID == "" || poll.ProjectID == "" {
					return fmt.Errorf("hawk cloud returned an incomplete device authorization")
				}
				if err := cloud.SaveDeviceConfig(cloud.DeviceConfig{Endpoint: endpoint, DeviceID: poll.DeviceID, ProjectID: poll.ProjectID}, poll.Token); err != nil {
					return err
				}
				cmd.Printf("Hawk Cloud connected for project %s.\n", poll.ProjectID)
				return nil
			case "expired":
				return fmt.Errorf("hawk cloud device authorization expired")
			default:
				return fmt.Errorf("hawk cloud returned unknown device authorization status %q", poll.Status)
			}
		}
	},
}

var cloudStatusCmd = &cobra.Command{
	Use: "status", Short: "Show Hawk Cloud connection status",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, cfg, err := cloud.LoadClient()
		if err != nil || !client.Enabled() {
			cmd.Println("Hawk Cloud is not connected.")
			return nil
		}
		cmd.Printf("Hawk Cloud connected: %s (device %s, project %s)\n", cfg.Endpoint, cfg.DeviceID, cfg.ProjectID)
		return nil
	},
}

var cloudContextCmd = &cobra.Command{
	Use:   "context",
	Short: "Sync repository context to Hawk Cloud",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, cfg, err := cloud.LoadClient()
		if err != nil || !client.Enabled() {
			return fmt.Errorf("hawk cloud is not connected")
		}
		detected, detectErr := detectGitContext(cmd.Context())
		repository, _ := cmd.Flags().GetString("repository")
		if repository == "" {
			repository = detected.Repository
		}
		if repository == "" {
			return detectErr
		}
		contextProvider, _ := cmd.Flags().GetString("provider")
		externalID, _ := cmd.Flags().GetString("external-id")
		branch, _ := cmd.Flags().GetString("branch")
		commit, _ := cmd.Flags().GetString("commit")
		ciRunID, _ := cmd.Flags().GetString("ci-run")
		ciStatus, _ := cmd.Flags().GetString("ci-status")
		ciWorkflow, _ := cmd.Flags().GetString("ci-workflow")
		deploymentID, _ := cmd.Flags().GetString("deployment")
		deploymentStatus, _ := cmd.Flags().GetString("deployment-status")
		deploymentEnvironment, _ := cmd.Flags().GetString("deployment-environment")
		if contextProvider == "" {
			contextProvider = detected.Provider
		}
		if contextProvider == "" {
			contextProvider = "git"
		}
		if branch == "" {
			branch = detected.Branch
		}
		if commit == "" {
			commit = detected.Commit
		}
		if externalID == "" {
			externalID = repository
		}
		event := cloud.DeliveryContext{ProjectID: cfg.ProjectID, Branch: branch, CommitSHA: commit}
		event.Repository.Provider, event.Repository.ExternalID, event.Repository.Name = contextProvider, externalID, repository
		if ciRunID == "" {
			ciRunID, ciWorkflow = os.Getenv("GITHUB_RUN_ID"), firstValue(ciWorkflow, os.Getenv("GITHUB_WORKFLOW"))
		}
		if ciRunID != "" {
			if ciStatus == "" {
				ciStatus = "running"
			}
			ciProvider := contextProvider
			if os.Getenv("GITHUB_RUN_ID") != "" && ciProvider == "git" {
				ciProvider = "github"
			}
			event.CIRun = &cloud.CIRunContext{Provider: ciProvider, ExternalID: ciRunID, Workflow: ciWorkflow, Status: ciStatus}
		}
		if deploymentID != "" {
			if deploymentStatus == "" {
				deploymentStatus = "running"
			}
			if deploymentEnvironment == "" {
				return fmt.Errorf("deployment-environment is required with --deployment")
			}
			event.Deployment = &cloud.DeploymentContext{Provider: contextProvider, ExternalID: deploymentID, Environment: deploymentEnvironment, Status: deploymentStatus}
		}
		client.RecordDeliveryContext(cmd.Context(), event)
		cmd.Println("Repository context queued for Hawk Cloud.")
		return nil
	},
}

func firstValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func init() {
	cloudLoginCmd.Flags().String("endpoint", "", "Hawk Cloud endpoint (or HAWK_CLOUD_URL)")
	cloudLoginCmd.Flags().String("label", "", "Name for this Hawk device")
	cloudConnectCmd.Flags().String("endpoint", "", "Hawk Cloud endpoint")
	cloudConnectCmd.Flags().String("device-id", "", "Hawk Cloud device ID")
	cloudConnectCmd.Flags().String("project-id", "", "Hawk Cloud project ID")
	cloudConnectCmd.Flags().String("token", "", "Hawk Cloud device token")
	cloudContextCmd.Flags().String("repository", "", "Repository name (auto-detected from Git when omitted)")
	cloudContextCmd.Flags().String("provider", "", "Repository provider (auto-detected when omitted)")
	cloudContextCmd.Flags().String("external-id", "", "Provider repository identifier (defaults to repository)")
	cloudContextCmd.Flags().String("branch", "", "Current branch (auto-detected when omitted)")
	cloudContextCmd.Flags().String("commit", "", "Current commit SHA (auto-detected when omitted)")
	cloudContextCmd.Flags().String("ci-run", "", "CI run identifier (defaults to GITHUB_RUN_ID)")
	cloudContextCmd.Flags().String("ci-status", "", "CI status: queued, running, succeeded, failed, or cancelled")
	cloudContextCmd.Flags().String("ci-workflow", "", "CI workflow name (defaults to GITHUB_WORKFLOW)")
	cloudContextCmd.Flags().String("deployment", "", "Deployment identifier")
	cloudContextCmd.Flags().String("deployment-status", "", "Deployment status")
	cloudContextCmd.Flags().String("deployment-environment", "", "Deployment environment, for example production")
	cloudCmd.AddCommand(cloudLoginCmd, cloudConnectCmd, cloudStatusCmd, cloudContextCmd)
	rootCmd.AddCommand(cloudCmd)
}
