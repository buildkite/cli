package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/buildkite/cli/v3/pkg/keyring"
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

func unsetEnv(t *testing.T, key string) {
	original, had := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if had {
			os.Setenv(key, original)
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

	t.Run("APITokenForOrg reads legacy tokens from config", func(t *testing.T) {
		t.Parallel()
		setEnv(t, "BUILDKITE_API_TOKEN", "")

		fs := afero.NewMemMapFs()
		// Write a config with legacy token entries
		content := []byte("organizations:\n  org1:\n    api_token: token-org1\n  org2:\n    api_token: token-org2\n")
		if err := afero.WriteFile(fs, configFile(), content, 0o600); err != nil {
			t.Fatal(err)
		}
		conf := New(fs, nil)

		if conf.APITokenForOrg("org1") != "token-org1" {
			t.Errorf("expected token-org1, got %s", conf.APITokenForOrg("org1"))
		}
		if conf.APITokenForOrg("org2") != "token-org2" {
			t.Errorf("expected token-org2, got %s", conf.APITokenForOrg("org2"))
		}
		if conf.APITokenForOrg("nonexistent") != "" {
			t.Errorf("expected empty token for nonexistent org, got %s", conf.APITokenForOrg("nonexistent"))
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
		}{
			{
				name:    "mixed case organization name",
				orgName: "gridX",
			},
			{
				name:    "uppercase organization name",
				orgName: "ACME",
			},
			{
				name:    "lowercase organization name",
				orgName: "buildkite",
			},
			{
				name:    "camelCase organization name",
				orgName: "myOrg",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				fs := afero.NewMemMapFs()
				conf := New(fs, nil)

				// Register organization
				if err := conf.EnsureOrganization(tc.orgName); err != nil {
					t.Fatalf("EnsureOrganization failed: %v", err)
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
			})
		}
	})

	t.Run("OutputFormat returns correct precedence", func(t *testing.T) {
		t.Parallel()

		t.Run("defaults to json", func(t *testing.T) {
			t.Parallel()
			setEnv(t, "BUILDKITE_OUTPUT_FORMAT", "")

			fs := afero.NewMemMapFs()
			conf := New(fs, nil)

			if got := conf.OutputFormat(); got != "json" {
				t.Errorf("OutputFormat() = %q, want %q", got, "json")
			}
		})

		t.Run("env overrides config", func(t *testing.T) {
			setEnv(t, "BUILDKITE_OUTPUT_FORMAT", "yaml")

			fs := afero.NewMemMapFs()
			conf := New(fs, nil)
			conf.SetOutputFormat("text", false)

			if got := conf.OutputFormat(); got != "yaml" {
				t.Errorf("OutputFormat() = %q, want %q (env should override)", got, "yaml")
			}
		})

		t.Run("config value is used", func(t *testing.T) {
			t.Parallel()
			setEnv(t, "BUILDKITE_OUTPUT_FORMAT", "")

			fs := afero.NewMemMapFs()
			conf := New(fs, nil)
			conf.SetOutputFormat("yaml", false)

			if got := conf.OutputFormat(); got != "yaml" {
				t.Errorf("OutputFormat() = %q, want %q", got, "yaml")
			}
		})
	})

	t.Run("Quiet returns correct precedence", func(t *testing.T) {
		t.Parallel()

		t.Run("defaults to false", func(t *testing.T) {
			t.Parallel()
			setEnv(t, "BUILDKITE_QUIET", "")

			fs := afero.NewMemMapFs()
			conf := New(fs, nil)

			if conf.Quiet() {
				t.Error("Quiet() = true, want false")
			}
		})

		t.Run("env overrides config", func(t *testing.T) {
			setEnv(t, "BUILDKITE_QUIET", "true")

			fs := afero.NewMemMapFs()
			conf := New(fs, nil)

			if !conf.Quiet() {
				t.Error("Quiet() = false, want true (env should override)")
			}
		})
	})

	t.Run("NoInput returns correct precedence", func(t *testing.T) {
		t.Parallel()

		t.Run("defaults to false", func(t *testing.T) {
			t.Parallel()
			setEnv(t, "BUILDKITE_NO_INPUT", "")

			fs := afero.NewMemMapFs()
			conf := New(fs, nil)

			if conf.NoInput() {
				t.Error("NoInput() = true, want false")
			}
		})

		t.Run("env overrides config", func(t *testing.T) {
			setEnv(t, "BUILDKITE_NO_INPUT", "true")

			fs := afero.NewMemMapFs()
			conf := New(fs, nil)

			if !conf.NoInput() {
				t.Error("NoInput() = false, want true (env should override)")
			}
		})
	})

	t.Run("Pager returns correct precedence", func(t *testing.T) {
		t.Parallel()

		t.Run("defaults to less -R", func(t *testing.T) {
			t.Parallel()
			setEnv(t, "PAGER", "")

			fs := afero.NewMemMapFs()
			conf := New(fs, nil)

			if got := conf.Pager(); got != "less -R" {
				t.Errorf("Pager() = %q, want %q", got, "less -R")
			}
		})

		t.Run("env overrides config", func(t *testing.T) {
			setEnv(t, "PAGER", "more")

			fs := afero.NewMemMapFs()
			conf := New(fs, nil)
			conf.SetPager("vim")

			if got := conf.Pager(); got != "more" {
				t.Errorf("Pager() = %q, want %q (env should override)", got, "more")
			}
		})

		t.Run("config value is used", func(t *testing.T) {
			t.Parallel()
			setEnv(t, "PAGER", "")

			fs := afero.NewMemMapFs()
			conf := New(fs, nil)
			conf.SetPager("vim")

			if got := conf.Pager(); got != "vim" {
				t.Errorf("Pager() = %q, want %q", got, "vim")
			}
		})
	})
}

func TestAPITokenForOrgNoKeyring(t *testing.T) {
	// Ensure BUILDKITE_NO_KEYRING disables keychain access entirely and that
	// APITokenForOrg falls through to the config file (legacy) path without
	// attempting to call the OS keychain.
	setEnv(t, "BUILDKITE_NO_KEYRING", "1")
	setEnv(t, "CI", "")
	setEnv(t, "BUILDKITE", "")
	setEnv(t, "BUILDKITE_API_TOKEN", "")
	keyring.ResetForTesting()
	t.Cleanup(keyring.ResetForTesting)

	fs := afero.NewMemMapFs()
	content := []byte("organizations:\n  my-org:\n    api_token: legacy-token\n")
	if err := afero.WriteFile(fs, configFile(), content, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	conf := New(fs, nil)

	// Should return the legacy file token without touching the keychain.
	if got := conf.APITokenForOrg("my-org"); got != "legacy-token" {
		t.Errorf("APITokenForOrg() = %q, want %q", got, "legacy-token")
	}

	// Keyring must report unavailable.
	kr := keyring.New()
	if kr.IsAvailable() {
		t.Error("expected keyring to be unavailable when BUILDKITE_NO_KEYRING=1")
	}
}

func TestHasStoredToken(t *testing.T) {
	// Force the keyring off so we exercise the legacy config path deterministically.
	setEnv(t, "BUILDKITE_NO_KEYRING", "1")
	setEnv(t, "CI", "")
	setEnv(t, "BUILDKITE", "")
	keyring.ResetForTesting()
	t.Cleanup(keyring.ResetForTesting)

	t.Run("legacy token present", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		content := []byte("organizations:\n  my-org:\n    api_token: legacy-token\n")
		if err := afero.WriteFile(fs, configFile(), content, 0o600); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}
		conf := New(fs, nil)

		if !conf.hasStoredToken("my-org") {
			t.Error("hasStoredToken() = false, want true when legacy config has a token")
		}
	})

	t.Run("no stored token", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		conf := New(fs, nil)

		if conf.hasStoredToken("my-org") {
			t.Error("hasStoredToken() = true, want false when no credential is stored")
		}
	})
}

