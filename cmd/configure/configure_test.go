package configure

import (
	"os"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/spf13/afero"
)

func TestConfigureOrganizationEnvironment(t *testing.T) {
	for _, command := range [][]string{{"configure"}, {"configure", "add"}} {
		for _, override := range []string{"", "flag-org"} {
			t.Run(command[len(command)-1]+"/"+override, func(t *testing.T) {
				t.Chdir(t.TempDir())
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				t.Setenv("BUILDKITE_API_TOKEN", "")
				t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "env-Org")
				keyring.MockForTesting()

				// An ignored organization must fail rather than wait for terminal input.
				stdin, err := os.Open(os.DevNull)
				if err != nil {
					t.Fatal(err)
				}
				oldStdin := os.Stdin
				os.Stdin = stdin
				t.Cleanup(func() {
					os.Stdin = oldStdin
					stdin.Close()
				})

				var app struct {
					Configure ConfigureCmd `cmd:""`
				}
				parser, err := kong.New(&app)
				if err != nil {
					t.Fatal(err)
				}
				args := append(append([]string{}, command...), "--token", "test-token")
				wantOrg := "env-Org"
				if override != "" {
					args = append(args, "--org", override)
					wantOrg = override
				}
				ctx, err := parser.Parse(args)
				if err != nil {
					t.Fatal(err)
				}
				if err := app.Configure.Run(ctx, cli.Globals{}); err != nil {
					t.Fatalf("configure failed: %v", err)
				}
				token, err := keyring.New().Get(wantOrg)
				if err != nil || token != "test-token" {
					t.Fatalf("token for %q = %q, %v; want test-token", wantOrg, token, err)
				}
				// Clear the env override to verify the persisted organization.
				t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
				if got := config.New(afero.NewOsFs(), nil).OrganizationSlug(); got != wantOrg {
					t.Fatalf("persisted organization = %q, want %q", got, wantOrg)
				}
			})
		}
	}
}

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
