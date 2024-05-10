package validation

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/afero"
)

func TestCheckValidConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("API token is configured", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)
		_ = conf.SetTokenForOrg("testing", "testing")
		_ = conf.SelectOrganization("testing")

		f := CheckValidConfiguration(conf)
		err := f(nil, nil)
		if err != nil {
			t.Error("expected no error returned", err)
		}
	})

	t.Run("API token is not configured", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)

		f := CheckValidConfiguration(conf)
		err := f(nil, nil)

		if err == nil {
			t.Error("expected an error returned")
		}
	})
}