const shadowWarning = "Warning: BUILDKITE_API_TOKEN is overriding the credential stored for this organization."

// captureWarnings redirects credential warnings to a buffer and resets the
// warning once-guards so each test observes the warnings its own calls emit.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := warningOutput
	warningOutput = buf
	envTokenWarningOnce = sync.Once{}
	legacyTokenWarningOnce = sync.Once{}
	t.Cleanup(func() {
		warningOutput = prev
		envTokenWarningOnce = sync.Once{}
		legacyTokenWarningOnce = sync.Once{}
	})
	return buf
}

func TestAPITokenForOrgEnvVar(t *testing.T) {
	// Force the keyring off so the only possible stored credential is legacy config.
	setEnv(t, "BUILDKITE_NO_KEYRING", "1")
	setEnv(t, "CI", "")
	setEnv(t, "BUILDKITE", "")
	setEnv(t, "BUILDKITE_API_TOKEN", "env-token")
	keyring.ResetForTesting()
	t.Cleanup(keyring.ResetForTesting)

	t.Run("env var is the only credential: returned without a warning", func(t *testing.T) {
		warnings := captureWarnings(t)
		fs := afero.NewMemMapFs()
		conf := New(fs, nil)

		if got := conf.APITokenForOrg("my-org"); got != "env-token" {
			t.Errorf("APITokenForOrg() = %q, want %q", got, "env-token")
		}
		if got := warnings.String(); got != "" {
			t.Errorf("expected no warning when env var is the sole credential, got %q", got)
		}
	})

	t.Run("env var shadows a stored credential: returned with a warning", func(t *testing.T) {
		warnings := captureWarnings(t)
		fs := afero.NewMemMapFs()
		content := []byte("organizations:\n  my-org:\n    api_token: legacy-token\n")
		if err := afero.WriteFile(fs, configFile(), content, 0o600); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}
		conf := New(fs, nil)

		// Env var still wins for the returned token.
		if got := conf.APITokenForOrg("my-org"); got != "env-token" {
			t.Errorf("APITokenForOrg() = %q, want %q", got, "env-token")
		}
		if got := warnings.String(); !strings.Contains(got, shadowWarning) {
			t.Errorf("expected shadowing warning, got %q", got)
		}
	})

	// Regression for the factory's two-step org resolution: a lookup against an
	// org with no stored credential must not consume the once, otherwise a later
	// lookup against an org the env var *does* shadow would be silenced.
	t.Run("non-shadowing lookup does not suppress a later shadowing warning", func(t *testing.T) {
		warnings := captureWarnings(t)
		fs := afero.NewMemMapFs()
		content := []byte("organizations:\n  stored-org:\n    api_token: legacy-token\n")
		if err := afero.WriteFile(fs, configFile(), content, 0o600); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}
		conf := New(fs, nil)

		// env-only-org has no stored credential: env token returned, no warning.
		if got := conf.APITokenForOrg("env-only-org"); got != "env-token" {
			t.Errorf("APITokenForOrg(env-only-org) = %q, want %q", got, "env-token")
		}
		if got := warnings.String(); got != "" {
			t.Errorf("expected no warning for non-shadowing org, got %q", got)
		}

		// stored-org IS shadowed: the warning must still fire, proving the
		// earlier non-shadowing lookup did not consume the once.
		if got := conf.APITokenForOrg("stored-org"); got != "env-token" {
			t.Errorf("APITokenForOrg(stored-org) = %q, want %q", got, "env-token")
		}
		if got := warnings.String(); !strings.Contains(got, shadowWarning) {
			t.Errorf("expected shadowing warning after non-shadowing lookup, got %q", got)
		}
	})
}

