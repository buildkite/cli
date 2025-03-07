package resolver

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/testutil"
)

func TestResolvePipelinesFromPath(t *testing.T) {
	t.Parallel()

	t.Run("no pipelines found", func(t *testing.T) {
		t.Parallel()
		// mock a response that doesn't match the current repository url
		s := testutil.MockHTTPServer(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/test.git"}]`)
		t.Cleanup(s.Close)

		f := testutil.CreateFactory(t, s.URL, "testOrg", testutil.GitRepository())
		pipelines, err := resolveFromRepository(f)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, len(pipelines), 0, "Number of pipelines")
	})

	t.Run("one pipeline", func(t *testing.T) {
		t.Parallel()
		// mock an http client response with a single pipeline matching the current repo url
		s := testutil.MockHTTPServer(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/cli.git"}]`)
		t.Cleanup(s.Close)

		f := testutil.CreateFactory(t, s.URL, "testOrg", testutil.GitRepository())
		pipelines, err := resolveFromRepository(f)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, len(pipelines), 1, "Number of pipelines")
	})

	t.Run("multiple pipelines", func(t *testing.T) {
		t.Parallel()
		// mock an http client response with 2 pipelines matching the current repo url
		s := testutil.MockHTTPServer(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/cli.git"}, {"slug": "my-pipeline-2", "repository": "git@github.com:buildkite/cli.git"}]`)
		t.Cleanup(s.Close)

		f := testutil.CreateFactory(t, s.URL, "testOrg", testutil.GitRepository())
		pipelines, err := resolveFromRepository(f)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, len(pipelines), 2, "Number of pipelines")
	})

	t.Run("no repository found", func(t *testing.T) {
		s := testutil.MockHTTPServer(`[{"slug": "", "repository": ""}]`)
		t.Cleanup(s.Close)

		f := testutil.CreateFactory(t, s.URL, "testOrg", nil)
		pipelines, err := resolveFromRepository(f)
		testutil.AssertEqual(t, pipelines == nil, true, "Expected nil pipelines")
		testutil.AssertNoError(t, err)
	})

	t.Run("no remote repository found", func(t *testing.T) {
		s := testutil.MockHTTPServer(`[{"slug": "", "repository": ""}]`)
		t.Cleanup(s.Close)

		f := testutil.CreateFactory(t, s.URL, "testOrg", testutil.GitRepository())
		pipelines, err := resolveFromRepository(f)
		testutil.AssertEqual(t, pipelines == nil, true, "Expected nil pipelines")
		testutil.AssertNoError(t, err)
	})
}
