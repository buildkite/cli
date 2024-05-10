package resolver_test

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/spf13/afero"
)

func TestPickers(t *testing.T) {
	t.Parallel()

	t.Run("cached picker will save to local config", func(t *testing.T) {
		t.Parallel()

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		pipelines := []pipeline.Pipeline{
			{Name: "pipeline", Org: "org"},
		}
		picked := resolver.CachedPicker(conf, resolver.PassthruPicker)(pipelines)

		if picked == nil {
			t.Fatal("Should not have received nil from picker")
		}

		b, _ := afero.ReadFile(fs, ".bk.yaml")
		expected := "pipelines:\n    - pipeline\n"
		if string(b) != expected {
			t.Fatalf("Local config file does not match expected: %s", string(b))
		}
	})

	t.Run("cached picker doesnt save if user makes no choice", func(t *testing.T) {
		t.Parallel()

		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		pipelines := []pipeline.Pipeline{}
		resolver.CachedPicker(conf, func(p []pipeline.Pipeline) *pipeline.Pipeline { return nil })(pipelines)

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
		resolver.CachedPicker(conf, func(p []pipeline.Pipeline) *pipeline.Pipeline { return &p[1] })(pipelines)

		b, _ := afero.ReadFile(fs, ".bk.yaml")
		expected := "pipelines:\n    - second\n    - first\n    - third\n"
		if string(b) != expected {
			t.Fatalf("Local config file does not match expected: %s", string(b))
		}
	})
}
