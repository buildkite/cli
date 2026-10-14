package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/goccy/go-yaml"
)

const (
	testOrg          = "acme"
	testClusterUUID  = "019a1dd3-3870-7e50-8a6d-f16efba96342"
	testRegistryUUID = "019a1dd8-a49b-70c5-99b8-b8f9f0ed6824"
)

func testFactory(t *testing.T, handler http.Handler) (*factory.Factory, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := buildkite.NewOpts(buildkite.WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	return &factory.Factory{RestAPIClient: client, Quiet: true, NoInput: true}, server
}

func registryPath(registryUUID string) string {
	path := "/v2/organizations/" + testOrg + "/clusters/" + testClusterUUID + "/cache-registries"
	if registryUUID != "" {
		path += "/" + registryUUID
	}
	return path
}

func TestReadPolicy(t *testing.T) {
	t.Run("normalizes YAML object from stdin", func(t *testing.T) {
		policy, err := readPolicy("-", strings.NewReader("save:\n  scopes:\n    branch: true\nrules:\n  - effect: allow\n"))
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(policy)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(encoded), `{"rules":[{"effect":"allow"}],"save":{"scopes":{"branch":true}}}`; got != want {
			t.Fatalf("policy = %s, want %s", got, want)
		}
	})

	t.Run("reads JSON object from file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "policy.json")
		if err := os.WriteFile(path, []byte(`{"restore":{"scopes":[{}]}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		policy, err := readPolicy(path, strings.NewReader("ignored"))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := policy["restore"]; !ok {
			t.Fatalf("policy = %#v", policy)
		}
	})

	for _, tt := range []struct {
		name, input, want string
	}{
		{"empty", " \n", "cannot be empty"},
		{"malformed", "save: [", "parsing cache registry policy"},
		{"array root", "- allow\n- deny\n", "must be an object"},
		{"null root", "null\n", "must be an object"},
	} {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			_, err := readPolicy("-", strings.NewReader(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestListCmdValidation(t *testing.T) {
	for _, tt := range []struct {
		perPage int
		wantErr bool
	}{
		{perPage: 0, wantErr: true},
		{perPage: 1},
		{perPage: 100},
		{perPage: 101, wantErr: true},
	} {
		t.Run(fmt.Sprintf("per-page %d", tt.perPage), func(t *testing.T) {
			err := (&ListCmd{PerPage: tt.perPage, Limit: 100}).Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestCommandHelpCallsOutPolicyAndSlugRisks(t *testing.T) {
	if help := (&CreateCmd{}).Help(); !strings.Contains(help, "default unrestricted") || !strings.Contains(help, "builds you trust") {
		t.Fatalf("create help does not explain the default policy risk:\n%s", help)
	}
	if help := (&UpdateCmd{}).Help(); !strings.Contains(help, "regenerates the registry slug") || !strings.Contains(help, "old slug") ||
		!strings.Contains(help, "denies all cache saves and restores") || !strings.Contains(help, "does not reset") {
		t.Fatalf("update help does not explain the rename and clear-policy risks:\n%s", help)
	}
}

func TestListCmdPaginationAndOutput(t *testing.T) {
	requests := 0
	var server *httptest.Server
	f, server := testFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != registryPath("") {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch requests {
		case 1:
			if got := r.URL.Query().Get("per_page"); got != "2" {
				t.Errorf("first per_page = %q", got)
			}
			fmt.Fprintf(w, `{"items":[{"uuid":"one","slug":"one","name":"One"},{"uuid":"two","slug":"two","name":"Two"}],"links":{"next":%q}}`, server.URL+registryPath("")+"?after=cursor-1&per_page=2")
		case 2:
			if r.URL.Query().Get("after") != "cursor-1" || r.URL.Query().Get("per_page") != "1" {
				t.Errorf("second query = %q", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"items":[{"uuid":"three","slug":"three","name":"Three"},{"uuid":"four","slug":"four","name":"Four"}],"links":{}}`)
		default:
			t.Fatalf("unexpected request %d", requests)
		}
	}))

	cmd := ListCmd{ClusterUUID: testClusterUUID, PerPage: 2, Limit: 3}
	var stdout bytes.Buffer
	if err := cmd.run(context.Background(), f, testOrg, &stdout, output.FormatJSON); err != nil {
		t.Fatal(err)
	}
	var registries []buildkite.CacheRegistry
	if err := json.Unmarshal(stdout.Bytes(), &registries); err != nil {
		t.Fatal(err)
	}
	if len(registries) != 3 || registries[2].UUID != "three" || requests != 2 {
		t.Fatalf("registries = %#v, requests = %d", registries, requests)
	}
}

