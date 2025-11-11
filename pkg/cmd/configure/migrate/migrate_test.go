package migrate

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/afero"
	"github.com/zalando/go-keyring"
)

func TestMigrateCommand(t *testing.T) {
	t.Run("migrates tokens from file to keychain", func(t *testing.T) {
		// Initialize mock keyring
		keyring.MockInit()

		// Start with file-based storage
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "file")

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		// Add some tokens to the file
		conf.SetTokenForOrg("org1", "token1")
		conf.SetTokenForOrg("org2", "token2")

		// Verify tokens are in file
		if !conf.HasFileTokens() {
			t.Fatal("expected tokens to be in file")
		}

		// Now switch to keychain mode
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "keychain")
		conf = config.New(fs, nil)

		f := &factory.Factory{Config: conf}

		// Run migration
		err := migrateRun(f, false)
		if err != nil {
			t.Fatalf("migration failed: %v", err)
		}

		// Verify tokens are accessible (should be in keychain now)
		token1 := conf.GetTokenForOrg("org1")
		if token1 != "token1" {
			t.Errorf("expected token1, got %s", token1)
		}

		token2 := conf.GetTokenForOrg("org2")
		if token2 != "token2" {
			t.Errorf("expected token2, got %s", token2)
		}
	})

	t.Run("reports when no tokens to migrate", func(t *testing.T) {
		keyring.MockInit()
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "keychain")

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		f := &factory.Factory{Config: conf}

		// Should not error when no tokens to migrate
		err := migrateRun(f, false)
		if err != nil {
			t.Errorf("expected no error when no tokens to migrate, got: %v", err)
		}
	})

	t.Run("fails when keychain storage is disabled", func(t *testing.T) {
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "file")

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		f := &factory.Factory{Config: conf}

		// Should error when trying to migrate with keychain disabled
		err := migrateRun(f, false)
		if err == nil {
			t.Error("expected error when keychain storage is disabled")
		}
	})

	t.Run("removes tokens from file when flag is set", func(t *testing.T) {
		keyring.MockInit()

		// Start with file-based storage
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "file")

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		// Add a token to the file
		conf.SetTokenForOrg("org1", "token1")

		// Switch to keychain mode
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "keychain")
		conf = config.New(fs, nil)

		f := &factory.Factory{Config: conf}

		// Run migration with remove flag
		err := migrateRun(f, true)
		if err != nil {
			t.Fatalf("migration failed: %v", err)
		}

		// Verify tokens are removed from file
		// We need to check with file storage mode
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "file")
		confFile := config.New(fs, nil)
		if confFile.HasFileTokens() {
			t.Error("expected tokens to be removed from file")
		}
	})
}
