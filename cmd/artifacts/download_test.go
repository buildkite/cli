package artifacts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

func newArtifactsTestFactory(t *testing.T, serverURL string) *factory.Factory {
	t.Helper()
	client, err := buildkite.NewOpts(buildkite.WithBaseURL(serverURL))
	if err != nil {
		t.Fatalf("new buildkite client: %v", err)
	}
	return &factory.Factory{RestAPIClient: client, Quiet: true, NoInput: true}
}

func writeArtifactsPage(t *testing.T, w http.ResponseWriter, artifacts []buildkite.Artifact, nextPageURL string) {
	t.Helper()
	if nextPageURL != "" {
		w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, nextPageURL))
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(artifacts); err != nil {
		t.Fatalf("encode artifacts: %v", err)
	}
}

func TestWriteNoArtifactsMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		path  []string
		state string
		want  string
	}{
		{"no filters", nil, "", "No artifacts found.\n"},
		{"path only", []string{"coverage/**"}, "", "No artifacts found matching path 'coverage/**'.\n"},
		{"state only", nil, "finished", "No artifacts found matching state 'finished'.\n"},
		{"both", []string{"coverage/**"}, "finished", "No artifacts found matching path 'coverage/**' and state 'finished'.\n"},
		{"multiple paths", []string{"log/*.json", "log/minitest.log"}, "finished", "No artifacts found matching path 'log/*.json' or 'log/minitest.log' and state 'finished'.\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			writeNoArtifactsMessage(&buf, tt.path, tt.state)
			if got := buf.String(); got != tt.want {
				t.Fatalf("writeNoArtifactsMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindArtifactWithJobUUIDUsesGetEndpoint(t *testing.T) {
	t.Parallel()

	const (
		jobUUID = "0193903e-ecd9-4c51-9156-0738da987e87"
		artID   = "0191727d-b5ce-4576-b37d-477ae0ca830c"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := fmt.Sprintf("/v2/organizations/acme/pipelines/monolith/builds/429/jobs/%s/artifacts/%s", jobUUID, artID)
		if r.URL.Path != wantPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, wantPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(buildkite.Artifact{ID: artID, Path: "coverage.xml"})
	}))
	t.Cleanup(server.Close)

	f := newArtifactsTestFactory(t, server.URL)
	got, err := findArtifact(context.Background(), f, "acme", "monolith", "429", artID, jobUUID)
	if err != nil {
		t.Fatalf("findArtifact() error = %v", err)
	}
	if got == nil || got.ID != artID {
		t.Fatalf("findArtifact() = %+v, want ID %q", got, artID)
	}
}

func TestFindArtifactWithoutJobUUIDScansList(t *testing.T) {
	t.Parallel()

	const artID = "wanted"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/v2/organizations/acme/pipelines/monolith/builds/429/artifacts"
		if r.URL.Path != wantPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, wantPath)
		}
		writeArtifactsPage(t, w, []buildkite.Artifact{
			{ID: "other"},
			{ID: artID, Path: "the-one.txt"},
		}, "")
	}))
	t.Cleanup(server.Close)

	f := newArtifactsTestFactory(t, server.URL)
	got, err := findArtifact(context.Background(), f, "acme", "monolith", "429", artID, "")
	if err != nil {
		t.Fatalf("findArtifact() error = %v", err)
	}
	if got == nil || got.Path != "the-one.txt" {
		t.Fatalf("findArtifact() = %+v, want the-one.txt", got)
	}
}

func TestFindArtifactNotFoundReturnsResourceError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeArtifactsPage(t, w, []buildkite.Artifact{{ID: "other"}}, "")
	}))
	t.Cleanup(server.Close)

	f := newArtifactsTestFactory(t, server.URL)
	_, err := findArtifact(context.Background(), f, "acme", "monolith", "429", "missing", "")
	if err == nil {
		t.Fatal("findArtifact() expected error, got nil")
	}
	if !errors.Is(err, bkErrors.ErrResourceNotFound) {
		t.Fatalf("findArtifact() error = %v, want ErrResourceNotFound", err)
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("findArtifact() error = %v, want to mention artifact ID", err)
	}
}

