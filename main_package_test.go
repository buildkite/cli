package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPackageRegistryUpdate(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "default-org")
	t.Setenv("BUILDKITE_API_TOKEN", "test-token")
	t.Setenv("BK_TELEMETRY", "false")

	for _, tt := range []struct {
		name, visibility, body, wantError string
		status                            int
	}{
		{name: "public", visibility: "public", body: `{"slug":"my-registry","public":true}`, status: http.StatusOK},
		{name: "private", visibility: "private", body: `{"slug":"my-registry","public":false}`, status: http.StatusOK},
		{name: "visibility ignored", visibility: "public", body: `{"slug":"my-registry","public":false}`, wantError: "requested public visibility, but the API reports private", status: http.StatusOK},
		{name: "forbidden", visibility: "public", body: `{"message":"Registry edit permission required"}`, wantError: "Registry edit permission required", status: http.StatusForbidden},
		{name: "missing visibility"},
		{name: "invalid visibility", visibility: "internal"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if tt.status == 0 {
					t.Errorf("unexpected request for invalid arguments: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if r.Method != http.MethodPatch || r.URL.RequestURI() != "/v2/packages/organizations/target-org/registries/my-registry" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				if r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("missing expected authorization or JSON content type")
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if got, want := strings.TrimSpace(string(body)), `{"visibility":"`+tt.visibility+`"}`; got != want {
					t.Errorf("request body = %s, want %s", got, want)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)

			args := []string{"package", "registry", "update", "my-registry", "--org", "target-org", "--json", "--no-input"}
			if tt.visibility != "" {
				args = append(args, "--visibility", tt.visibility)
			}
			code, stdout, stderr := runCLI(t, args...)
			switch {
			case tt.status == 0:
				if code == 0 || requests != 0 || !strings.Contains(stderr, "--visibility") {
					t.Fatalf("exit=%d requests=%d stderr=%q", code, requests, stderr)
				}
			case tt.wantError != "":
				if code == 0 || requests != 1 || stdout != "" || !strings.Contains(stderr, tt.wantError) {
					t.Fatalf("exit=%d requests=%d stdout=%q stderr=%q", code, requests, stdout, stderr)
				}
			default:
				var registry struct {
					Slug   string `json:"slug"`
					Public *bool  `json:"public"`
				}
				err := json.Unmarshal([]byte(stdout), &registry)
				if code != 0 || requests != 1 || err != nil || registry.Slug != "my-registry" || registry.Public == nil || *registry.Public != (tt.visibility == "public") {
					t.Fatalf("exit=%d requests=%d stdout=%q stderr=%q decode error=%v", code, requests, stdout, stderr, err)
				}
			}
		})
	}
}
