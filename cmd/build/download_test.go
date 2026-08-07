package build

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/build"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

func newBuildTestFactory(t *testing.T, serverURL string) *factory.Factory {
	t.Helper()
	client, err := buildkite.NewOpts(buildkite.WithBaseURL(serverURL))
	if err != nil {
		t.Fatalf("new buildkite client: %v", err)
	}
	return &factory.Factory{RestAPIClient: client, Quiet: true, NoInput: true}
}

// newDownloadTestServer wires the three endpoints download() calls into a
// single httptest.Server: build get, artifacts list, and artifact download.
//
// artifacts is read at request time via the pointer, so callers can populate
// it after the server has been created (e.g. once server.URL is known and
// they can embed it in Artifact.DownloadURL). A nil pointer serves an empty
// list. capturedQuery and downloads are optional side channels for tests
// that want to assert on the request or count downloads.
func newDownloadTestServer(t *testing.T, buildUUID string, artifacts *[]buildkite.Artifact, capturedQuery *string, downloads *atomic.Int32) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// Build get — returns a build with the given UUID and no jobs (so the
	// log-download path is skipped in these tests).
	mux.HandleFunc("/v2/organizations/acme/pipelines/monolith/builds/429", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("build get method = %q, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(buildkite.Build{ID: buildUUID})
	})

	// Artifacts list — captures the raw query string so tests can assert on
	// path/state without shipping a URL-matcher into the shared helper.
	mux.HandleFunc("/v2/organizations/acme/pipelines/monolith/builds/429/artifacts", func(w http.ResponseWriter, r *http.Request) {
		if capturedQuery != nil {
			*capturedQuery = r.URL.RawQuery
		}
		w.Header().Set("Content-Type", "application/json")
		payload := []buildkite.Artifact{}
		if artifacts != nil {
			payload = *artifacts
		}
		_ = json.NewEncoder(w).Encode(payload)
	})

	// Individual artifact download — bumps the counter and returns a byte
	// pattern derived from the requested path so tests can assert per-artifact.
	mux.HandleFunc("/artifact/", func(w http.ResponseWriter, r *http.Request) {
		if downloads != nil {
			downloads.Add(1)
		}
		id := strings.TrimPrefix(r.URL.Path, "/artifact/")
		_, _ = fmt.Fprintf(w, "artifact-body:%s", id)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestDownloadPassesArtifactFiltersToAPI(t *testing.T) {
	// No t.Parallel(): t.Chdir is incompatible with parallel tests.
	const buildUUID = "b-uuid-1"

	var query string
	server := newDownloadTestServer(t, buildUUID, nil, &query, nil)

	t.Chdir(t.TempDir())

	bld := &build.Build{Organization: "acme", Pipeline: "monolith", BuildNumber: 429}
	dir, matches, err := download(context.Background(), bld, "log/rspec*.json", "Finished", newBuildTestFactory(t, server.URL))
	if err != nil {
		t.Fatalf("download() error = %v", err)
	}
	if dir != "build-"+buildUUID {
		t.Errorf("dir = %q, want build-%s", dir, buildUUID)
	}
	if matches != 0 {
		t.Errorf("matches = %d, want 0 (empty artifact list)", matches)
	}

	if !strings.Contains(query, "path=log%2Frspec%2A.json") {
		t.Errorf("query %q missing path filter", query)
	}
	// The shared List helper lower-cases state, so "Finished" reaches the API as "finished".
	if !strings.Contains(query, "state=finished") {
		t.Errorf("query %q missing state=finished", query)
	}
	if !strings.Contains(query, "per_page=100") {
		t.Errorf("query %q missing per_page=100", query)
	}
}

func TestDownloadNoFiltersOmitsQueryParams(t *testing.T) {
	// No t.Parallel(): t.Chdir is incompatible with parallel tests.
	var query string
	server := newDownloadTestServer(t, "b-uuid-empty", nil, &query, nil)

	t.Chdir(t.TempDir())

	bld := &build.Build{Organization: "acme", Pipeline: "monolith", BuildNumber: 429}
	if _, _, err := download(context.Background(), bld, "", "", newBuildTestFactory(t, server.URL)); err != nil {
		t.Fatalf("download() error = %v", err)
	}

	if strings.Contains(query, "path=") {
		t.Errorf("query %q unexpectedly carries path=", query)
	}
	if strings.Contains(query, "state=") {
		t.Errorf("query %q unexpectedly carries state=", query)
	}
	// PerPage: 100 is set unconditionally by the shared List helper.
	if !strings.Contains(query, "per_page=100") {
		t.Errorf("query %q missing per_page=100", query)
	}
}

func TestDownloadWritesArtifactsWithFlatNames(t *testing.T) {
	// No t.Parallel(): t.Chdir is incompatible with parallel tests.
	const buildUUID = "b-uuid-2"

	var (
		downloads atomic.Int32
		arts      []buildkite.Artifact
	)
	// The handler reads *arts at request time, so we can populate it below
	// once we know server.URL and can embed it in each Artifact.DownloadURL.
	server := newDownloadTestServer(t, buildUUID, &arts, nil, &downloads)
	arts = []buildkite.Artifact{
		{ID: "art-1", Filename: "rspec.json", DownloadURL: server.URL + "/artifact/art-1"},
		{ID: "art-2", Filename: "coverage.xml", DownloadURL: server.URL + "/artifact/art-2"},
	}

	t.Chdir(t.TempDir())

	bld := &build.Build{Organization: "acme", Pipeline: "monolith", BuildNumber: 429}
	dir, matches, err := download(context.Background(), bld, "", "", newBuildTestFactory(t, server.URL))
	if err != nil {
		t.Fatalf("download() error = %v", err)
	}
	if matches != len(arts) {
		t.Errorf("matches = %d, want %d", matches, len(arts))
	}

	if got := downloads.Load(); got != int32(len(arts)) {
		t.Fatalf("downloads = %d, want %d", got, len(arts))
	}

	for _, a := range arts {
		want := filepath.Join(dir, fmt.Sprintf("artifact-%s-%s", a.ID, a.Filename))
		body, readErr := os.ReadFile(want)
		if readErr != nil {
			t.Fatalf("read %q: %v", want, readErr)
		}
		if wantBody := "artifact-body:" + a.ID; string(body) != wantBody {
			t.Errorf("%q contents = %q, want %q", want, body, wantBody)
		}
	}
}

func TestDownloadCmdFlagParsing(t *testing.T) {
	t.Parallel()

	var cmd DownloadCmd
	parser, err := kong.New(&cmd)
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	if _, err := parser.Parse([]string{
		"123",
		"--pipeline", "monolith",
		"--branch", "main",
		"--artifacts-path", "log/rspec*.json",
		"--artifacts-state", "Finished",
	}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cmd.BuildNumber != "123" {
		t.Errorf("BuildNumber = %q, want 123", cmd.BuildNumber)
	}
	if cmd.Pipeline != "monolith" {
		t.Errorf("Pipeline = %q, want monolith", cmd.Pipeline)
	}
	if cmd.Branch != "main" {
		t.Errorf("Branch = %q, want main", cmd.Branch)
	}
	if cmd.ArtifactsPath != "log/rspec*.json" {
		t.Errorf("ArtifactsPath = %q, want log/rspec*.json", cmd.ArtifactsPath)
	}
	if cmd.ArtifactsState != "Finished" {
		t.Errorf("ArtifactsState = %q, want Finished (parser preserves casing)", cmd.ArtifactsState)
	}
}

func TestDownloadCmdUserMineXor(t *testing.T) {
	t.Parallel()

	var cmd DownloadCmd
	parser, err := kong.New(&cmd)
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	// --user and --mine are marked xor in the struct tags; kong should reject
	// the combination.
	if _, err := parser.Parse([]string{"--user", "alice@example.com", "--mine"}); err == nil {
		t.Fatal("Parse() expected an error for --user + --mine, got nil")
	}
}

func TestWarnUnmatchedArtifactFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		path  string
		state string
		want  string
	}{
		{"no filters", "", "", ""},
		{"path only", "log/foo*.json", "", "Warning: no artifacts matched path \"log/foo*.json\".\n"},
		{"state only", "", "expired", "Warning: no artifacts matched state \"expired\".\n"},
		{"both", "log/foo*.json", "expired", "Warning: no artifacts matched path \"log/foo*.json\" and state \"expired\".\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			warnUnmatchedArtifactFilter(&buf, tt.path, tt.state)
			if got := buf.String(); got != tt.want {
				t.Fatalf("warnUnmatchedArtifactFilter(%q, %q) = %q, want %q", tt.path, tt.state, got, tt.want)
			}
		})
	}
}

