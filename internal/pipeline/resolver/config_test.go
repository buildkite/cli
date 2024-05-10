package resolver

import (
	"context"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/afero"
)

func TestResolvePipelineFromConfig(t *testing.T) {

	t.Run("missing config", func(t *testing.T) {
		t.Parallel()

		resolve := ResolveFromConfig(nil)
		selected, err := resolve(context.Background())

		if err != nil && err.Error() != "could not determine config to use for pipeline resolution" {
			t.Errorf("unknown error encountered")
		}

		if selected != nil {
			t.Errorf("pipeline must be nil")
		}
	})

	t.Run("no pipelines from config", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)
		resolve := ResolveFromConfig(conf)
		selected, err := resolve(context.Background())
		if err != nil {
			t.Errorf("failed to resolve from config")
		}

		if selected != nil {
			t.Errorf("pipeline must be nil")
		}
	})

	t.Run("Resolve to one pipeline", func(t *testing.T) {
		t.Parallel()

		pipelines := []string{"pipeline1"}
		conf := config.New(afero.NewMemMapFs(), nil)
		conf.AddPreferredPipeline(pipelines)
		resolve := ResolveFromConfig(conf)
		selected, err := resolve(context.Background())
		if err != nil {
			t.Errorf("failed to resolve from config")
		}

		if selected == nil {
			t.Errorf("pipeline must not be nil")
		}

		if selected != nil && selected.Name != pipelines[0] {
			t.Errorf("pipeline name must be pipeline1")
		}
	})

	t.Run("Resolve to many pipelines", func(t *testing.T) {
		t.Parallel()

		pipelines := []string{"pipeline1", "pipeline2", "pipeline3"}
		conf := config.New(afero.NewMemMapFs(), nil)
		conf.AddPreferredPipeline(pipelines)
		resolve := ResolveFromConfig(conf)
		selected, err := resolve(context.Background())
		if err != nil {
			t.Errorf("failed to resolve from config")
		}

		if selected == nil {
			t.Errorf("pipeline must not be nil")
		}

		if selected != nil && selected.Name != pipelines[0] {
			t.Errorf("pipeline name should resolve temporarily to pipeline1")
		}
	})

}
