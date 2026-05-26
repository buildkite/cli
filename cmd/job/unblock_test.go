package job

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
)

func TestParseUnblockFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    map[string]any
		wantErr bool
	}{
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
		{
			name:  "string field",
			input: `{"release":"v1.2.3"}`,
			want: map[string]any{
				"release": "v1.2.3",
			},
		},
		{
			name:  "nested field",
			input: `{"payload":{"confirm":true},"targets":["staging","production"]}`,
			want: map[string]any{
				"payload": map[string]any{
					"confirm": true,
				},
				"targets": []any{"staging", "production"},
			},
		},
		{
			name:    "array is invalid",
			input:   `["staging"]`,
			wantErr: true,
		},
		{
			name:    "null is invalid",
			input:   `null`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   `{`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseUnblockFields(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseUnblockFields() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			gotJSON, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal got: %v", err)
			}
			wantJSON, err := json.Marshal(tt.want)
			if err != nil {
				t.Fatalf("marshal want: %v", err)
			}
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("parseUnblockFields() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestUnblockJobUsesRESTEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/v2/organizations/buildkite/jobs/job-1/unblock" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		var body struct {
			Fields map[string]any `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Fields["release"] != "v1.2.3" {
			t.Fatalf("fields = %#v", body.Fields)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"job-1","state":"unblocked","web_url":"https://buildkite.com/buildkite/cli/builds/42#job-1"}`))
	}))
	defer server.Close()

	client, err := buildkite.NewOpts(
		buildkite.WithBaseURL(server.URL),
		buildkite.WithTokenAuth("test-token"),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	job, err := unblockJob(context.Background(), client, "buildkite", "job-1", map[string]any{"release": "v1.2.3"})
	if err != nil {
		t.Fatalf("unblockJob() error = %v", err)
	}
	if job.ID != "job-1" || job.State != "unblocked" {
		t.Fatalf("job = %#v", job)
	}
}

func TestGetJobLogUsesOrganizationEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v2/organizations/buildkite/jobs/job-1/log" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("Accept = %q", r.Header.Get("Accept"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":"hello log","size":9}`))
	}))
	defer server.Close()

	client, err := buildkite.NewOpts(
		buildkite.WithBaseURL(server.URL),
		buildkite.WithTokenAuth("test-token"),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	log, err := getJobLog(context.Background(), client, "buildkite", "job-1")
	if err != nil {
		t.Fatalf("getJobLog() error = %v", err)
	}
	if log.Content != "hello log" || log.Size != 9 {
		t.Fatalf("log = %#v", log)
	}
}

func TestReprioritizeJobUsesOrganizationEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/v2/organizations/buildkite/jobs/job-1/reprioritize" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		var body struct {
			Priority int `json:"priority"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Priority != 10 {
			t.Fatalf("priority = %d", body.Priority)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"job-1","state":"scheduled","web_url":"https://buildkite.com/buildkite/cli/builds/42#job-1"}`))
	}))
	defer server.Close()

	client, err := buildkite.NewOpts(
		buildkite.WithBaseURL(server.URL),
		buildkite.WithTokenAuth("test-token"),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	job, err := reprioritizeJob(context.Background(), client, "buildkite", "job-1", 10)
	if err != nil {
		t.Fatalf("reprioritizeJob() error = %v", err)
	}
	if job.ID != "job-1" {
		t.Fatalf("job = %#v", job)
	}
}
