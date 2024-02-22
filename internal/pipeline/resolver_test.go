package pipeline_test

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/pipeline"
)

func TestAggregateResolver(t *testing.T) {
	t.Parallel()

	t.Run("it loops over resolvers until one returns", func(t *testing.T) {
		t.Parallel()

		agg := pipeline.AggregateResolver{
			func() (pipeline.Pipeline, error) { return nil, nil },
			func() (pipeline.Pipeline, error) { return 42, nil },
		}

		p, err := agg.Resolve()

		if p.(int) != 42 {
			t.Fatalf("Resolve function did not return expected value: %d", p.(int))
		}
		if err != nil {
			t.Fatal("Resolve returned an error")
		}
	})

	t.Run("returns nil if nothing resolves", func(t *testing.T) {
		t.Parallel()

		agg := pipeline.AggregateResolver{}

		p, err := agg.Resolve()

		if p != nil && err != nil {
			t.Fatal("Resolve did not return nil")
		}
	})
}
