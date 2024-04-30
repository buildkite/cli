package pipelines

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
)

func TestResolvePipelineFromConfig(t *testing.T) {

	t.Run("local config does not exist", func(t *testing.T) {
		l := config.LocalConfig{ // empty local config
		}

		resolve := PipelineResolverFromConfig(&l)
		selected, err := resolve()
		if err != nil {
			t.Errorf("failed to resolve from config")
		}

		if selected != nil {
			t.Errorf("pipeline must be nil")
		}
	})

	t.Run("local config exists with default pipeline defined", func(t *testing.T) {
		l := config.LocalConfig{
			DefaultPipeline: "bk-1",
			Organization:    "bk",
			Pipelines:       []string{"bk-1"},
		}

		resolve := PipelineResolverFromConfig(&l)
		selected, err := resolve()
		if err != nil {
			t.Errorf("failed to resolve from config")
		}

		if selected == nil {
			t.Errorf("no pipeline selected from config")
		}

		if selected.Name != l.DefaultPipeline {
			t.Errorf("expected %s, got %s ", l.DefaultPipeline, selected.Name)
		}

	})

}
