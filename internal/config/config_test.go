package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

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
		t.Parallel()

		fs := afero.NewMemMapFs()
		err := prepareTestDirectory(fs, "local.basic.yaml", localConfigFilePath)
		if err != nil {
			t.Fatal(err)
		}

		// try to load configuration
		conf := New(fs, nil)

		// confirm we get the expected values
		if conf.localConfig.GetString("selected_org") != "buildkite-test" {
			t.Errorf("OrganizationSlug() does not match: %s", conf.OrganizationSlug())
		}
		if conf.localConfig.GetString("organizations.buildkite-test.api_token") != "test-token-1234" {
			t.Errorf("APIToken() does not match: %s", conf.APIToken())
		}

		if len(conf.PreferredPipelines()) != 2 {
			t.Errorf("PreferredPipelines() does not match: %d", len(conf.PreferredPipelines()))
		}
	})
}
