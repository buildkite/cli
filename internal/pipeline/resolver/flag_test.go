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

	t.Run("pipeline slug uses config org", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("testing", true)
		f := resolver.ResolveFromFlag("my-pipeline", conf)
		pipeline, err := f(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if pipeline.Org != "testing" {
			t.Errorf("expected org 'testing', got '%s'", pipeline.Org)
		}
		if pipeline.Name != "my-pipeline" {
			t.Errorf("expected pipeline 'my-pipeline', got '%s'", pipeline.Name)
		}
	})

	t.Run("org/pipeline slug extracts org", func(t *testing.T) {
		t.Parallel()

		conf := config.New(afero.NewMemMapFs(), nil)
		conf.SelectOrganization("testing", true)
		f := resolver.ResolveFromFlag("other-org/my-pipeline", conf)
		pipeline, err := f(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if pipeline.Org != "other-org" {
			t.Errorf("expected org 'other-org', got '%s'", pipeline.Org)
		}
		if pipeline.Name != "my-pipeline" {
			t.Errorf("expected pipeline 'my-pipeline', got '%s'", pipeline.Name)
		}
	})

}
