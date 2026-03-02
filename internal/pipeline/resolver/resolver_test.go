package resolver_test

import (
	"context"
	"testing"

	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
)

func TestAggregateResolver(t *testing.T) {
	t.Parallel()

	t.Run("it loops over resolvers until one returns", func(t *testing.T) {
		t.Parallel()

		agg := resolver.AggregateResolver{
			func(context.Context) (*pipeline.Pipeline, error) { return nil, nil },
			func(context.Context) (*pipeline.Pipeline, error) { return &pipeline.Pipeline{Name: "test"}, nil },
		}

		p, err := agg.Resolve(context.Background())

		if p.Name != "test" {
			t.Fatalf("Resolve function did not return expected value: %s", p.Name)
		}
		if err != nil {
			t.Fatal("Resolve returned an error")
		}
	})

	t.Run("returns nil if nothing resolves", func(t *testing.T) {
		t.Parallel()

		agg := resolver.AggregateResolver{}

		p, err := agg.Resolve(context.Background())

		if p != nil && err != nil {
			t.Fatal("Resolve did not return nil")
		}
	})
}

func TestWithOrg(t *testing.T) {
	t.Parallel()

	t.Run("returns original resolver when org is empty", func(t *testing.T) {
		t.Parallel()

		resolve := resolver.WithOrg("", func(context.Context) (*pipeline.Pipeline, error) {
			return &pipeline.Pipeline{Org: "config-org", Name: "pipeline"}, nil
		})

		p, err := resolve(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Org != "config-org" {
			t.Fatalf("expected org config-org, got %s", p.Org)
		}
	})

	t.Run("overrides resolved organization", func(t *testing.T) {
		t.Parallel()

		resolve := resolver.WithOrg("override-org", func(context.Context) (*pipeline.Pipeline, error) {
			return &pipeline.Pipeline{Org: "config-org", Name: "pipeline"}, nil
		})

		p, err := resolve(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Org != "override-org" {
			t.Fatalf("expected org override-org, got %s", p.Org)
		}
	})
}
