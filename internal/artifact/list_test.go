package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v5"
)

func newTestClient(t *testing.T, serverURL string) *buildkite.Client {
	t.Helper()
	client, err := buildkite.NewOpts(buildkite.WithBaseURL(serverURL))
	if err != nil {
		t.Fatalf("new buildkite client: %v", err)
	}
	return client
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

func TestListHitsBuildEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/v2/organizations/acme/pipelines/monolith/builds/429/artifacts"
		if r.URL.Path != wantPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, wantPath)
		}
		writeArtifactsPage(t, w, []buildkite.Artifact{{ID: "a1", Path: "coverage.xml"}}, "")
	}))
	t.Cleanup(server.Close)

	got, err := List(context.Background(), newTestClient(t, server.URL), "acme", "monolith", "429", "", "", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "a1" {
		t.Fatalf("List() = %+v, want single artifact a1", got)
	}
}

func TestListHitsJobEndpoint(t *testing.T) {
	t.Parallel()

	const jobUUID = "0193903e-ecd9-4c51-9156-0738da987e87"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := fmt.Sprintf("/v2/organizations/acme/pipelines/monolith/builds/429/jobs/%s/artifacts", jobUUID)
		if r.URL.Path != wantPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, wantPath)
		}
		writeArtifactsPage(t, w, []buildkite.Artifact{{ID: "a1"}}, "")
	}))
	t.Cleanup(server.Close)

	if _, err := List(context.Background(), newTestClient(t, server.URL), "acme", "monolith", "429", jobUUID, "", ""); err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

func TestListPassesFilters(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("path"); got != "coverage/**" {
			t.Fatalf("path = %q, want coverage/**", got)
		}
		if got := q.Get("state"); got != "finished" {
			t.Fatalf("state = %q, want finished", got)
		}
		if got := q.Get("per_page"); got != "100" {
			t.Fatalf("per_page = %q, want 100", got)
		}
		writeArtifactsPage(t, w, []buildkite.Artifact{}, "")
	}))
	t.Cleanup(server.Close)

	if _, err := List(context.Background(), newTestClient(t, server.URL), "acme", "monolith", "429", "", "coverage/**", "finished"); err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

func TestListLowercasesState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("state"); got != "finished" {
			t.Fatalf("state = %q, want finished (lower-cased)", got)
		}
		writeArtifactsPage(t, w, []buildkite.Artifact{}, "")
	}))
	t.Cleanup(server.Close)

	if _, err := List(context.Background(), newTestClient(t, server.URL), "acme", "monolith", "429", "", "", "Finished"); err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

func TestListPassesFiltersOnJobEndpoint(t *testing.T) {
	t.Parallel()

	const jobUUID = "0193903e-ecd9-4c51-9156-0738da987e87"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := fmt.Sprintf("/v2/organizations/acme/pipelines/monolith/builds/429/jobs/%s/artifacts", jobUUID)
		if r.URL.Path != wantPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, wantPath)
		}
		q := r.URL.Query()
		if got := q.Get("path"); got != "log/rspec*.json" {
			t.Fatalf("path = %q, want log/rspec*.json", got)
		}
		if got := q.Get("state"); got != "finished" {
			t.Fatalf("state = %q, want finished", got)
		}
		writeArtifactsPage(t, w, []buildkite.Artifact{}, "")
	}))
	t.Cleanup(server.Close)

	if _, err := List(context.Background(), newTestClient(t, server.URL), "acme", "monolith", "429", jobUUID, "log/rspec*.json", "finished"); err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

func TestListPaginates(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	var calls int
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			next := server.URL + r.URL.Path + "?page=2&per_page=100"
			writeArtifactsPage(t, w, []buildkite.Artifact{{ID: "a1"}, {ID: "a2"}}, next)
		case "2":
			next := server.URL + r.URL.Path + "?page=3&per_page=100"
			writeArtifactsPage(t, w, []buildkite.Artifact{{ID: "a3"}}, next)
		case "3":
			writeArtifactsPage(t, w, []buildkite.Artifact{{ID: "a4"}}, "")
		default:
			t.Fatalf("unexpected page = %q", page)
		}
	}))
	t.Cleanup(server.Close)

	got, err := List(context.Background(), newTestClient(t, server.URL), "acme", "monolith", "429", "", "", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	wantIDs := []string{"a1", "a2", "a3", "a4"}
	if len(got) != len(wantIDs) {
		t.Fatalf("got %d artifacts, want %d", len(got), len(wantIDs))
	}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Fatalf("artifact %d ID = %q, want %q", i, got[i].ID, id)
		}
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestListPropagatesError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	if _, err := List(context.Background(), newTestClient(t, server.URL), "acme", "monolith", "429", "", "", ""); err == nil {
		t.Fatal("List() expected error, got nil")
	}
}
