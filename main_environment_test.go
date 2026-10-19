package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/pkg/keyring"
	"gopkg.in/yaml.v3"
)

func TestJobEnvironment(t *testing.T) {
	const jobID = "0190046e-e199-453b-a302-a21a4d649d31"
	const values = `{"env":{"Z_VALUE":"synthetic-only-secret","EMPTY":"","MULTILINE":"one\ntwo\t\"quoted\"\\path"}}`
	for _, tt := range []struct {
		name, config, body, format, wantError string
		flags                                 []string
		status                                int
		missingToken, missingOrg, storedToken bool
		redirect                              bool
	}{
		{name: "default JSON", body: values, format: "json"},
		{name: "JSON overrides config", body: values, config: "text", flags: []string{"--json"}, format: "json"},
		{name: "YAML", body: values, flags: []string{"--yaml"}, format: "yaml"},
		{name: "text", body: values, flags: []string{"--text"}, format: "text"},
		{name: "configured YAML", body: values, config: "yaml", format: "yaml"},
		{name: "output flag", body: values, config: "text", flags: []string{"-o", "json"}, format: "json"},
		{name: "empty", body: `{"env":{}}`, format: "json"},
		{name: "null environment", body: `{"env":null}`, format: "json"},
		{name: "empty text", body: `{"env":{}}`, flags: []string{"--text"}, format: "text"},
		{name: "stored org credential", body: values, flags: []string{"--org", "override-org"}, storedToken: true, format: "json"},
		{name: "missing token", missingToken: true, wantError: "bk auth login"},
		{name: "missing organization", missingOrg: true, wantError: "no organization configured"},
		{name: "unauthorized", status: 401, body: `{"message":"synthetic-only-secret"}`, wantError: "HTTP 401"},
		{name: "scope denied", status: 403, body: `{"message":"synthetic-only-secret"}`, wantError: "read_job_env"},
		{name: "not found", status: 404, body: `{"message":"synthetic-only-secret"}`, wantError: "HTTP 404"},
		{name: "malformed error body", status: 422, body: `synthetic-only-secret`, wantError: "HTTP 422"},
		{name: "malformed success body", body: `{"env":{"VALUE":"synthetic-only-secret"`, wantError: "response was invalid"},
		{name: "wrong value type", body: `{"env":{"VALUE":987654321012345}}`, wantError: "response was invalid"},
		{name: "redirected success", redirect: true, body: values, format: "json"},
		{name: "redirected malformed response", redirect: true, body: `synthetic-only-secret`, wantError: "response was invalid"},
		{name: "redirected error", redirect: true, status: 403, body: `{"message":"synthetic-only-secret"}`, wantError: "HTTP 403"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "fixture-org")
			t.Setenv("BUILDKITE_API_TOKEN", "synthetic-auth-token")
			t.Setenv("BUILDKITE_OUTPUT_FORMAT", tt.config)
			t.Setenv("BK_TELEMETRY", "false")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			keyring.MockForTesting()
			t.Cleanup(keyring.ResetForTesting)
			org := "fixture-org"
			if tt.missingOrg {
				t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
			}
			if tt.missingToken || tt.storedToken {
				t.Setenv("BUILDKITE_API_TOKEN", "")
			}
			if tt.storedToken {
				org = "override-org"
				if err := keyring.New().Set(org, "synthetic-auth-token"); err != nil {
					t.Fatal(err)
				}
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				path := "/v2/organizations/" + org + "/jobs/" + jobID + "/env"
				if r.Method != http.MethodGet || (r.URL.Path != path && (!tt.redirect || r.URL.Path != "/environment-data")) {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer synthetic-auth-token" || r.Header.Get("Accept") != "application/json" {
					t.Error("missing authentication or JSON Accept header")
				}
				if tt.redirect && r.URL.Path == path {
					http.Redirect(w, r, "/environment-data", http.StatusFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			args := []string{"job", "environment", jobID, "--debug", "--quiet", "--no-input"}
			code, stdout, stderr := runCLI(t, append(args, tt.flags...)...)
			wantRequests := 1
			if tt.redirect {
				wantRequests = 2
			}
			if tt.missingToken || tt.missingOrg {
				wantRequests = 0
			}
			if requests != wantRequests {
				t.Fatalf("requests = %d, want %d", requests, wantRequests)
			}
			for _, marker := range []string{"synthetic-only-secret", "synthetic-auth-token", "987654321012345"} {
				if strings.Contains(stderr, marker) {
					t.Fatalf("stderr leaked %q: %s", marker, stderr)
				}
			}
			if tt.wantError != "" {
				if code != 1 || stdout != "" || !strings.Contains(stderr, tt.wantError) {
					t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
				}
				return
			}
			if code != 0 || !strings.Contains(stderr, "[response body omitted]") {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if tt.format == "text" {
				want := ""
				if tt.body == values {
					want = "EMPTY=\"\"\nMULTILINE=\"one\\ntwo\\t\\\"quoted\\\"\\\\path\"\nZ_VALUE=\"synthetic-only-secret\"\n"
				}
				if stdout != want {
					t.Fatalf("text=%q, want %q", stdout, want)
				}
				return
			}
			var got map[string]map[string]string
			var err error
			if tt.format == "yaml" {
				err = yaml.Unmarshal([]byte(stdout), &got)
			} else {
				err = json.Unmarshal([]byte(stdout), &got)
			}
			want := map[string]map[string]string{"env": {}}
			if tt.body == values {
				want["env"] = map[string]string{"Z_VALUE": "synthetic-only-secret", "EMPTY": "", "MULTILINE": "one\ntwo\t\"quoted\"\\path"}
			}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("output=%q decode error=%v; want %v", stdout, err, want)
			}
		})
	}
}