func TestDownloadCapsArtifactConcurrency(t *testing.T) {
	// No t.Parallel(): t.Chdir is incompatible with parallel tests.
	const (
		buildUUID    = "b-uuid-conc"
		numArtifacts = 32
	)
	if downloadWorkerLimit >= numArtifacts {
		t.Fatalf("test needs numArtifacts (%d) > downloadWorkerLimit (%d) to be meaningful", numArtifacts, downloadWorkerLimit)
	}

	// The artifact handler holds each request briefly so that unbounded
	// fan-out (all N goroutines in flight at once) would be observable as a
	// peak equal to numArtifacts, and a working cap shows up as peak ≤ limit.
	var inflight, peak atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/organizations/acme/pipelines/monolith/builds/429", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(buildkite.Build{ID: buildUUID})
	})
	var arts []buildkite.Artifact
	mux.HandleFunc("/v2/organizations/acme/pipelines/monolith/builds/429/artifacts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(arts)
	})
	mux.HandleFunc("/artifact/", func(w http.ResponseWriter, r *http.Request) {
		n := inflight.Add(1)
		defer inflight.Add(-1)
		// CAS the running peak upwards. Simple max-of-atomics.
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		// Hold long enough for parallel calls to overlap. Total wall-clock
		// is ~(numArtifacts/limit) * this sleep.
		time.Sleep(20 * time.Millisecond)
		_, _ = fmt.Fprint(w, "ok")
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	// Now that server.URL is known, wire every artifact's DownloadURL to it.
	for i := range numArtifacts {
		arts = append(arts, buildkite.Artifact{
			ID:          fmt.Sprintf("art-%d", i),
			Filename:    fmt.Sprintf("file-%d.txt", i),
			DownloadURL: fmt.Sprintf("%s/artifact/%d", server.URL, i),
		})
	}

	t.Chdir(t.TempDir())

	bld := &build.Build{Organization: "acme", Pipeline: "monolith", BuildNumber: 429}
	_, matches, err := download(context.Background(), bld, "", "", newBuildTestFactory(t, server.URL))
	if err != nil {
		t.Fatalf("download() error = %v", err)
	}
	if matches != numArtifacts {
		t.Errorf("matches = %d, want %d", matches, numArtifacts)
	}

	got := peak.Load()
	if got > downloadWorkerLimit {
		t.Errorf("peak in-flight downloads = %d, want <= %d", got, downloadWorkerLimit)
	}
	// Guard against the test passing trivially if downloads happen to
	// serialise (e.g. a sleep that's too short to overlap). We expect real
	// parallelism up to the cap.
	if got < 2 {
		t.Errorf("peak in-flight downloads = %d, expected some parallelism (>= 2) — the sleep may be too short or the cap is 1", got)
	}
}

func TestDownloadReturnsZeroMatchesWhenFilterExcludesAll(t *testing.T) {
	// No t.Parallel(): t.Chdir is incompatible with parallel tests.
	// Empty artifacts payload — download() should return (dir, 0, nil).
	server := newDownloadTestServer(t, "b-uuid-none", nil, nil, nil)
	t.Chdir(t.TempDir())

	bld := &build.Build{Organization: "acme", Pipeline: "monolith", BuildNumber: 429}
	dir, matches, err := download(context.Background(), bld, "does-not-exist/*", "", newBuildTestFactory(t, server.URL))
	if err != nil {
		t.Fatalf("download() error = %v", err)
	}
	if matches != 0 {
		t.Errorf("matches = %d, want 0", matches)
	}
	// The build directory should still exist so callers can rely on the
	// warning-and-continue contract.
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Errorf("expected build directory %q to exist after unmatched filter: %v", dir, statErr)
	}
}

func TestDownloadCmdHelpMentionsArtifactFilters(t *testing.T) {
	t.Parallel()

	var cmd DownloadCmd
	help := cmd.Help()
	for _, want := range []string{"--artifacts-path", "--artifacts-state", "log/rspec*.json"} {
		if !strings.Contains(help, want) {
			t.Errorf("Help() missing %q", want)
		}
	}
}
