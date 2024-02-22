package pipelines

import (
	"net/http"
	"testing"

	"github.com/buildkite/go-buildkite/v3/buildkite"
)

func TestResolvePipelines(t *testing.T) {
	t.Parallel()

	// t.Run("one pipeline", func(t *testing.T) {
	// 	t.Parallel()
	// })

	// t.Run("multiple pipelines", func(t *testing.T) {
	// 	t.Parallel()
	// })

	// t.Run("error getting repo urls", func(t *testing.T) {
	// 	t.Parallel()
	// })

	t.Run("no pipelines found", func(t *testing.T) {
		t.Parallel()

		client := &http.Client{
			Transport: &mockRoundTripper{&http.Response{StatusCode: 200}},
		}
		bkClient := buildkite.NewClient(client)
		pipelines, _ := ResolveFromPath(".", "testorg", bkClient)
		// if err != nil {
		// 	t.Errorf("Error: %s", err)
		// }
		if len(pipelines) != 0 {
			t.Errorf("Expected 0 pipeline, got %d", len(pipelines))
		}

	})
}

type mockRoundTripper struct {
	resp *http.Response
}

func (rt *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt.resp, nil
}
