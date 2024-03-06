package pipelines

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

func TestResolvePipelineFromConfig(t *testing.T) {

	t.Run("local config does not exist", func(t *testing.T) {
		//Test code here
		f := factory.Factory{
			LocalConfig: &config.LocalConfig{},
		}
		pipeline := ResolveFromConfig(&f)
		t.Log(pipeline)
		if pipeline != "" {
			t.Errorf("Expected empty string, got %s", pipeline)
		}
	})

	t.Run("local config exists but no pipeline defined", func(t *testing.T) {
		//Test code here
		t.Errorf("Test not implemented")
	})

	t.Run("local config exists and pipeline defined", func(t *testing.T) {
		//Test code here
		t.Errorf("Test not implemented")
	})

}