func TestListCmdRejectsRepeatedCursor(t *testing.T) {
	requests := 0
	var server *httptest.Server
	f, server := testFactory(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		fmt.Fprintf(w, `{"items":[{"uuid":%q}],"links":{"next":%q}}`, fmt.Sprintf("registry-%d", requests), server.URL+registryPath("")+"?after=same-cursor")
	}))
	cmd := ListCmd{ClusterUUID: testClusterUUID, PerPage: 2, Limit: 10}
	_, err := cmd.fetch(context.Background(), f, testOrg)
	if err == nil || !strings.Contains(err.Error(), "repeated cache registries cursor") {
		t.Fatalf("error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestListCmdTextOutput(t *testing.T) {
	description := "Shared dependencies"
	f, _ := testFactory(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"items":[{"uuid":%q,"slug":"ruby-gems","name":"Ruby gems","description":"Shared dependencies"}],"links":{}}`, testRegistryUUID)
	}))
	cmd := ListCmd{ClusterUUID: testClusterUUID, PerPage: 30, Limit: 100}
	var stdout bytes.Buffer
	if err := cmd.run(context.Background(), f, testOrg, &stdout, output.FormatText); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SLUG", "NAME", "DESCRIPTION", "UUIDs:", "ruby-gems", description, testRegistryUUID} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("text output missing %q:\n%s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), testRegistryUUID[:28]+"...") {
		t.Fatalf("UUID was truncated from text output:\n%s", stdout.String())
	}
}

func TestViewCmdRequestAndYAMLOutput(t *testing.T) {
	f, _ := testFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != registryPath(testRegistryUUID) {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprintf(w, `{"uuid":%q,"slug":"ruby-gems","name":"Ruby gems","policy":{"save":{"enabled":true}}}`, testRegistryUUID)
	}))
	cmd := ViewCmd{ClusterUUID: testClusterUUID, RegistryUUID: testRegistryUUID}
	var stdout bytes.Buffer
	if err := cmd.run(context.Background(), f, testOrg, &stdout, output.FormatYAML); err != nil {
		t.Fatal(err)
	}
	var registry map[string]any
	if err := yaml.Unmarshal(stdout.Bytes(), &registry); err != nil {
		t.Fatal(err)
	}
	if registry["uuid"] != testRegistryUUID || registry["slug"] != "ruby-gems" {
		t.Fatalf("output = %#v", registry)
	}

	stdout.Reset()
	if err := cmd.run(context.Background(), f, testOrg, &stdout, output.FormatText); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Cache Registry: ruby-gems", "UUID", testRegistryUUID, "Policy", `"save"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("text output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRenderRegistryTextDoesNotTruncatePolicy(t *testing.T) {
	longValue := strings.Repeat("scope-value-", 20) + "tail-marker"
	text := renderRegistryText(buildkite.CacheRegistry{
		Slug: "large-policy",
		Policy: buildkite.CacheRegistryPolicy{
			"restore": map[string]any{
				"scopes": []any{map[string]any{"branch": longValue}},
			},
		},
	})
	if !strings.Contains(text, "Policy:\n\n{") || !strings.Contains(text, longValue) {
		t.Fatalf("policy was truncated from text output:\n%s", text)
	}
}

func TestRenderRegistryTextShowsNullPolicy(t *testing.T) {
	text := renderRegistryText(buildkite.CacheRegistry{Slug: "no-cache"})
	if !strings.Contains(text, "Policy:\n\nnull\n") {
		t.Fatalf("null policy is not visible in text output:\n%s", text)
	}
}

func TestCreateCmdRequestAndOutput(t *testing.T) {
	f, _ := testFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != registryPath("") {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["name"] != "Ruby gems" || body["description"] != "Shared gems" {
			t.Errorf("body = %#v", body)
		}
		policy, ok := body["policy"].(map[string]any)
		if !ok || policy["save"] == nil {
			t.Errorf("policy = %#v", body["policy"])
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"uuid":%q,"slug":"ruby-gems","name":"Ruby gems"}`, testRegistryUUID)
	}))
	description := "Shared gems"
	cmd := CreateCmd{ClusterUUID: testClusterUUID, Name: "Ruby gems", Description: &description, PolicyFile: "-"}
	var stdout bytes.Buffer
	if err := cmd.run(context.Background(), f, testOrg, strings.NewReader("save:\n  enabled: true\n"), &stdout, output.FormatJSON); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), testRegistryUUID) {
		t.Fatalf("output = %s", stdout.String())
	}
}

func TestUpdateCmdValidationAndRequest(t *testing.T) {
	value := "value"
	for _, tt := range []struct {
		name string
		cmd  UpdateCmd
		want string
	}{
		{"requires mutation", UpdateCmd{}, "at least one"},
		{"description conflict", UpdateCmd{Description: &value, ClearDescription: true}, "cannot be used together"},
		{"emoji conflict", UpdateCmd{Emoji: &value, ClearEmoji: true}, "cannot be used together"},
		{"color conflict", UpdateCmd{Color: &value, ClearColor: true}, "cannot be used together"},
		{"policy conflict", UpdateCmd{PolicyFile: "policy.yml", ClearPolicy: true}, "cannot be used together"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	f, _ := testFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != registryPath(testRegistryUUID) {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["name"] != "Packages" || body["description"] != nil || body["policy"] != nil {
			t.Errorf("body = %#v", body)
		}
		if _, exists := body["emoji"]; exists {
			t.Errorf("body unexpectedly included emoji: %#v", body)
		}
		fmt.Fprintf(w, `{"uuid":%q,"slug":"packages","name":"Packages"}`, testRegistryUUID)
	}))
	name := "Packages"
	cmd := UpdateCmd{
		ClusterUUID:      testClusterUUID,
		RegistryUUID:     testRegistryUUID,
		Name:             &name,
		ClearDescription: true,
		ClearPolicy:      true,
	}
	var stdout bytes.Buffer
	if err := cmd.run(context.Background(), f, testOrg, strings.NewReader(""), &stdout, output.FormatJSON); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteCmdConfirmationAndRequest(t *testing.T) {
	requests := 0
	f, _ := testFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodDelete || r.URL.Path != registryPath(testRegistryUUID) {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	cmd := DeleteCmd{ClusterUUID: testClusterUUID, RegistryUUID: testRegistryUUID}
	var stderr bytes.Buffer

	input, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inputWriter.WriteString("n\n"); err != nil {
		t.Fatal(err)
	}
	if err := inputWriter.Close(); err != nil {
		t.Fatal(err)
	}
	originalStdin := os.Stdin
	os.Stdin = input
	t.Cleanup(func() { os.Stdin = originalStdin })
	f.NoInput = false
	if err := cmd.run(context.Background(), f, testOrg, &stderr); err != nil {
		t.Fatal(err)
	}
	os.Stdin = originalStdin
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	if requests != 0 || !strings.Contains(stderr.String(), "Deletion cancelled") {
		t.Fatalf("requests = %d, stderr = %q", requests, stderr.String())
	}

	f.NoInput = true
	err = cmd.run(context.Background(), f, testOrg, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--no-input") || requests != 0 {
		t.Fatalf("error = %v, requests = %d", err, requests)
	}

	f.SkipConfirm = true
	if err := cmd.run(context.Background(), f, testOrg, &stderr); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || !strings.Contains(stderr.String(), "deleted successfully") {
		t.Fatalf("requests = %d, stderr = %q", requests, stderr.String())
	}
}

func TestDeleteCmdSurfacesDefaultRegistryError(t *testing.T) {
	f, _ := testFactory(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"message":"Cannot destroy default cache registry"}`)
	}))
	f.SkipConfirm = true
	cmd := DeleteCmd{ClusterUUID: testClusterUUID, RegistryUUID: testRegistryUUID}
	err := cmd.run(context.Background(), f, testOrg, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "error deleting cache registry") || !strings.Contains(err.Error(), "Cannot destroy default cache registry") {
		t.Fatalf("error = %v", err)
	}
}

func TestViewCmdSurfacesAPIError(t *testing.T) {
	f, _ := testFactory(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"cache registry access denied"}`)
	}))
	cmd := ViewCmd{ClusterUUID: testClusterUUID, RegistryUUID: testRegistryUUID}
	err := cmd.run(context.Background(), f, testOrg, io.Discard, output.FormatJSON)
	if err == nil || !strings.Contains(err.Error(), "error loading cache registry") || !strings.Contains(err.Error(), "cache registry access denied") {
		t.Fatalf("error = %v", err)
	}
}