func TestExperiments(t *testing.T) {
	t.Run("defaults to preflight", func(t *testing.T) {
		unsetEnv(t, "BUILDKITE_EXPERIMENTS")

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)

		if got := conf.Experiments(); got != DefaultExperiments {
			t.Errorf("Experiments() = %q, want %q", got, DefaultExperiments)
		}
	})

	t.Run("env overrides config", func(t *testing.T) {
		setEnv(t, "BUILDKITE_EXPERIMENTS", "alpha")

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)
		conf.SetExperiments("beta")

		if got := conf.Experiments(); got != "alpha" {
			t.Errorf("Experiments() = %q, want %q (env should override)", got, "alpha")
		}
	})

	t.Run("env empty string does not fall through", func(t *testing.T) {
		setEnv(t, "BUILDKITE_EXPERIMENTS", "")

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)
		conf.SetExperiments("beta")

		if got := conf.Experiments(); got != "" {
			t.Errorf("Experiments() = %q, want %q (empty env should not fall through)", got, "")
		}
	})

	t.Run("config overrides the default", func(t *testing.T) {
		unsetEnv(t, "BUILDKITE_EXPERIMENTS")

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)
		conf.SetExperiments("beta")

		if got := conf.Experiments(); got != "beta" {
			t.Errorf("Experiments() = %q, want %q", got, "beta")
		}
	})

	t.Run("SetExperiments persists", func(t *testing.T) {
		unsetEnv(t, "BUILDKITE_EXPERIMENTS")

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)

		if err := conf.SetExperiments("preflight,beta"); err != nil {
			t.Fatalf("SetExperiments() error: %v", err)
		}

		conf2 := New(fs, nil)
		if got := conf2.Experiments(); got != "preflight,beta" {
			t.Errorf("Experiments() after reload = %q, want %q", got, "preflight,beta")
		}
	})
}

func TestHasExperimentEnvOverride(t *testing.T) {
	t.Run("empty env override disables default experiments", func(t *testing.T) {
		setEnv(t, "BUILDKITE_EXPERIMENTS", "")

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)

		if conf.HasExperiment(ExperimentPreflight) {
			t.Errorf("HasExperiment(%q) = true, want false", ExperimentPreflight)
		}
	})
}

