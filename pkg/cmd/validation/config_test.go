package validation

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
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

func TestValidateConfiguration_MissingValues(t *testing.T) {
	t.Run("missing token and org", func(t *testing.T) {
		t.Setenv("BUILDKITE_API_TOKEN", "")
		t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
		conf := newTestConfig(t)
		if err := ValidateConfiguration(conf, "pipeline view"); err == nil {
			t.Fatalf("expected error when token and org are missing")
		}
	})

	t.Run("missing org", func(t *testing.T) {
		t.Setenv("BUILDKITE_API_TOKEN", "token")
		t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
		conf := newTestConfig(t)
		if err := ValidateConfiguration(conf, "pipeline view"); err == nil {
			t.Fatalf("expected error when org is missing")
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

func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CI", "true") // disable keyring access in tests
	return config.New(nil, nil)
}
