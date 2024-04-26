package resolver_test

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
)

func TestAggregateResolver(t *testing.T) {
	t.Parallel()

	t.Run("it loops over resolvers until one returns", func(t *testing.T) {
		t.Parallel()

		agg := resolver.AggregateResolver{
			func() (*pipeline.Pipeline, error) { return nil, nil },
			func() (*pipeline.Pipeline, error) { return &pipeline.Pipeline{Name: "test"}, nil },
		}

		p, err := agg.Resolve()

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

		p, err := agg.Resolve()

		if p != nil && err != nil {
			t.Fatal("Resolve did not return nil")
		}
	})
}
