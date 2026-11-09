package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

func TestFindQueue(t *testing.T) {
	t.Parallel()
	const queues = `[
		{"id":"q-z","key":"zulu","hosted":false},
		{"id":"q-default","key":"default","hosted":false},
		{"id":"q-cloud","key":"cloud","hosted":true},
		{"id":"q-build","key":"build","hosted":false}
	]`
	for _, tt := range []struct {
		name, key, defaultID, queues, want, wantErr string
		status                                      int
	}{
		{name: "explicit key", key: "build", want: "build"},
		{name: "configured default not literal default", defaultID: "q-build", want: "build"},
		{name: "explicit overrides hosted default", key: "build", defaultID: "q-cloud", want: "build"},
		{name: "explicit literal default", key: "default", want: "default"},
		{name: "missing key", key: "missing", wantErr: `no queue with key "missing" found`},
		{name: "exact case", key: "Build", wantErr: `no queue with key "Build" found`},
		{name: "does not match ID", key: "q-build", wantErr: `no queue with key "q-build" found`},
		{name: "does not trim key", key: " build ", wantErr: `no queue with key " build " found`},
		{name: "explicit hosted", key: "cloud", wantErr: `queue "cloud" is not a self-hosted queue`},
		{name: "hosted default", defaultID: "q-cloud", wantErr: `queue "cloud" is not a self-hosted queue`},
		{name: "no default", wantErr: "no default queue is configured"},
		{name: "inaccessible default", defaultID: "q-other", wantErr: `configured default queue "q-other" was not found`},
		{name: "no queues", queues: `[]`, wantErr: "no default queue is configured"},
		{name: "only hosted", key: "cloud", queues: `[{"id":"q-cloud","key":"cloud","hosted":true}]`, wantErr: "is not a self-hosted queue"},
		{name: "unknown hosting", key: "unknown", queues: `[{"id":"q-unknown","key":"unknown"}]`, wantErr: "is not a self-hosted queue"},
		{name: "default API failure", status: http.StatusForbidden, wantErr: "getting cluster"},
		{name: "queues API failure", key: "build", status: http.StatusForbidden, wantErr: "listing queues"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clusterRequests := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method != http.MethodGet {
					t.Errorf("unexpected method %s", r.Method)
				}
				if tt.status != 0 {
					w.WriteHeader(tt.status)
					fmt.Fprint(w, `{"message":"Forbidden"}`)
					return
				}
				switch r.URL.Path {
				case "/v2/organizations/test-org/clusters/cluster-1":
					clusterRequests++
					fmt.Fprintf(w, `{"id":"cluster-1","default_queue_id":%q}`, tt.defaultID)
				case "/v2/organizations/test-org/clusters/cluster-1/queues":
					if tt.queues != "" {
						fmt.Fprint(w, tt.queues)
					} else {
						fmt.Fprint(w, queues)
					}
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer s.Close()
			client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
			if err != nil {
				t.Fatal(err)
			}
			got, err := FindQueue(context.Background(), &factory.Factory{RestAPIClient: client}, "test-org", "cluster-1", tt.key)
			if tt.wantErr == "" {
				if err != nil || got != tt.want {
					t.Fatalf("FindQueue() = %q, %v; want %q", got, err, tt.want)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				if tt.status == 0 {
					message := err.Error()
					if !strings.Contains(message, "--queue <key>") || !strings.Contains(message, `cluster "cluster-1"`) {
						t.Errorf("missing retry/cluster context: %s", message)
					}
					if tt.queues == "" && !strings.Contains(message, "Available self-hosted queue keys:\n  build\n  default\n  zulu") {
						t.Errorf("missing sorted self-hosted keys: %s", message)
					}
					if strings.Contains(message, "\n  cloud") || strings.Contains(message, "\n  unknown") {
						t.Errorf("listed an unverified self-hosted queue: %s", message)
					}
					if tt.queues != "" && !strings.Contains(message, "(none)") {
						t.Errorf("missing no self-hosted queues advice: %s", message)
					}
				}
			}
			if tt.key != "" && clusterRequests != 0 {
				t.Errorf("explicit key made %d cluster requests", clusterRequests)
			}
		})
	}
}

func TestFindQueuePagination(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"build", "", "missing"} {
		t.Run("key="+key, func(t *testing.T) {
			t.Parallel()
			queueRequests := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/cluster-1") {
					fmt.Fprint(w, `{"default_queue_id":"q-build"}`)
					return
				}
				queueRequests++
				if r.URL.Query().Get("per_page") != "100" {
					t.Errorf("unexpected page size: %s", r.URL.RawQuery)
				}
				switch r.URL.Query().Get("page") {
				case "1":
					w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2>; rel="next"`, r.Host, r.URL.Path))
					fmt.Fprint(w, `[{"id":"q-z","key":"zulu","hosted":false}]`)
				case "2":
					fmt.Fprint(w, `[{"id":"q-build","key":"build","hosted":false}]`)
				default:
					t.Errorf("unexpected page: %s", r.URL.RawQuery)
					fmt.Fprint(w, `[]`)
				}
			}))
			defer s.Close()
			client, err := buildkite.NewOpts(buildkite.WithBaseURL(s.URL))
			if err != nil {
				t.Fatal(err)
			}
			got, err := FindQueue(context.Background(), &factory.Factory{RestAPIClient: client}, "test-org", "cluster-1", key)
			if key == "missing" {
				if err == nil || !strings.Contains(err.Error(), "\n  build\n  zulu") {
					t.Fatalf("expected both pages in error: %v", err)
				}
			} else if err != nil || got != "build" {
				t.Fatalf("FindQueue() = %q, %v", got, err)
			}
			if queueRequests != 2 {
				t.Errorf("queue requests = %d, want 2", queueRequests)
			}
		})
	}
}
