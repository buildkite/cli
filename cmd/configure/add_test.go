package configure

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/spf13/afero"
)

func TestGetTokenForOrg(t *testing.T) {
	t.Parallel()

	t.Run("returns empty string when no token exists", func(t *testing.T) {
		t.Parallel()
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		f := &factory.Factory{Config: conf}

		token := getTokenForOrg(f, "nonexistent")
		if token != "" {
			t.Errorf("expected empty string, got %s", token)
		}
	})

	t.Run("returns token when it exists for organization", func(t *testing.T) {
		t.Parallel()
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		f := &factory.Factory{Config: conf}

		// Set up a token for an organization
		expectedToken := "bk_test_token_12345"
		conf.SetTokenForOrg("test-org", expectedToken)

		token := getTokenForOrg(f, "test-org")
		if token != expectedToken {
			t.Errorf("expected %s, got %s", expectedToken, token)
		}
	})

	t.Run("returns different tokens for different organizations", func(t *testing.T) {
		t.Parallel()
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		f := &factory.Factory{Config: conf}

		// Set up tokens for different organizations
		token1 := "bk_test_token_org1"
		token2 := "bk_test_token_org2"
		conf.SetTokenForOrg("org1", token1)
		conf.SetTokenForOrg("org2", token2)

		if getTokenForOrg(f, "org1") != token1 {
			t.Errorf("expected %s for org1", token1)
		}
		if getTokenForOrg(f, "org2") != token2 {
			t.Errorf("expected %s for org2", token2)
		}
	})
}

func TestConfigureWithCredentials(t *testing.T) {
	t.Parallel()

	t.Run("configures organization and token", func(t *testing.T) {
		t.Parallel()
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

		if conf.GetTokenForOrg(org) != token {
			t.Errorf("expected token to be %s, got %s", token, conf.GetTokenForOrg(org))
		}
	})
}

func TestConfigureTokenReuse(t *testing.T) {
	t.Parallel()

	t.Run("reuses existing token when available", func(t *testing.T) {
		t.Parallel()
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		f := &factory.Factory{Config: conf}

		org := "test-org"
		existingToken := "bk_existing_token_12345"

		// Pre-configure a token for the organization
		conf.SetTokenForOrg(org, existingToken)

		// Verify the token can be retrieved
		retrievedToken := getTokenForOrg(f, org)
		if retrievedToken != existingToken {
			t.Errorf("expected to retrieve existing token %s, got %s", existingToken, retrievedToken)
		}

		// Configure with the existing token (simulating the logic in ConfigureRun)
		err := ConfigureWithCredentials(f, org, retrievedToken)
		if err != nil {
			t.Errorf("expected no error, got %s", err)
		}

		// Verify the configuration still works
		if conf.OrganizationSlug() != org {
			t.Errorf("expected organization to be %s, got %s", org, conf.OrganizationSlug())
		}

		if conf.GetTokenForOrg(org) != existingToken {
			t.Errorf("expected token to be %s, got %s", existingToken, conf.GetTokenForOrg(org))
		}
	})
}

func TestConfigureRequiresGitRepository(t *testing.T) {
	t.Parallel()

	t.Run("fails when not in a git repository", func(t *testing.T) {
		t.Parallel()
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		// Create a factory with nil GitRepository (simulating not being in a git repo)
		f := &factory.Factory{Config: conf, GitRepository: nil}

		err := ConfigureRun(f)

		if err == nil {
			t.Error("expected error when not in a git repository, got nil")
		}

		expectedErr := "not in a Git repository - bk should be configured at the root of a Git repository"
		if err.Error() != expectedErr {
			t.Errorf("expected error message %q, got %q", expectedErr, err.Error())
		}
	})

	t.Run("succeeds when in a git repository", func(t *testing.T) {
		// Skip this test because we can't easily mock the interactive prompts
		// In a real implementation, we would need to mock the promptForInput function
		// or restructure the code to allow for testing without interactive input
		t.Skip("skipping test that requires interactive input")
	})
}
