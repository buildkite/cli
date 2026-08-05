package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/build"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
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
// The build get response is fixed; the artifact set and the query the caller
// used to fetch it are captured via the returned pointers.
func newDownloadTestServer(t *testing.T, buildUUID string, artifacts []buildkite.Artifact, capturedQuery *string, downloads *atomic.Int32) *httptest.Server {
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
		_ = json.NewEncoder(w).Encode(artifacts)
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
	server := newDownloadTestServer(t, buildUUID, []buildkite.Artifact{}, &query, nil)

	t.Chdir(t.TempDir())

	bld := &build.Build{Organization: "acme", Pipeline: "monolith", BuildNumber: 429}
	dir, err := download(context.Background(), bld, "log/rspec*.json", "Finished", newBuildTestFactory(t, server.URL))
	if err != nil {
		t.Fatalf("download() error = %v", err)
	}
	if dir != "build-"+buildUUID {
		t.Errorf("dir = %q, want build-%s", dir, buildUUID)
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
	server := newDownloadTestServer(t, "b-uuid-empty", []buildkite.Artifact{}, &query, nil)

	t.Chdir(t.TempDir())

	bld := &build.Build{Organization: "acme", Pipeline: "monolith", BuildNumber: 429}
	if _, err := download(context.Background(), bld, "", "", newBuildTestFactory(t, server.URL)); err != nil {
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

	var downloads atomic.Int32
	server := newDownloadTestServer(t, buildUUID, nil, nil, &downloads)

	// Point the DownloadURL back at the server's /artifact/ route so the
	// download step is exercised end-to-end.
	arts := []buildkite.Artifact{
		{ID: "art-1", Filename: "rspec.json", DownloadURL: server.URL + "/artifact/art-1"},
		{ID: "art-2", Filename: "coverage.xml", DownloadURL: server.URL + "/artifact/art-2"},
	}
	// Rewrite the server's artifacts handler with the real payload.
	server.Config.Handler = withArtifacts(t, buildUUID, arts, &downloads)

	t.Chdir(t.TempDir())

	bld := &build.Build{Organization: "acme", Pipeline: "monolith", BuildNumber: 429}
	dir, err := download(context.Background(), bld, "", "", newBuildTestFactory(t, server.URL))
	if err != nil {
		t.Fatalf("download() error = %v", err)
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

// withArtifacts rebuilds a handler mux around a fixed artifact payload — used
// by tests that need the artifacts list handler to return real data while
// keeping the build-get and download endpoints intact.
func withArtifacts(t *testing.T, buildUUID string, arts []buildkite.Artifact, downloads *atomic.Int32) http.Handler {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/v2/organizations/acme/pipelines/monolith/builds/429", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(buildkite.Build{ID: buildUUID})
	})

	mux.HandleFunc("/v2/organizations/acme/pipelines/monolith/builds/429/artifacts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(arts)
	})

	mux.HandleFunc("/artifact/", func(w http.ResponseWriter, r *http.Request) {
		if downloads != nil {
			downloads.Add(1)
		}
		id := strings.TrimPrefix(r.URL.Path, "/artifact/")
		_, _ = fmt.Fprintf(w, "artifact-body:%s", id)
	})
	return mux
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

func TestDownloadRejectsInvalidArtifactState(t *testing.T) {
	// No t.Parallel(): t.Chdir is incompatible with parallel tests.
	// Any HTTP call fails the test — download() must short-circuit on
	// validation before touching the wire.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call to %s — validation should have short-circuited", r.URL.Path)
	}))
	t.Cleanup(server.Close)

	t.Chdir(t.TempDir())

	bld := &build.Build{Organization: "acme", Pipeline: "monolith", BuildNumber: 429}
	_, err := download(context.Background(), bld, "", "finshed", newBuildTestFactory(t, server.URL))
	if err == nil {
		t.Fatal("download() with invalid state = nil, want validation error")
	}
	if !errors.Is(err, bkErrors.ErrValidation) {
		t.Fatalf("download() error = %v, want ErrValidation", err)
	}
	if !strings.Contains(err.Error(), "finshed") {
		t.Errorf("download() error %q should mention the rejected input", err)
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
