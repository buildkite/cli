package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

func setEnv(t *testing.T, key, value string) {
	original, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("failed to set env %s: %v", key, err)
	}
	t.Cleanup(func() {
		var restoreErr error
		if had {
			restoreErr = os.Setenv(key, original)
		} else {
			restoreErr = os.Unsetenv(key)
		}
		if restoreErr != nil {
			t.Fatalf("failed to restore env %s: %v", key, restoreErr)
		}
	})
}

func prepareTestDirectory(fs afero.Fs, fixturePath, configPath string) error {
	// read the content of the fixture config file from the real filesystem
	in, err := os.ReadFile(filepath.Join("../../fixtures/config", fixturePath))
	if err != nil {
		return err
	}

	// create the config file in the afero filesystem
	err = fs.MkdirAll(filepath.Dir(configPath), os.ModePerm)
	if err != nil {
		return err
	}
	out, err := fs.Create(configPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = out.Write(in)
	if err != nil {
		return err
	}

	return nil
}

func TestConfig(t *testing.T) {
	t.Parallel()

	t.Run("read in local config", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		setEnv(t, "BUILDKITE_ORGANIZATION_SLUG", "")
		setEnv(t, "BUILDKITE_API_TOKEN", "")
		err := prepareTestDirectory(fs, "local.basic.yaml", localConfigFilePath)
		if err != nil {
			t.Fatal(err)
		}

		// try to load configuration
		conf := New(fs, nil)

		if got := conf.OrganizationSlug(); got != "buildkite-test" {
			t.Errorf("OrganizationSlug() does not match: %s", got)
		}
		if got := conf.APIToken(); got != "test-token-1234" {
			t.Errorf("APIToken() does not match: %s", got)
		}
		if got := conf.PreferredPipelines(); len(got) != 2 {
			t.Errorf("PreferredPipelines() does not match: %d", len(got))
		}
	})

	t.Run("GetTokenForOrg returns token for specific organization", func(t *testing.T) {
		t.Parallel()

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)

		// Set tokens for different organizations
		token1 := "token-org1"
		token2 := "token-org2"
		conf.SetTokenForOrg("org1", token1)
		conf.SetTokenForOrg("org2", token2)

		if conf.GetTokenForOrg("org1") != token1 {
			t.Errorf("expected token for org1 to be %s, got %s", token1, conf.GetTokenForOrg("org1"))
		}
		if conf.GetTokenForOrg("org2") != token2 {
			t.Errorf("expected token for org2 to be %s, got %s", token2, conf.GetTokenForOrg("org2"))
		}
		if conf.GetTokenForOrg("nonexistent") != "" {
			t.Errorf("expected empty token for nonexistent org, got %s", conf.GetTokenForOrg("nonexistent"))
		}
	})

	t.Run("loadFileConfig returns error on invalid yaml", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		path := filepath.Join(t.TempDir(), "bk.yaml")
		if err := afero.WriteFile(fs, path, []byte("selected_org: [oops"), 0o600); err != nil {
			t.Fatalf("failed to write invalid yaml: %v", err)
		}

		_, err := loadFileConfig(fs, path)
		if err == nil {
			t.Fatalf("expected error for invalid yaml, got nil")
		}
	})

	t.Run("loadFileConfig ignores missing file", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		_, err := loadFileConfig(fs, "does-not-exist.yaml")
		if err != nil {
			t.Fatalf("expected no error for missing file, got %v", err)
		}
	})

	t.Run("preserves organization name case", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name    string
			orgName string
			token   string
		}{
			{
				name:    "mixed case organization name",
				orgName: "gridX",
				token:   "test-token-gridX",
			},
			{
				name:    "uppercase organization name",
				orgName: "ACME",
				token:   "test-token-ACME",
			},
			{
				name:    "lowercase organization name",
				orgName: "buildkite",
				token:   "test-token-buildkite",
			},
			{
				name:    "camelCase organization name",
				orgName: "myOrg",
				token:   "test-token-myOrg",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				fs := afero.NewMemMapFs()
				conf := New(fs, nil)

				// Set token for organization
				if err := conf.SetTokenForOrg(tc.orgName, tc.token); err != nil {
					t.Fatalf("SetTokenForOrg failed: %v", err)
				}

				// Select organization (simulate user config scenario)
				if err := conf.SelectOrganization(tc.orgName, false); err != nil {
					t.Fatalf("SelectOrganization failed: %v", err)
				}

				// Create a new config instance to simulate reading from file
				conf2 := New(fs, nil)

				// Verify organization name case is preserved
				gotOrg := conf2.OrganizationSlug()
				if gotOrg != tc.orgName {
					t.Errorf("expected organization slug %q, got %q - case was not preserved", tc.orgName, gotOrg)
				}

				// Verify token can be retrieved with exact case
				gotToken := conf2.GetTokenForOrg(tc.orgName)
				if gotToken != tc.token {
					t.Errorf("expected token %q, got %q", tc.token, gotToken)
				}
			})
		}
	})
}
