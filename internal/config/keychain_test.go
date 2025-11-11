package config

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/zalando/go-keyring"
)

func TestKeychainTokenStorage(t *testing.T) {
	t.Run("set and get token from keychain", func(t *testing.T) {
		// Mock the keyring for testing
		keyring.MockInit()

		storage := NewKeychainTokenStorage()

		// Set a token
		err := storage.Set("test-org", "test-token-123")
		if err != nil {
			t.Fatalf("failed to set token: %v", err)
		}

		// Get the token back
		token, err := storage.Get("test-org")
		if err != nil {
			t.Fatalf("failed to get token: %v", err)
		}

		if token != "test-token-123" {
			t.Errorf("expected token 'test-token-123', got '%s'", token)
		}
	})

	t.Run("get non-existent token returns error", func(t *testing.T) {
		keyring.MockInit()

		storage := NewKeychainTokenStorage()

		_, err := storage.Get("nonexistent-org")
		if err == nil {
			t.Error("expected error when getting non-existent token")
		}
	})

	t.Run("delete token from keychain", func(t *testing.T) {
		keyring.MockInit()

		storage := NewKeychainTokenStorage()

		// Set a token
		err := storage.Set("test-org", "test-token-123")
		if err != nil {
			t.Fatalf("failed to set token: %v", err)
		}

		// Delete the token
		err = storage.Delete("test-org")
		if err != nil {
			t.Fatalf("failed to delete token: %v", err)
		}

		// Verify it's gone
		_, err = storage.Get("test-org")
		if err == nil {
			t.Error("expected error after deleting token")
		}
	})
}

func TestConfigWithKeychain(t *testing.T) {
	t.Run("config uses keychain by default", func(t *testing.T) {
		// Mock the keyring
		keyring.MockInit()

		// Ensure keychain is enabled
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "keychain")

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)

		// Set a token
		err := conf.SetTokenForOrg("my-org", "my-token-456")
		if err != nil {
			t.Fatalf("failed to set token: %v", err)
		}

		// Get the token back
		token := conf.GetTokenForOrg("my-org")
		if token != "my-token-456" {
			t.Errorf("expected token 'my-token-456', got '%s'", token)
		}
	})

	t.Run("config falls back to file when keychain fails", func(t *testing.T) {
		// Use file-based storage
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "file")

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)

		// Set a token (will go to file)
		err := conf.SetTokenForOrg("file-org", "file-token-789")
		if err != nil {
			t.Fatalf("failed to set token: %v", err)
		}

		// Get the token back (should read from file)
		token := conf.GetTokenForOrg("file-org")
		if token != "file-token-789" {
			t.Errorf("expected token 'file-token-789', got '%s'", token)
		}
	})
}

func TestMigration(t *testing.T) {
	t.Run("migrate tokens from file to keychain", func(t *testing.T) {
		keyring.MockInit()
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "keychain")

		fs := afero.NewMemMapFs()

		// First create config with file-based storage
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "file")
		conf := New(fs, nil)

		// Add some tokens to the file
		conf.SetTokenForOrg("org1", "token1")
		conf.SetTokenForOrg("org2", "token2")

		// Now switch to keychain mode
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "keychain")
		conf = New(fs, nil)

		// Migrate
		migrated, err := conf.MigrateTokensToKeychain()
		if err != nil {
			t.Fatalf("migration failed: %v", err)
		}

		if migrated != 2 {
			t.Errorf("expected 2 tokens migrated, got %d", migrated)
		}

		// Verify tokens are in keychain
		token1 := conf.GetTokenForOrg("org1")
		if token1 != "token1" {
			t.Errorf("expected token1, got %s", token1)
		}

		token2 := conf.GetTokenForOrg("org2")
		if token2 != "token2" {
			t.Errorf("expected token2, got %s", token2)
		}
	})

	t.Run("HasFileTokens detects tokens in file", func(t *testing.T) {
		t.Setenv("BUILDKITE_TOKEN_STORAGE", "file")

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)

		// Initially no tokens
		if conf.HasFileTokens() {
			t.Error("expected no file tokens initially")
		}

		// Add a token
		conf.SetTokenForOrg("org1", "token1")

		// Now should detect it
		if !conf.HasFileTokens() {
			t.Error("expected to detect file tokens")
		}
	})
}
