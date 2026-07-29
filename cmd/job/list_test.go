package job

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/spf13/afero"
)

func TestJobListKeyFilterFlags(t *testing.T) {
	var cmd ListCmd
	parser, err := kong.New(&cmd, kong.Vars{"output_default_format": ""})
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	if _, err := parser.Parse([]string{"--build", "429", "--step-key", "test", "--group-key", "verification"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cmd.StepKey != "test" {
		t.Fatalf("StepKey = %q, want test", cmd.StepKey)
	}
	if cmd.GroupKey != "verification" {
		t.Fatalf("GroupKey = %q, want verification", cmd.GroupKey)
	}
	for _, want := range []string{"--step-key", "--group-key", "Server-side filters"} {
		if !strings.Contains(cmd.Help(), want) {
			t.Fatalf("Help() does not contain %q", want)
		}
	}
}

func TestFetchJobListByBuildUsesJobsEndpoint(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v2/organizations/test-org/pipelines/my-app/builds/429/jobs" {
			t.Fatalf("path = %s, want dedicated jobs endpoint", r.URL.Path)
		}
		if got := r.URL.Query()["state[]"]; fmt.Sprint(got) != "[failed timed_out]" {
			t.Fatalf("state[] = %v, want repeated failed and timed_out values", got)
		}
		if got := r.URL.Query().Get("include_retried_jobs"); got != "false" {
			t.Fatalf("include_retried_jobs = %q, want false", got)
		}
		if got := r.URL.Query().Get("step_key"); got != "test" {
			t.Fatalf("step_key = %q, want test", got)
		}
		if got := r.URL.Query().Get("group_key"); got != "verification" {
			t.Fatalf("group_key = %q, want verification", got)
		}
		if got := r.URL.Query().Get("per_page"); got != "20" {
			t.Fatalf("per_page = %q, want 20", got)
		}

		writeJobsPage(t, w, []buildkite.Job{{ID: "job-1", State: "failed"}}, "")
	}))
	t.Cleanup(server.Close)

	f := newJobListTestFactory(t, server.URL, nil)
	opts := jobListOptions{
		pipeline: "my-app",
		build:    "429",
		stepKey:  "test",
		groupKey: "verification",
		state:    []string{"failed", "timed_out"},
		limit:    20,
	}
	listOpts, err := jobListOptionsFromFlags(&opts)
	if err != nil {
		t.Fatalf("jobListOptionsFromFlags() error = %v", err)
	}

	jobs, resolvedPipeline, _, err := fetchJobList(context.Background(), f, "test-org", opts, listOpts)
	if err != nil {
		t.Fatalf("fetchJobList() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if resolvedPipeline == nil || resolvedPipeline.Org != "test-org" || resolvedPipeline.Name != "my-app" {
		t.Fatalf("resolved pipeline = %#v", resolvedPipeline)
	}
	if len(jobs) != 1 || jobs[0].ID != "job-1" {
		t.Fatalf("jobs = %#v", jobs)
	}
}

