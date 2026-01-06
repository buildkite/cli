package resolver_test

import (
	"testing"

	"os"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/goccy/go-yaml"
	"github.com/spf13/afero"
)

type savedConfig struct {
	SelectedOrg string   `yaml:"selected_org"`
	Pipelines   []string `yaml:"pipelines"`
}

func readSavedConfig(t *testing.T, fs afero.Fs) savedConfig {
	b, err := afero.ReadFile(fs, ".bk.yaml")
	if err != nil {
		if os.IsNotExist(err) {
			return savedConfig{}
		}
		t.Fatalf("failed to read config: %v", err)
	}

	var cfg savedConfig
	if len(b) == 0 {
		return cfg
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	return cfg
}

func TestPickers(t *testing.T) {
	t.Parallel()

	t.Run("cached picker will save to local config", func(t *testing.T) {
		t.Parallel()

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		pipelines := []pipeline.Pipeline{
			{Name: "pipeline", Org: "org"},
		}
		picked := resolver.CachedPicker(conf, resolver.PassthruPicker, true)(pipelines)

		if picked == nil {
			t.Fatal("Should not have received nil from picker")
		}

		saved := readSavedConfig(t, fs)
		if len(saved.Pipelines) != 1 || saved.Pipelines[0] != "pipeline" {
			t.Fatalf("Local config pipelines do not match expected: %#v", saved.Pipelines)
		}
	})

	t.Run("cached picker doesnt save if user makes no choice", func(t *testing.T) {
		t.Parallel()

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		pipelines := []pipeline.Pipeline{}
		resolver.CachedPicker(conf, func(p []pipeline.Pipeline) *pipeline.Pipeline { return nil }, true)(pipelines)

		b, _ := afero.ReadFile(fs, ".bk.yaml")
		expected := ""
		if string(b) != expected {
			t.Fatalf("Local config file does not match expected: %s", string(b))
		}
	})

	t.Run("cached picker saves correct pipeline first", func(t *testing.T) {
		t.Parallel()

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		pipelines := []pipeline.Pipeline{
			{Name: "first"},
			{Name: "second"},
			{Name: "third"},
		}
		resolver.CachedPicker(conf, func(p []pipeline.Pipeline) *pipeline.Pipeline { return &p[1] }, true)(pipelines)

		saved := readSavedConfig(t, fs)
		expected := []string{"second", "first", "third"}
		if len(saved.Pipelines) != len(expected) {
			t.Fatalf("Local config pipelines length mismatch: got %d want %d", len(saved.Pipelines), len(expected))
		}
		for i, name := range expected {
			if saved.Pipelines[i] != name {
				t.Fatalf("Local config pipelines mismatch at %d: got %q want %q", i, saved.Pipelines[i], name)
			}
		}
	})
}
