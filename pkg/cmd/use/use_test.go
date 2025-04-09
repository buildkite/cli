package use

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/afero"
)

func TestCmdUse(t *testing.T) {
	t.Parallel()

	t.Run("uses already selected org", func(t *testing.T) {
		t.Parallel()
		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("testing", true)
		selected := "testing"
		err := useRun(&selected, conf, true)
		if err != nil {
			t.Error("expected no error")
		}
		if conf.OrganizationSlug() != "testing" {
			t.Error("expected no change in organization")
		}
	})

	t.Run("uses existing org", func(t *testing.T) {
		t.Parallel()

		// add some configurations
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)
		conf.SelectOrganization("testing", true)
		conf.SetTokenForOrg("testing", "token")
		conf.SetTokenForOrg("default", "token")
		// now get a new empty config
		conf = config.New(fs, nil)
		selected := "testing"
		err := useRun(&selected, conf, true)
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
		err := useRun(&selected, conf, true)
		if err == nil {
			t.Error("expected an error")
		}
	})
}