func TestFetchJobsByBuildCursorPaginationAndLimits(t *testing.T) {
	tests := []struct {
		name           string
		totalJobs      int
		opts           jobListOptions
		fetchAll       bool
		wantJobs       int
		wantPageSizes  []int
		wantPageStarts []int
	}{
		{
			name:           "limit 20 requests and emits at most 20",
			totalJobs:      250,
			opts:           jobListOptions{limit: 20},
			wantJobs:       20,
			wantPageSizes:  []int{20},
			wantPageStarts: []int{0},
		},
		{
			name:           "limit 150 follows the next cursor",
			totalJobs:      250,
			opts:           jobListOptions{limit: 150},
			wantJobs:       150,
			wantPageSizes:  []int{100, 50},
			wantPageStarts: []int{0, 100},
		},
		{
			name:           "no limit follows every cursor",
			totalJobs:      205,
			opts:           jobListOptions{noLimit: true},
			fetchAll:       true,
			wantJobs:       205,
			wantPageSizes:  []int{100, 100, 100},
			wantPageStarts: []int{0, 100, 200},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pageSizes []int
			var pageStarts []int
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				perPage, err := strconv.Atoi(r.URL.Query().Get("per_page"))
				if err != nil {
					t.Fatalf("invalid per_page: %v", err)
				}
				if perPage > pageSize {
					t.Fatalf("per_page = %d, exceeds %d", perPage, pageSize)
				}
				start := 0
				if after := r.URL.Query().Get("after"); after != "" {
					start, err = strconv.Atoi(after)
					if err != nil {
						t.Fatalf("invalid after cursor: %v", err)
					}
				}
				pageSizes = append(pageSizes, perPage)
				pageStarts = append(pageStarts, start)
				if got := r.URL.Query().Get("step_key"); got != "test" {
					t.Fatalf("step_key = %q, want test on every page", got)
				}
				if got := r.URL.Query().Get("group_key"); got != "verification" {
					t.Fatalf("group_key = %q, want verification on every page", got)
				}

				end := min(start+perPage, tt.totalJobs)
				jobs := make([]buildkite.Job, 0, end-start)
				for i := start; i < end; i++ {
					jobs = append(jobs, buildkite.Job{ID: fmt.Sprintf("job-%03d", i)})
				}
				next := ""
				if end < tt.totalJobs {
					next = fmt.Sprintf("%s%s?after=%d&per_page=%d", server.URL, r.URL.Path, end, perPage)
				}
				writeJobsPage(t, w, jobs, next)
			}))
			t.Cleanup(server.Close)

			f := newJobListTestFactory(t, server.URL, nil)
			tt.opts.stepKey = "test"
			tt.opts.groupKey = "verification"
			jobs, err := fetchJobsByBuild(context.Background(), f.RestAPIClient, "test-org", "my-app", "429", tt.opts, tt.fetchAll)
			if err != nil {
				t.Fatalf("fetchJobsByBuild() error = %v", err)
			}
			if len(jobs) != tt.wantJobs {
				t.Fatalf("jobs = %d, want %d", len(jobs), tt.wantJobs)
			}
			if fmt.Sprint(pageSizes) != fmt.Sprint(tt.wantPageSizes) {
				t.Fatalf("page sizes = %v, want %v", pageSizes, tt.wantPageSizes)
			}
			if fmt.Sprint(pageStarts) != fmt.Sprint(tt.wantPageStarts) {
				t.Fatalf("page starts = %v, want %v", pageStarts, tt.wantPageStarts)
			}
		})
	}
}