func TestHasExperiment(t *testing.T) {
	tests := []struct {
		name        string
		experiments string
		query       string
		want        bool
	}{
		{"preflight defaults on", "", ExperimentPreflight, true},
		{"single match", "preflight", "preflight", true},
		{"multiple with match", "foo,preflight,bar", "preflight", true},
		{"override without match", "foo,bar", "preflight", false},
		{"whitespace handling", " preflight , bar ", "preflight", true},
		{"other experiments still default off", "", "beta", false},
		{"partial name no match", "preflightx", "preflight", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetEnv(t, "BUILDKITE_EXPERIMENTS")

			fs := afero.NewMemMapFs()
			conf := New(fs, nil)
			conf.SetExperiments(tt.experiments)

			if got := conf.HasExperiment(tt.query); got != tt.want {
				t.Errorf("HasExperiment(%q) with experiments=%q: got %v, want %v", tt.query, tt.experiments, got, tt.want)
			}
		})
	}
}

func TestCredentialStore(t *testing.T) {
	t.Run("defaults to auto", func(t *testing.T) {
		unsetEnv(t, keyring.CredentialStoreEnv)

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)

		if got := conf.CredentialStore(); got != keyring.StoreAuto {
			t.Errorf("CredentialStore() = %q, want %q", got, keyring.StoreAuto)
		}
	})

	t.Run("env overrides user config", func(t *testing.T) {
		setEnv(t, keyring.CredentialStoreEnv, keyring.StoreKeyring)

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)
		if err := conf.SetCredentialStore(keyring.StoreSHM); err != nil {
			t.Fatalf("SetCredentialStore: %v", err)
		}

		if got := conf.CredentialStore(); got != keyring.StoreKeyring {
			t.Errorf("CredentialStore() = %q, want %q (env should override)", got, keyring.StoreKeyring)
		}
	})

	t.Run("user config overrides default", func(t *testing.T) {
		unsetEnv(t, keyring.CredentialStoreEnv)

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)
		if err := conf.SetCredentialStore(keyring.StoreSHM); err != nil {
			t.Fatalf("SetCredentialStore: %v", err)
		}

		if got := conf.CredentialStore(); got != keyring.StoreSHM {
			t.Errorf("CredentialStore() = %q, want %q", got, keyring.StoreSHM)
		}
	})

	t.Run("local config is ignored", func(t *testing.T) {
		// credential_store is intentionally user-only; a value in local
		// .bk.yaml must not affect resolution.
		unsetEnv(t, keyring.CredentialStoreEnv)

		fs := afero.NewMemMapFs()
		localPath := localConfigFilePath
		if err := afero.WriteFile(fs, localPath, []byte("credential_store: shm\n"), 0o600); err != nil {
			t.Fatalf("write local config: %v", err)
		}
		conf := New(fs, nil)

		if got := conf.CredentialStore(); got != keyring.StoreAuto {
			t.Errorf("CredentialStore() = %q, want %q (local should be ignored)", got, keyring.StoreAuto)
		}
	})

	t.Run("rejects unknown values", func(t *testing.T) {
		unsetEnv(t, keyring.CredentialStoreEnv)

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)

		if err := conf.SetCredentialStore("vault"); err == nil {
			t.Error("SetCredentialStore(\"vault\") expected error, got nil")
		}
		if got := conf.CredentialStore(); got != keyring.StoreAuto {
			t.Errorf("CredentialStore() after rejected write = %q, want %q", got, keyring.StoreAuto)
		}
	})

	t.Run("empty value clears the preference", func(t *testing.T) {
		unsetEnv(t, keyring.CredentialStoreEnv)

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)
		if err := conf.SetCredentialStore(keyring.StoreSHM); err != nil {
			t.Fatalf("SetCredentialStore: %v", err)
		}
		if err := conf.SetCredentialStore(""); err != nil {
			t.Fatalf("SetCredentialStore(\"\") error: %v", err)
		}

		if got := conf.CredentialStore(); got != keyring.StoreAuto {
			t.Errorf("CredentialStore() after clear = %q, want %q", got, keyring.StoreAuto)
		}
	})

	t.Run("setter persists across reload", func(t *testing.T) {
		unsetEnv(t, keyring.CredentialStoreEnv)

		fs := afero.NewMemMapFs()
		conf := New(fs, nil)
		if err := conf.SetCredentialStore(keyring.StoreSHM); err != nil {
			t.Fatalf("SetCredentialStore: %v", err)
		}

		conf2 := New(fs, nil)
		if got := conf2.CredentialStore(); got != keyring.StoreSHM {
			t.Errorf("CredentialStore() after reload = %q, want %q", got, keyring.StoreSHM)
		}
	})
}
