package migrate

import (
	"fmt"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/cobra"
)

func NewCmdMigrate(f *factory.Factory) *cobra.Command {
	var removeFromFile bool

	cmd := &cobra.Command{
		Use:   "migrate",
		Args:  cobra.NoArgs,
		Short: "Migrate API tokens from config file to secure keychain storage",
		Long: `Migrate API tokens from the plain text config file to secure keychain storage.

This command will:
  1. Detect any tokens stored in ~/.config/bk.yaml (or equivalent)
  2. Copy them to your system's secure credential storage
     - macOS: Keychain
     - Windows: Credential Manager
     - Linux: Secret Service (GNOME Keyring / KWallet)
  3. Optionally remove tokens from the config file (use --remove-from-file)

After migration, tokens will be read from the keychain by default, with the config
file serving as a fallback. You can verify keychain storage is being used by checking
that your config file no longer contains plain text tokens.

To force file-based storage instead, set: BUILDKITE_TOKEN_STORAGE=file`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return migrateRun(f, removeFromFile)
		},
	}

	cmd.Flags().BoolVar(&removeFromFile, "remove-from-file", false, "Remove tokens from config file after successful migration")

	return cmd
}

func migrateRun(f *factory.Factory, removeFromFile bool) error {
	conf := f.Config

	// Check if keychain storage is enabled
	if !config.ShouldUseKeychain() {
		return fmt.Errorf("keychain storage is not enabled\n\nKeychain storage is disabled via BUILDKITE_TOKEN_STORAGE environment variable.\nTo use keychain storage, either unset this variable or set it to 'keychain'.")
	}

	// Check if there are any tokens to migrate
	if !conf.HasFileTokens() {
		fmt.Println("✓ No tokens found in config file - nothing to migrate")
		fmt.Println("\nYour tokens are likely already in the keychain, or you haven't configured any organizations yet.")
		fmt.Println("Use 'bk configure add' to add an organization and token.")
		return nil
	}

	// Show what will be migrated
	fmt.Println("🔍 Checking for tokens in config file...")

	// Get all configured organizations (this is a best effort - we'll migrate what we can)
	orgs := conf.ConfiguredOrganizations()
	if len(orgs) > 0 {
		fmt.Printf("\nFound organization(s) in config:\n")
		for _, org := range orgs {
			fmt.Printf("  • %s\n", org)
		}
	}

	// Perform migration
	fmt.Println("\n🔐 Migrating tokens to keychain...")
	migrated, err := conf.MigrateTokensToKeychain()
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	if migrated == 0 {
		fmt.Println("⚠️  No tokens were migrated")
		return nil
	}

	fmt.Printf("✓ Successfully migrated %d token(s) to keychain\n", migrated)

	// Optionally remove from file
	if removeFromFile {
		fmt.Println("\n🗑️  Removing tokens from config file...")
		if err := conf.RemoveTokensFromFile(); err != nil {
			fmt.Printf("⚠️  Warning: Could not remove tokens from config file: %v\n", err)
			fmt.Println("You may need to manually edit ~/.config/bk.yaml")
		} else {
			fmt.Println("✓ Tokens removed from config file")
		}
	} else {
		fmt.Println("\n💡 Tip: Use --remove-from-file to clean up tokens from the config file")
		fmt.Println("   Tokens will remain in the file as a backup, but the keychain will be used first.")
	}

	fmt.Println("\n✅ Migration complete! Your tokens are now stored securely in the system keychain.")

	return nil
}