func TestBuildJobListFetchesAllPagesBeforeClientFilteringAndOrdering(t *testing.T) {
	started := time.Now().Add(-30 * time.Minute)
	firstPage := []buildkite.Job{
		{ID: "wrong-queue", State: "failed", ClusterQueueID: "other-queue-id", StartedAt: &buildkite.Timestamp{Time: started}, FinishedAt: &buildkite.Timestamp{Time: started.Add(20 * time.Minute)}},
	}
	secondPage := []buildkite.Job{
		{ID: "short", State: "failed", ClusterQueueID: "test-queue-id", StartedAt: &buildkite.Timestamp{Time: started}, FinishedAt: &buildkite.Timestamp{Time: started.Add(2 * time.Minute)}},
		{ID: "long", State: "failed", ClusterQueueID: "test-queue-id", StartedAt: &buildkite.Timestamp{Time: started}, FinishedAt: &buildkite.Timestamp{Time: started.Add(15 * time.Minute)}},
	}

	var restRequests int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var request struct {
				OperationName string `json:"operationName"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode GraphQL request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			switch request.OperationName {
			case "FindClusters":
				_, _ = w.Write([]byte(`{"data":{"organization":{"clusters":{"edges":[{"node":{"id":"cluster-id","name":"Test"}}],"pageInfo":{"hasNextPage":false}}}}}`))
			case "FindQueuesForCluster":
				_, _ = w.Write([]byte(`{"data":{"node":{"__typename":"Cluster","id":"cluster-id","name":"Test","queues":{"edges":[{"node":{"id":"test-queue-id","key":"test"}}],"pageInfo":{"hasNextPage":false}}}}}`))
			default:
				t.Fatalf("unexpected GraphQL operation %q", request.OperationName)
			}
			return
		}

		restRequests++
		if restRequests == 1 {
			writeJobsPage(t, w, firstPage, server.URL+r.URL.Path+"?after=next&per_page=100")
			return
		}
		writeJobsPage(t, w, secondPage, "")
	}))
	t.Cleanup(server.Close)

	f := newJobListTestFactory(t, server.URL, nil)
	f.GraphQLClient = graphql.NewClient(server.URL, server.Client())
	opts := jobListOptions{pipeline: "my-app", build: "429", queue: "test", duration: ">1m", orderBy: "duration", limit: 1}
	listOpts, err := jobListOptionsFromFlags(&opts)
	if err != nil {
		t.Fatalf("jobListOptionsFromFlags() error = %v", err)
	}
	jobs, _, queueIDs, err := fetchJobList(context.Background(), f, "test-org", opts, listOpts)
	if err != nil {
		t.Fatalf("fetchJobList() error = %v", err)
	}
	jobs, err = applyClientSideFilters(jobs, opts, queueIDs)
	if err != nil {
		t.Fatalf("applyClientSideFilters() error = %v", err)
	}
	jobs = sortJobs(jobs, opts.orderBy)
	if len(jobs) > opts.limit {
		jobs = jobs[:opts.limit]
	}

	if restRequests != 2 {
		t.Fatalf("REST requests = %d, want all 2 pages", restRequests)
	}
	if len(jobs) != 1 || jobs[0].ID != "long" {
		t.Fatalf("jobs = %#v, want longest matching job", jobs)
	}
}

func TestResolveJobListPipeline(t *testing.T) {
	t.Run("from pipeline flag", func(t *testing.T) {
		f := newJobListTestFactory(t, "http://127.0.0.1", nil)
		got, err := resolveJobListPipeline(context.Background(), f, "other-org/my-app")
		if err != nil {
			t.Fatalf("resolveJobListPipeline() error = %v", err)
		}
		if got.Org != "other-org" || got.Name != "my-app" {
			t.Fatalf("pipeline = %#v", got)
		}
	})

	t.Run("from config", func(t *testing.T) {
		f := newJobListTestFactory(t, "http://127.0.0.1", nil)
		if err := f.Config.SetPreferredPipelines([]pipeline.Pipeline{{Org: "test-org", Name: "configured-app"}}); err != nil {
			t.Fatalf("SetPreferredPipelines() error = %v", err)
		}
		got, err := resolveJobListPipeline(context.Background(), f, "")
		if err != nil {
			t.Fatalf("resolveJobListPipeline() error = %v", err)
		}
		if got.Org != "test-org" || got.Name != "configured-app" {
			t.Fatalf("pipeline = %#v", got)
		}
	})

	t.Run("from repository", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v2/organizations/test-org/pipelines" {
				t.Fatalf("path = %s, want pipelines endpoint", r.URL.Path)
			}
			_, _ = w.Write([]byte(`[{"slug":"repo-app","repository":"git@github.com:buildkite/cli.git"}]`))
		}))
		t.Cleanup(server.Close)

		f := newJobListTestFactory(t, server.URL, jobListTestRepository(t, "https://github.com/buildkite/cli.git"))
		got, err := resolveJobListPipeline(context.Background(), f, "")
		if err != nil {
			t.Fatalf("resolveJobListPipeline() error = %v", err)
		}
		if got.Org != "test-org" || got.Name != "repo-app" {
			t.Fatalf("pipeline = %#v", got)
		}
	})

	t.Run("missing pipeline context", func(t *testing.T) {
		t.Chdir(t.TempDir())
		f := newJobListTestFactory(t, "http://127.0.0.1", nil)
		_, err := resolveJobListPipeline(context.Background(), f, "")
		if err == nil || !strings.Contains(err.Error(), "--build requires a pipeline") || !strings.Contains(err.Error(), "--pipeline") {
			t.Fatalf("error = %v, want useful pipeline guidance", err)
		}
	})
}

func TestFetchJobListWithoutBuildRetainsCrossBuildEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/organizations/test-org/builds" {
			t.Fatalf("path = %s, want cross-build endpoint", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"jobs":[{"id":"cross-build-job"}]}]`))
	}))
	t.Cleanup(server.Close)

	f := newJobListTestFactory(t, server.URL, nil)
	opts := jobListOptions{limit: 100}
	listOpts, err := jobListOptionsFromFlags(&opts)
	if err != nil {
		t.Fatalf("jobListOptionsFromFlags() error = %v", err)
	}
	jobs, resolvedPipeline, _, err := fetchJobList(context.Background(), f, "test-org", opts, listOpts)
	if err != nil {
		t.Fatalf("fetchJobList() error = %v", err)
	}
	if resolvedPipeline != nil {
		t.Fatalf("resolved pipeline = %#v, want nil for cross-build path", resolvedPipeline)
	}
	if len(jobs) != 1 || jobs[0].ID != "cross-build-job" {
		t.Fatalf("jobs = %#v", jobs)
	}
}

func TestJobListOptionsRejectBuildWithBuildTimeFilters(t *testing.T) {
	tests := []jobListOptions{
		{build: "429", since: "1h"},
		{build: "429", until: "30m"},
	}

	for _, opts := range tests {
		_, err := jobListOptionsFromFlags(&opts)
		if err == nil || !strings.Contains(err.Error(), "cannot be used with --build") {
			t.Fatalf("jobListOptionsFromFlags(%+v) error = %v, want incompatible flags error", opts, err)
		}
	}
}

