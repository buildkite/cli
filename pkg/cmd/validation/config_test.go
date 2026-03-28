package validation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/config"
	bkKeyring "github.com/buildkite/cli/v3/pkg/keyring"
	"github.com/buildkite/cli/v3/pkg/oauth"
)

func TestValidateConfiguration_ExemptCommands(t *testing.T) {
	t.Setenv("BUILDKITE_API_TOKEN", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
	conf := newTestConfig(t)

	for _, path := range []string{
		"pipeline validate",
		"pipeline migrate",
		"configure",
		"configure default",
		"configure add",
	} {
		if err := ValidateConfiguration(conf, path); err != nil {
			t.Fatalf("expected no error for exempt command %q, got %v", path, err)
		}
	}
}

func TestValidateConfiguration_ExemptCommandsDoNotRefreshTokens(t *testing.T) {
	bkKeyring.MockForTesting()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("BUILDKITE_API_TOKEN", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
	t.Setenv(oauth.EnvClientID, "env-client")
	t.Setenv(oauth.LegacyEnvClientID, "")

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(oauth.TokenResponse{
			Error:     "invalid_grant",
			ErrorDesc: "should not be called",
		}); err != nil {
			t.Fatalf("Encode returned error: %v", err)
		}
	}))
	defer server.Close()

	kr := bkKeyring.New()
	if err := kr.SetSession("test-org", &oauth.Session{
		Version:      oauth.SessionVersion,
		Host:         server.URL,
		ClientID:     "stored-client",
		AccessToken:  "bkua_expired_access",
		RefreshToken: "bkrt_old_refresh",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SetSession returned error: %v", err)
	}

	conf := config.New(nil, nil)
	if err := ValidateConfiguration(conf, "configure default"); err != nil {
		t.Fatalf("expected no error for configure default, got %v", err)
	}
	if requests != 0 {
		t.Fatalf("refresh requests = %d, want 0", requests)
	}
}

func TestValidateConfiguration_MissingValues(t *testing.T) {
	t.Run("missing token and org", func(t *testing.T) {
		t.Setenv("BUILDKITE_API_TOKEN", "")
		t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
		conf := newTestConfig(t)
		if err := ValidateConfiguration(conf, "pipeline view"); err == nil {
			t.Fatalf("expected error when token and org are missing")
		}
	})

	t.Run("missing token", func(t *testing.T) {
		t.Setenv("BUILDKITE_API_TOKEN", "")
		t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "org")
		conf := newTestConfig(t)
		if err := ValidateConfiguration(conf, "pipeline view"); err == nil {
			t.Fatalf("expected error when token is missing")
		}
	})

	t.Run("token and org present", func(t *testing.T) {
		t.Setenv("BUILDKITE_API_TOKEN", "token2")
		t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "org2")
		conf := newTestConfig(t)
		if err := ValidateConfiguration(conf, "pipeline view"); err != nil {
			t.Fatalf("expected no error when token and org are set, got %v", err)
		}
	})
}

func TestValidateConfigurationForOrgRequiresCredentialsForOverrideOrg(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("BUILDKITE_API_TOKEN", "")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "current-org")

	conf := newTestConfig(t)
	if err := conf.EnsureOrganization("current-org"); err != nil {
		t.Fatalf("EnsureOrganization returned error: %v", err)
	}

	if err := ValidateConfigurationForOrg(conf, "pipeline list", "override-org"); err == nil {
		t.Fatal("expected missing credentials error for override org")
	}
}

func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	bkKeyring.MockForTesting()
	return config.New(nil, nil)
}