func TestDownloadArtifactUsesArtifactPathAsDest(t *testing.T) {
	// No t.Parallel(): t.Chdir is incompatible with parallel tests.
	const body = "hello"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	// Run from a temp cwd so the relative destPath lands somewhere isolated.
	t.Chdir(t.TempDir())

	f := newArtifactsTestFactory(t, server.URL)
	art := &buildkite.Artifact{Path: "logs/rspec.json", DownloadURL: server.URL}

	dest, err := downloadArtifact(context.Background(), f, art)
	if err != nil {
		t.Fatalf("downloadArtifact() error = %v", err)
	}
	if dest != filepath.FromSlash("logs/rspec.json") {
		t.Fatalf("dest = %q, want logs/rspec.json (OS-adjusted)", dest)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != body {
		t.Fatalf("file contents = %q, want %q", got, body)
	}
}

func TestDownloadCmdValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmd     DownloadCmd
		wantErr bool
	}{
		{"no artifact ID, no filters", DownloadCmd{}, false},
		{"filters without artifact ID", DownloadCmd{Path: []string{"coverage/**", "log/*"}, State: "finished"}, false},
		{"artifact ID alone", DownloadCmd{ArtifactID: "art-1"}, false},
		{"artifact ID with job UUID (fast path)", DownloadCmd{ArtifactID: "art-1", JobUUID: "job-1"}, false},
		{"artifact ID with path rejected", DownloadCmd{ArtifactID: "art-1", Path: []string{"coverage/**"}}, true},
		{"artifact ID with state rejected", DownloadCmd{ArtifactID: "art-1", State: "finished"}, true},
		{"artifact ID with both rejected", DownloadCmd{ArtifactID: "art-1", Path: []string{"coverage/**", "log/*"}, State: "finished"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.cmd.validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("validate() = nil, want error")
				}
				if !errors.Is(err, bkErrors.ErrValidation) {
					t.Fatalf("validate() error = %v, want ErrValidation", err)
				}
				if !strings.Contains(err.Error(), "--path and --state") {
					t.Errorf("validate() error = %v, want to mention --path and --state", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate() = %v, want nil", err)
			}
		})
	}
}

func TestDownloadCmdValidateRejectsUnknownState(t *testing.T) {
	t.Parallel()

	cmd := DownloadCmd{State: "finshed"}
	err := cmd.validate()
	if err == nil {
		t.Fatal("validate() = nil, want validation error for unknown state")
	}
	if !errors.Is(err, bkErrors.ErrValidation) {
		t.Fatalf("validate() error = %v, want ErrValidation", err)
	}
	if !strings.Contains(err.Error(), "finshed") {
		t.Errorf("validate() error %q should mention the rejected input", err)
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
		"art-123",
		"--build", "429",
		"-p", "monolith",
		"--job-uuid", "job-uuid-1",
		"--path", "coverage/**",
		"--path", "log/a,b.json",
		"--state", "Finished",
	}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cmd.ArtifactID != "art-123" {
		t.Errorf("ArtifactID = %q, want art-123", cmd.ArtifactID)
	}
	if cmd.BuildNumber != "429" {
		t.Errorf("BuildNumber = %q, want 429", cmd.BuildNumber)
	}
	if cmd.Pipeline != "monolith" {
		t.Errorf("Pipeline = %q, want monolith", cmd.Pipeline)
	}
	if cmd.JobUUID != "job-uuid-1" {
		t.Errorf("JobUUID = %q, want job-uuid-1", cmd.JobUUID)
	}
	if want := []string{"coverage/**", "log/a,b.json"}; !slices.Equal(cmd.Path, want) {
		t.Errorf("Path = %q, want %q", cmd.Path, want)
	}
	if cmd.State != "Finished" {
		t.Errorf("State = %q, want Finished (parser preserves casing)", cmd.State)
	}
}

func TestDownloadCmdHelpMentionsFilters(t *testing.T) {
	t.Parallel()

	var cmd DownloadCmd
	help := cmd.Help()
	for _, want := range []string{"--path", "--state", "log/rspec*.json", "bk artifacts list"} {
		if !strings.Contains(help, want) {
			t.Errorf("Help() missing %q", want)
		}
	}
}

func TestDownloadAllMultiplePaths(t *testing.T) {
	t.Chdir(t.TempDir())
	var server *httptest.Server
	downloads := make(map[string]int)
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/files/") {
			downloads[r.URL.Path]++
			fmt.Fprint(w, r.URL.Path)
			return
		}
		a := buildkite.Artifact{ID: "rspec", Path: "log/rspec1.json", DownloadURL: server.URL + "/files/rspec"}
		b := buildkite.Artifact{ID: "minitest", Path: "log/minitest.log", DownloadURL: server.URL + "/files/minitest"}
		switch r.URL.Query().Get("path") {
		case "log/rspec*.json":
			writeArtifactsPage(t, w, []buildkite.Artifact{a}, "")
		case "log/*":
			writeArtifactsPage(t, w, []buildkite.Artifact{a, b}, "")
		default:
			t.Errorf("unexpected path filter: %s", r.URL)
			writeArtifactsPage(t, w, nil, "")
		}
	}))
	t.Cleanup(server.Close)
	var cmd DownloadCmd
	parser, err := kong.New(&cmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"--path", "log/rspec*.json", "--path", "log/*"}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.downloadAll(context.Background(), newArtifactsTestFactory(t, server.URL), "acme", "monolith", "429"); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{"log/rspec1.json": "/files/rspec", "log/minitest.log": "/files/minitest"} {
		got, err := os.ReadFile(filepath.Join("artifacts-build-429", path))
		if err != nil || string(got) != body {
			t.Errorf("download %s = %q, %v; want %q", path, got, err, body)
		}
		if downloads[body] != 1 {
			t.Errorf("downloads of %s = %d, want 1", body, downloads[body])
		}
	}
}
