package cmd

import (
	"context"
	"fmt"
	"strings"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/spf13/cobra"
)

var credentialsCmd = &cobra.Command{
	Use:   "credentials",
	Short: "Manage secure API key storage (macOS Keychain / Linux secret service)",
}

var credentialsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show where API keys are stored",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cmd.Println(graycodeconfig.FormatCredentialCLIStatus(ctx))
		return nil
	},
}

var credentialsRemoveCmd = &cobra.Command{
	Use:   "remove <provider|env-var>",
	Short: "Remove a stored API key from the OS secret store",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		removed, err := graycodeconfig.RemoveStoredCredential(ctx, args[0])
		if err != nil {
			return err
		}
		cmd.Printf("%s\n", auditTint(fmt.Sprintf("Removed %d key(s) from %s: %s", len(removed), graycodeconfig.CredentialStoreName(), strings.Join(removed, ", ")), doneGreen))
		return nil
	},
}

var credentialsMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Import plaintext credential files into the OS secret store",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		storage := graycodeconfig.CredentialStorageStatus(ctx)
		if !storage.Writable {
			return fmt.Errorf("cannot migrate: %s", storage.Detail)
		}
		n, err := graycodeconfig.MigrateEnvFileCredentials(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			cmd.Println(auditTint("No plaintext credential files found (already using secure storage).", textMuted))
		} else {
			cmd.Printf("%s\n", auditTint(fmt.Sprintf("Migrated %d key(s) to %s and removed plaintext credential files.", n, graycodeconfig.CredentialStoreName()), doneGreen))
		}
		return nil
	},
}

func init() {
	credentialsCmd.AddCommand(credentialsStatusCmd)
	credentialsCmd.AddCommand(credentialsMigrateCmd)
	credentialsCmd.AddCommand(credentialsRemoveCmd)
}
