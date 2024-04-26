package resolver

import (
	"net/http"
	"testing"

	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/h2non/gock"
)

func TestResolvePipelinesFromPath(t *testing.T) {
	t.Parallel()

	t.Run("path has no repo URL", func(t *testing.T) {
		defer gock.Off()

		gock.New("https://api.buildkite.com/v2/organizations/testOrg").
			Get("/pipelines").
			Reply(200).
			BodyString(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/test.git"}]`)

		client := &http.Client{Transport: &http.Transport{}}
		gock.InterceptClient(client)

		bkClient := buildkite.NewClient(client)
		pipelines, err := resolveFromPath("../..", "testOrg", bkClient)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 0 {
			t.Errorf("Expected 0 pipeline, got %d", len(pipelines))
		}

	})

	t.Run("no pipelines found", func(t *testing.T) {

		defer gock.Off()

		gock.New("https://api.buildkite.com/v2/organizations/testOrg").
			Get("/pipelines").
			Reply(200).
			BodyString(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/test.git"}]`)

		client := &http.Client{Transport: &http.Transport{}}
		gock.InterceptClient(client)

		bkClient := buildkite.NewClient(client)
		pipelines, err := resolveFromPath(".", "testOrg", bkClient)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 0 {
			t.Errorf("Expected 0 pipeline, got %d", len(pipelines))
		}

	})

	t.Run("one pipeline", func(t *testing.T) {

		defer gock.Off()

		gock.New("https://api.buildkite.com/v2/organizations/testOrg").
			Get("/pipelines").
			Reply(200).
			BodyString(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/cli.git"}]`)

		client := &http.Client{Transport: &http.Transport{}}
		gock.InterceptClient(client)

		bkClient := buildkite.NewClient(client)
		pipelines, err := resolveFromPath(".", "testOrg", bkClient)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 1 {
			t.Errorf("Expected 1 pipeline, got %d", len(pipelines))
		}
	})

	t.Run("multiple pipelines", func(t *testing.T) {

		defer gock.Off()

		gock.New("https://api.buildkite.com/v2/organizations/testOrg").
			Get("/pipelines").
			Reply(200).
			BodyString(`[{"slug": "my-pipeline", "repository": "git@github.com:buildkite/cli.git"},
						{"slug": "my-pipeline-2", "repository": "git@github.com:buildkite/cli.git"}]`)

		client := &http.Client{Transport: &http.Transport{}}
		gock.InterceptClient(client)

		bkClient := buildkite.NewClient(client)
		pipelines, err := resolveFromPath(".", "testOrg", bkClient)
		if err != nil {
			t.Errorf("Error: %s", err)
		}
		if len(pipelines) != 2 {
			t.Errorf("Expected 2 pipeline, got %d", len(pipelines))
		}
	})

}
