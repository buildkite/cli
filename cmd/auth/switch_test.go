package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
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

func TestCmdSwitch(t *testing.T) {
	t.Parallel()

	t.Run("switches already selected org", func(t *testing.T) {
		t.Parallel()
		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("testing", true)
		selected := "testing"
		err := switchRun(&selected, conf, true, false)
		if err != nil {
			t.Error("expected no error")
		}
		if conf.OrganizationSlug() != "testing" {
			t.Error("expected no change in organization")
		}
	})

	t.Run("switches existing org", func(t *testing.T) {
		t.Parallel()

		// add some configurations
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		conf.SelectOrganization("testing", true)
		conf.EnsureOrganization("testing")
		conf.EnsureOrganization("default")
		// now get a new empty config
		conf = config.New(fs, nil)
		selected := "testing"
		err := switchRun(&selected, conf, true, false)
		if err != nil {
			t.Errorf("expected no error: %s", err)
		}
		if conf.OrganizationSlug() != "testing" {
			t.Error("expected no change in organization")
		}
	})

	t.Run("errors if missing org", func(t *testing.T) {
		t.Parallel()
		selected := "testing"
		conf := config.New(afero.NewMemMapFs(), nil)
		err := switchRun(&selected, conf, true, false)
		if err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("reads organization from user's config file", func(t *testing.T) {
		home := t.TempDir()
		setEnv(t, "HOME", home)
		xdgConfig := filepath.Join(home, ".config")
		setEnv(t, "XDG_CONFIG_HOME", xdgConfig)
		setEnv(t, "BUILDKITE_API_TOKEN", "")
		setEnv(t, "BUILDKITE_ORGANIZATION_SLUG", "")
		if err := os.MkdirAll(xdgConfig, 0o755); err != nil {
			t.Fatalf("failed to create config dir: %v", err)
		}

		switchrConfigPath := filepath.Join(xdgConfig, "bk.yaml")
		content := []byte("selected_org: testing\norganizations:\n  testing:\n    api_token: token-123\n")
		if err := os.WriteFile(switchrConfigPath, content, 0o644); err != nil {
			t.Fatalf("failed to write switchr config: %v", err)
		}

		conf := config.New(afero.NewOsFs(), nil)
		if got := conf.OrganizationSlug(); got != "testing" {
			t.Fatalf("expected organization from file, got %q", got)
		}
		if got := conf.APIToken(); got != "token-123" {
			t.Fatalf("expected token from file, got %q", got)
		}

		selected := "testing"
		if err := switchRun(&selected, conf, false, true); err != nil {
			t.Fatalf("expected switchRun to succeed: %v", err)
		}
	})

	t.Run("preserves organization name case", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name    string
			orgName string
		}{
			{"mixed case", "gridX"},
			{"uppercase", "ACME"},
			{"lowercase", "buildkite"},
			{"camelCase", "myOrg"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				fs := afero.NewMemMapFs()
				conf := config.New(fs, nil)

				// Configure organization with specific case
				if err := conf.EnsureOrganization(tc.orgName); err != nil {
					t.Fatalf("EnsureOrganization failed: %v", err)
				}
				if err := conf.SelectOrganization(tc.orgName, false); err != nil {
					t.Fatalf("SelectOrganization failed: %v", err)
				}

				// Switch the organization
				if err := switchRun(&tc.orgName, conf, false, true); err != nil {
					t.Fatalf("switchRun failed: %v", err)
				}

				// Verify case is preserved
				gotOrg := conf.OrganizationSlug()
				if gotOrg != tc.orgName {
					t.Errorf("expected organization %q, got %q - case was not preserved", tc.orgName, gotOrg)
				}
			})
		}
	})
}
