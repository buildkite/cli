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
	origDisplay := DisplayBuildsFunc
	defer func() { DisplayBuildsFunc = origDisplay }()
	type call struct {
		buildCount int
		withHeader bool
	}
	var calls []call
	DisplayBuildsFunc = func(cmd *cobra.Command, builds []buildkite.Build, format output.Format, withHeader bool) error {
		calls = append(calls, call{buildCount: len(builds), withHeader: withHeader})
		return nil
	}
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

	for i, page := range ft.pages {
		if i < len(ft.pages)-1 && len(page) != perPage {
			t.Errorf("page %d: expected %d items, got %d", i+1, perPage, len(page))
		}
	}

	for i, pp := range ft.perPageRequested {
		if pp != perPage {
			t.Errorf("request %d: expected per_page=%d, got %d", i+1, perPage, pp)
		}
	}

	if len(calls) != len(pages) {
		// In text format, display is called per page loaded
		// Accept that last page may have been truncated by slicing limit
		// So allow len(calls) to equal number of pages actually iterated (ft.perPageRequested)
		if len(calls) != len(ft.perPageRequested) {
			// fallback strict failure
			t.Fatalf("expected display calls %d or %d, got %d", len(pages), len(ft.perPageRequested), len(calls))
		}
	}
	if len(calls) > 0 && !calls[0].withHeader {
		t.Errorf("expected first display call to have header")
	}
	for i := 1; i < len(calls); i++ {
		if calls[i].withHeader {
			t.Errorf("expected only first display call to have header, call %d had header", i+1)
		}
	}
}

func setupConfirmationTestEnv(t *testing.T) (*factory.Factory, *cobra.Command, *config.Config) {
	t.Helper()
	totalBuilds := maxBuildLimit*2 + 100
	all := make([]buildkite.Build, 0, totalBuilds)
	for i := 0; i < totalBuilds; i++ {
		all = append(all, buildkite.Build{Number: i + 1})
	}
	pages := [][]buildkite.Build{}
	for i := 0; i < len(all); i += pageSize {
		end := i + pageSize
		if end > len(all) {
			end = len(all)
		}
		pages = append(pages, all[i:end])
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
	cmd := &cobra.Command{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cmd.SetContext(ctx)
	return f, cmd, conf
}

func TestFetchBuildsConfirmationDeclineFirst(t *testing.T) {
	origDisplay := DisplayBuildsFunc
	DisplayBuildsFunc = func(cmd *cobra.Command, builds []buildkite.Build, format output.Format, withHeader bool) error {
		return nil
	}
	t.Cleanup(func() { DisplayBuildsFunc = origDisplay })

	f, cmd, conf := setupConfirmationTestEnv(t)
	opts := buildListOptions{pipeline: "my-pipeline", noLimit: true}
	listOpts := &buildkite.BuildsListOptions{ListOptions: buildkite.ListOptions{PerPage: pageSize}}

	confirmCalls := 0
	origConfirm := ConfirmFunc
	ConfirmFunc = func(confirmed *bool, title string) error {
		confirmCalls++
		*confirmed = false
		return nil
	}
	t.Cleanup(func() { ConfirmFunc = origConfirm })

	builds, err := fetchBuilds(cmd, f, conf.OrganizationSlug(), opts, listOpts, output.FormatText)
	if err != nil {
		t.Fatalf("fetchBuilds returned error: %v", err)
	}
	if confirmCalls != 1 {
		t.Fatalf("expected 1 confirmation call, got %d", confirmCalls)
	}
	if len(builds) != maxBuildLimit {
		t.Fatalf("expected %d builds when declined, got %d", maxBuildLimit, len(builds))
	}
}

func TestFetchBuildsConfirmationAcceptThenDecline(t *testing.T) {
	origDisplay := DisplayBuildsFunc
	DisplayBuildsFunc = func(cmd *cobra.Command, builds []buildkite.Build, format output.Format, withHeader bool) error {
		return nil
	}
	t.Cleanup(func() { DisplayBuildsFunc = origDisplay })

	f, cmd, conf := setupConfirmationTestEnv(t)
	opts := buildListOptions{pipeline: "my-pipeline", noLimit: true}
	listOpts := &buildkite.BuildsListOptions{ListOptions: buildkite.ListOptions{PerPage: pageSize}}

	confirmCalls := 0
	origConfirm := ConfirmFunc
	ConfirmFunc = func(confirmed *bool, title string) error {
		confirmCalls++
		if confirmCalls == 1 {
			*confirmed = true
		} else {
			*confirmed = false
		}
		return nil
	}
	t.Cleanup(func() { ConfirmFunc = origConfirm })

	builds, err := fetchBuilds(cmd, f, conf.OrganizationSlug(), opts, listOpts, output.FormatText)
	if err != nil {
		t.Fatalf("fetchBuilds returned error: %v", err)
	}
	expected := maxBuildLimit * 2
	if len(builds) != expected {
		t.Fatalf("expected %d builds after accepting once then declining, got %d", expected, len(builds))
	}
	if confirmCalls != 2 {
		t.Fatalf("expected 2 confirmation calls, got %d", confirmCalls)
	}
}
