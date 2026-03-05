package configure

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/spf13/afero"
)

func TestGetTokenForOrg(t *testing.T) {
	t.Run("returns empty string when no token exists", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		f := &factory.Factory{Config: conf}

		token := getTokenForOrg(f, "nonexistent")
		if token != "" {
			t.Errorf("expected empty string, got %s", token)
		}
	})

	t.Run("returns token when it exists in keychain", func(t *testing.T) {
		keyring.MockForTesting()

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		f := &factory.Factory{Config: conf}

		kr := keyring.New()
		kr.Set("test-org", "bk_test_token_12345")

		token := getTokenForOrg(f, "test-org")
		if token != "bk_test_token_12345" {
			t.Errorf("expected bk_test_token_12345, got %s", token)
		}
	})

	t.Run("returns different tokens for different organizations", func(t *testing.T) {
		keyring.MockForTesting()

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		f := &factory.Factory{Config: conf}

		kr := keyring.New()
		kr.Set("org1", "bk_test_token_org1")
		kr.Set("org2", "bk_test_token_org2")

		if getTokenForOrg(f, "org1") != "bk_test_token_org1" {
			t.Errorf("expected bk_test_token_org1 for org1")
		}
		if getTokenForOrg(f, "org2") != "bk_test_token_org2" {
			t.Errorf("expected bk_test_token_org2 for org2")
		}
	})
}

func TestConfigureWithCredentials(t *testing.T) {
	t.Run("configures organization and token", func(t *testing.T) {
		keyring.MockForTesting()

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		f := &factory.Factory{Config: conf}

		org := "test-org"
		token := "bk_test_token_12345"

		err := ConfigureWithCredentials(f, org, token)
		if err != nil {
			t.Errorf("expected no error, got %s", err)
		}

		if conf.OrganizationSlug() != org {
			t.Errorf("expected organization to be %s, got %s", org, conf.OrganizationSlug())
		}

		kr := keyring.New()
		got, _ := kr.Get(org)
		if got != token {
			t.Errorf("expected token to be %s, got %s", token, got)
		}
	})
}

func TestConfigureTokenReuse(t *testing.T) {
	t.Run("reuses existing token when available", func(t *testing.T) {
		keyring.MockForTesting()

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		f := &factory.Factory{Config: conf}

		org := "test-org"
		existingToken := "bk_existing_token_12345"

		// Pre-configure a token in the keychain
		kr := keyring.New()
		kr.Set(org, existingToken)

		// Verify the token can be retrieved
		retrievedToken := getTokenForOrg(f, org)
		if retrievedToken != existingToken {
			t.Errorf("expected to retrieve existing token %s, got %s", existingToken, retrievedToken)
		}

		// Configure with the existing token
		err := ConfigureWithCredentials(f, org, retrievedToken)
		if err != nil {
			t.Errorf("expected no error, got %s", err)
		}

		if conf.OrganizationSlug() != org {
			t.Errorf("expected organization to be %s, got %s", org, conf.OrganizationSlug())
		}

		got, _ := kr.Get(org)
		if got != existingToken {
			t.Errorf("expected token to be %s, got %s", existingToken, got)
		}
	})
}
