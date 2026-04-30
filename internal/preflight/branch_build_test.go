package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestResolveBuilds_BatchesRequestsToAvoidLongQuery(t *testing.T) {
	const maxRawQueryLen = 6500

	var requestQueryLens []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/builds") {
			http.NotFound(w, r)
			return
		}

		requestQueryLens = append(requestQueryLens, len(r.URL.RawQuery))
		if len(r.URL.RawQuery) > maxRawQueryLen {
			http.Error(w, `{"message":"request uri too long"}`, http.StatusRequestURITooLong)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		builds := make([]buildkite.Build, 0, len(r.URL.Query()["branch[]"]))
		for _, branch := range r.URL.Query()["branch[]"] {
			builds = append(builds, buildkite.Build{Branch: branch, State: "passed"})
		}
		if err := json.NewEncoder(w).Encode(builds); err != nil {
			t.Errorf("encoding response: %v", err)
		}
	}))
	defer server.Close()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("creating buildkite client: %v", err)
	}

	branches := make([]BranchBuild, 80)
	for i := range branches {
		branches[i] = BranchBuild{
			Branch: fmt.Sprintf("bk/preflight/%s-%02d", strings.Repeat("x", 96), i),
		}
	}

	if err := ResolveBuilds(context.Background(), client, "test-org", "test-pipeline", branches); err != nil {
		t.Fatalf("ResolveBuilds() error: %v", err)
	}

	if len(requestQueryLens) < 2 {
		t.Fatalf("expected ResolveBuilds to batch requests, got %d request(s)", len(requestQueryLens))
	}

	for _, queryLen := range requestQueryLens {
		if queryLen > maxRawQueryLen {
			t.Fatalf("expected each request query to stay under %d bytes, got %d", maxRawQueryLen, queryLen)
		}
	}

	for i := range branches {
		if branches[i].Build == nil {
			t.Fatalf("expected branch %q to have a resolved build", branches[i].Branch)
		}
		if branches[i].Build.Branch != branches[i].Branch {
			t.Fatalf("expected build for %q, got %q", branches[i].Branch, branches[i].Build.Branch)
		}
	}
}
