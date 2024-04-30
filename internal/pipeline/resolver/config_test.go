package resolver

import (
	"os"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"gopkg.in/yaml.v3"
)

func TestResolvePipelineFromConfig(t *testing.T) {

	t.Run("local config does not exist", func(t *testing.T) {
		f := factory.Factory{
			LocalConfig: &config.LocalConfig{},
		}

		pipelines, _ := ResolveFromConfig(&f)
		if len(pipelines) > 0 {
			t.Errorf("Expected empty string, got %d pipelines: %v", len(pipelines), pipelines)
		}
	})

	t.Run("local config exists but no pipeline defined", func(t *testing.T) {
		f := factory.Factory{
			LocalConfig: &config.LocalConfig{},
		}

		f.LocalConfig.Pipeline = ""
		pipelines, _ := ResolveFromConfig(&f)
		if len(pipelines) > 0 {
			t.Errorf("Expected empty string, got %d pipelines: %v", len(pipelines), pipelines)
		}

	})

	t.Run("local config exists and a pipeline is defined", func(t *testing.T) {
		f := factory.Factory{
			LocalConfig: &config.LocalConfig{},
		}

		newLocalConfig := make(map[string]interface{})
		newLocalConfig["pipeline"] = "new-sample-pipeline"
		newData, err := yaml.Marshal(&newLocalConfig)
		if err != nil {
			t.Errorf("Error: %s", err)
		}

		err = os.WriteFile(".bk.yaml", newData, 0o644)
		if err != nil {
			t.Errorf("Error: %s", err)
		}

		pipelines, _ := ResolveFromConfig(&f)
		if len(pipelines) > 0 && pipelines[0] != newLocalConfig["pipeline"] {
			t.Errorf("Expected %s, got %s", newLocalConfig["pipeline"], pipelines[0])
		}
		os.Remove(".bk.yaml")
	})

}
