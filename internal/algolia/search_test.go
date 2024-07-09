package algolia

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/algolia/algoliasearch-client-go/v3/algolia/search"
)

func TestSearch(t *testing.T) {
	t.Parallel()

	t.Run("return hits with correct structure", func(t *testing.T) {
		t.Parallel()
		client := search.NewClientWithConfig(search.Configuration{Requester: &requester{
			resp: http.Response{
				StatusCode: 200,
				Body: io.NopCloser(strings.NewReader(`{
					"hits": [
						{
					"url": "https://buildkite.com/docs"
					}
					]
				}`)),
			},
		}})

		index = client.InitIndex("testing")
		results, err := Search("testing")

		if err != nil {
			t.Fatal("Should not receive an error", err)
		}
		if count := len(results); count != 1 {
			t.Fatalf("Should have returned some results: %d", count)
		}
		if results[0] != "https://buildkite.com/docs" {
			t.Fatalf("Incorrect result URL found: %s", results[0])
		}
	})

	t.Run("hits with incorrect structure", func(t *testing.T) {
		t.Parallel()
		client := search.NewClientWithConfig(search.Configuration{Requester: &requester{
			resp: http.Response{
				StatusCode: 200,
				Body: io.NopCloser(strings.NewReader(`{
					"hits": [
						{
					"not matching": "https://buildkite.com/docs"
					}
					]
				}`)),
			},
		}})

		index = client.InitIndex("testing")
		results, err := Search("testing")

		if err != nil {
			t.Fatal("Should not receive an error", err)
		}
		if count := len(results); count != 0 {
			t.Fatalf("Expected no results. Got: %d", count)
		}
	})

	t.Run("more than enough hits", func(t *testing.T) {
		t.Parallel()
		client := search.NewClientWithConfig(search.Configuration{Requester: &requester{
			resp: http.Response{
				StatusCode: 200,
				Body: io.NopCloser(strings.NewReader(`{
					"hits": [
						{
					"url": "https://buildkite.com/docs/1"
					},
					{
				"url": "https://buildkite.com/docs/2"
				},
					{
				"url": "https://buildkite.com/docs/3"
				},
					{
				"url": "https://buildkite.com/docs/4"
				}
					]
				}`)),
			},
		}})

		index = client.InitIndex("testing")
		results, err := Search("testing")

		if err != nil {
			t.Fatal("Should not receive an error", err)
		}
		if count := len(results); count != 3 {
			t.Fatalf("Expected no results. Got: %d", count)
		}
		if results[0] != "https://buildkite.com/docs/1" {
			t.Fatalf("Incorrect result URL found: %s", results[0])
		}
	})
}

type requester struct {
	resp http.Response
}

func (r *requester) Request(req *http.Request) (*http.Response, error) {
	return &r.resp, nil
}
