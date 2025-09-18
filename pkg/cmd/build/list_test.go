package build

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

func TestFilterBuilds(t *testing.T) {
	now := time.Now()
	builds := []buildkite.Build{
		{
			Number:     1,
			Message:    "Fast build",
			StartedAt:  &buildkite.Timestamp{Time: now.Add(-5 * time.Minute)},
			FinishedAt: &buildkite.Timestamp{Time: now.Add(-4 * time.Minute)}, // 1 minute
		},
		{
			Number:     2,
			Message:    "Long build",
			StartedAt:  &buildkite.Timestamp{Time: now.Add(-30 * time.Minute)},
			FinishedAt: &buildkite.Timestamp{Time: now.Add(-10 * time.Minute)}, // 20 minutes
		},
	}

	opts := buildListOptions{duration: "10m"}
	filtered, err := applyClientSideFilters(builds, opts)
	if err != nil {
		t.Fatalf("applyClientSideFilters failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("Expected 1 build >= 10m, got %d", len(filtered))
	}

	opts = buildListOptions{message: "Fast"}
	filtered, err = applyClientSideFilters(builds, opts)
	if err != nil {
		t.Fatalf("applyClientSideFilters failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("Expected 1 build with 'Fast', got %d", len(filtered))
	}
}

type fakeTransport struct {
	pages            [][]buildkite.Build
	perPageRequested []int
}

func (ft *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	q := req.URL.Query()
	page := 1
	if p := q.Get("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
	}
	perPage := 0
	if pp := q.Get("per_page"); pp != "" {
		fmt.Sscanf(pp, "%d", &perPage)
	}
	ft.perPageRequested = append(ft.perPageRequested, perPage)
	idx := page - 1
	var data []byte
	if idx < 0 || idx >= len(ft.pages) {
		data = []byte("[]")
	} else {
		b, _ := json.Marshal(ft.pages[idx])
		data = b
	}
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(data)),
		Header:     make(http.Header),
		Request:    req,
	}
	resp.Header.Set("Content-Type", "application/json")
	return resp, nil
}

func TestFetchBuildsLimitSlicing(t *testing.T) {
	all := make([]buildkite.Build, 0, 250)
	for i := 0; i < 250; i++ {
		all = append(all, buildkite.Build{Number: i + 1})
	}
	perPage := 100
	pages := [][]buildkite.Build{
		all[0:perPage],
		all[perPage : 2*perPage],
		all[2*perPage:],
	}

	ft := &fakeTransport{pages: pages}

	apiClient, err := buildkite.NewOpts(
		buildkite.WithBaseURL("https://api.example.buildkite"),
		buildkite.WithHTTPClient(&http.Client{Transport: ft}),
	)
	if err != nil {
		t.Fatalf("failed to create api client: %v", err)
	}

	fs := afero.NewMemMapFs()
	conf := config.New(fs, nil)
	conf.SelectOrganization("uber", true)

	f := &factory.Factory{Config: conf, RestAPIClient: apiClient}

	opts := buildListOptions{pipeline: "my-pipeline", limit: 230}
	listOpts := &buildkite.BuildsListOptions{ListOptions: buildkite.ListOptions{PerPage: perPage}}

	cmd := &cobra.Command{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.SetContext(ctx)
	builds, err := fetchBuilds(cmd, f, conf.OrganizationSlug(), opts, listOpts, output.FormatText)
	if err != nil {
		t.Fatalf("fetchBuilds returned error: %v", err)
	}
	if len(builds) != 230 {
		t.Fatalf("expected %d builds, got %d", 230, len(builds))
	}
	if builds[len(builds)-1].Number != 230 {
		t.Fatalf("expected last build number 230, got %d", builds[len(builds)-1].Number)
	}

	// Assert that each page (except possibly the last) contains exactly perPage items
	for i, page := range ft.pages {
		if i < len(ft.pages)-1 && len(page) != perPage {
			t.Errorf("page %d: expected %d items, got %d", i+1, perPage, len(page))
		}
	}

	// Assert that the per_page parameter was always set to perPage
	for i, pp := range ft.perPageRequested {
		if pp != perPage {
			t.Errorf("request %d: expected per_page=%d, got %d", i+1, perPage, pp)
		}
	}
}