func TestJobListOptionsRequireBuildForKeyFilters(t *testing.T) {
	tests := []jobListOptions{
		{stepKey: "test"},
		{groupKey: "verification"},
		{stepKey: "test", groupKey: "verification"},
	}

	for _, opts := range tests {
		_, err := jobListOptionsFromFlags(&opts)
		if err == nil || !strings.Contains(err.Error(), "require --build") {
			t.Fatalf("jobListOptionsFromFlags(%+v) error = %v, want build requirement", opts, err)
		}
	}
}

func TestDisplayJobsOutputCompatibility(t *testing.T) {
	job := buildkite.Job{ID: "job-1", State: "failed", Label: "Test", WebURL: "https://buildkite.com/example#job-1"}

	tests := []struct {
		name   string
		format output.Format
		want   []string
	}{
		{name: "text", format: output.FormatText, want: []string{"STATE", "failed", "Test", job.WebURL}},
		{name: "json", format: output.FormatJSON, want: []string{`"id": "job-1"`, `"state": "failed"`}},
		{name: "yaml", format: output.FormatYAML, want: []string{"id: job-1", "state: failed"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := displayJobs([]buildkite.Job{job}, tt.format, &buf); err != nil {
				t.Fatalf("displayJobs() error = %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(buf.String(), want) {
					t.Fatalf("output does not contain %q:\n%s", want, buf.String())
				}
			}
		})
	}
}

func newJobListTestFactory(t *testing.T, serverURL string, repo *git.Repository) *factory.Factory {
	t.Helper()
	client, err := buildkite.NewOpts(buildkite.WithBaseURL(serverURL))
	if err != nil {
		t.Fatalf("new Buildkite client: %v", err)
	}
	conf := config.New(afero.NewMemMapFs(), nil)
	if err := conf.SelectOrganization("test-org", true); err != nil {
		t.Fatalf("SelectOrganization() error = %v", err)
	}
	return &factory.Factory{
		Config:        conf,
		GitRepository: repo,
		RestAPIClient: client,
		Quiet:         true,
		NoInput:       true,
	}
}

func jobListTestRepository(t *testing.T, remoteURL string) *git.Repository {
	t.Helper()
	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("init repository: %v", err)
	}
	if _, err := repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{remoteURL}}); err != nil {
		t.Fatalf("create remote: %v", err)
	}
	return repo
}

func writeJobsPage(t *testing.T, w http.ResponseWriter, jobs []buildkite.Job, next string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(buildkite.JobsList{
		Items: jobs,
		Links: buildkite.JobsListLinks{Next: buildkite.JobsListLink(next)},
	}); err != nil {
		t.Fatalf("encode jobs page: %v", err)
	}
}

func TestDisplayJobs_EmptyJSON(t *testing.T) {
	var buf bytes.Buffer
	err := displayJobs([]buildkite.Job{}, output.FormatJSON, &buf)
	if err != nil {
		t.Fatalf("displayJobs failed: %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "[]" {
		t.Errorf("Expected empty JSON array '[]', got %q", got)
	}
}

func TestDisplayJobs_EmptyYAML(t *testing.T) {
	var buf bytes.Buffer
	err := displayJobs([]buildkite.Job{}, output.FormatYAML, &buf)
	if err != nil {
		t.Fatalf("displayJobs failed: %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "[]" {
		t.Errorf("Expected empty YAML array '[]', got %q", got)
	}
}

func TestFilterJobs(t *testing.T) {
	now := time.Now()
	jobs := []buildkite.Job{
		{
			ID:              "job-1",
			State:           "running",
			AgentQueryRules: []string{"queue=test-queue"},
			StartedAt:       &buildkite.Timestamp{Time: now.Add(-5 * time.Minute)},
			FinishedAt:      &buildkite.Timestamp{Time: now.Add(-4 * time.Minute)}, // 1 minute
		},
		{
			ID:              "job-2",
			State:           "passed",
			AgentQueryRules: []string{"queue=other-queue"},
			StartedAt:       &buildkite.Timestamp{Time: now.Add(-30 * time.Minute)},
			FinishedAt:      &buildkite.Timestamp{Time: now.Add(-10 * time.Minute)}, // 20 minutes
		},
	}

	opts := jobListOptions{duration: ">10m"}
	filtered, err := applyClientSideFilters(jobs, opts, nil)
	if err != nil {
		t.Fatalf("applyClientSideFilters failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("Expected 1 job >= 10m, got %d", len(filtered))
	}

	opts = jobListOptions{queue: "test-queue"}
	filtered, err = applyClientSideFilters(jobs, opts, nil)
	if err != nil {
		t.Fatalf("applyClientSideFilters failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("Expected 1 job with 'test-queue', got %d", len(filtered))
	}
}
