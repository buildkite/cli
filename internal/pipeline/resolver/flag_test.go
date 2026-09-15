package resolver_test

import (
	"context"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/spf13/afero"
)

func TestResolveFromFlag(t *testing.T) {
	t.Parallel()

	t.Run("empty flag returns nil", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("testing", true)
		f := resolver.ResolveFromFlag("", conf)
		pipeline, err := f(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if pipeline != nil {
			t.Error("expected nil pipeline for empty flag")
		}
	})

	t.Run("pipeline slug uses config org and preserves case", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("ExampleOrg", true)
		f := resolver.ResolveFromFlag("ExamplePipeline", conf)
		pipeline, err := f(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if pipeline.Org != "ExampleOrg" {
			t.Errorf("expected org 'ExampleOrg', got '%s'", pipeline.Org)
		}
		if pipeline.Name != "ExamplePipeline" {
			t.Errorf("expected pipeline 'ExamplePipeline', got '%s'", pipeline.Name)
		}
	})

	t.Run("underscores in pipeline name are normalized to dashes", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("testing", true)
		f := resolver.ResolveFromFlag("my_pipeline", conf)
		pipeline, err := f(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if pipeline.Name != "my-pipeline" {
			t.Errorf("expected pipeline 'my-pipeline', got '%s'", pipeline.Name)
		}
	})

	t.Run("org/pipeline slug extracts org and preserves case", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("testing", true)
		f := resolver.ResolveFromFlag("ExampleOrg/Example-Pipeline", conf)
		pipeline, err := f(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if pipeline.Org != "ExampleOrg" {
			t.Errorf("expected org 'ExampleOrg', got '%s'", pipeline.Org)
		}
		if pipeline.Name != "Example-Pipeline" {
			t.Errorf("expected pipeline 'Example-Pipeline', got '%s'", pipeline.Name)
		}
	})
}
