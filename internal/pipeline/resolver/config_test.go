package resolver

import (
	"context"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/spf13/afero"
)

func TestResolvePipelineFromConfig(t *testing.T) {
	t.Parallel()

	t.Run("no pipelines from config", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)
		resolve := ResolveFromConfig(conf, PassthruPicker)
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

		pipelines := []pipeline.Pipeline{{Name: "pipeline1"}}
		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SetPreferredPipelines(pipelines, true)
		resolve := ResolveFromConfig(conf, PassthruPicker)
		selected, err := resolve(context.Background())
		if err != nil {
			t.Errorf("failed to resolve from config")
		}

		if selected == nil {
			t.Errorf("pipeline must not be nil")
		}

		if selected != nil && selected.Name != pipelines[0].Name {
			t.Errorf("pipeline name must be pipeline1")
		}
	})

	t.Run("Resolve to many pipelines", func(t *testing.T) {
		t.Parallel()

		pipelines := []pipeline.Pipeline{{Name: "pipeline1"}, {Name: "pipeline2"}, {Name: "pipeline3"}}
		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SetPreferredPipelines(pipelines, true)
		resolve := ResolveFromConfig(conf, PassthruPicker)
		selected, err := resolve(context.Background())
		if err != nil {
			t.Errorf("failed to resolve from config")
		}

		if selected == nil {
			t.Errorf("pipeline must not be nil")
		}

		if selected != nil && selected.Name != pipelines[0].Name {
			t.Errorf("pipeline name should resolve temporarily to pipeline1")
		}
	})
}
