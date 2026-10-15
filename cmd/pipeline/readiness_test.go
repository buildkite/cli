package pipeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/config"
	buildkite "github.com/buildkite/go-buildkite/v5"
	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"gopkg.in/yaml.v3"
)

type offlineSchemaTransport struct{ base http.RoundTripper }

func (t offlineSchemaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "raw.githubusercontent.com" {
		return nil, errors.New("offline schema fixture")
	}
	return t.base.RoundTrip(r)
}

func useOfflinePipelineSchema(t *testing.T) {
	t.Helper()
	previous := http.DefaultTransport
	http.DefaultTransport = offlineSchemaTransport{previous}
	t.Cleanup(func() { http.DefaultTransport = previous })
}

func TestCreateReadiness(t *testing.T) {
	useOfflinePipelineSchema(t)
	for _, tc := range []struct {
		name, file, wantErr, savedOrg string
		flags                         []string
		orgStatus                     int
		createStatus, wantMutations   int
		clusters, queues              int
		paused, busy                  bool
		missingCredentials            bool
		webhookFails                  bool
	}{
		{name: "preserve existing routing", file: "steps: []\nagents:\n  queue: old\n", clusters: 1, queues: 1},
		{name: "route generated steps", flags: []string{"--template", "go"}, clusters: 1, queues: 1},
		{name: "busy is not unavailable", file: "steps: []\n", clusters: 1, queues: 1, busy: true},
		{name: "missing explicit target credentials", flags: []string{"--template", "go"}, missingCredentials: true, wantErr: "no credentials for organization \"target\"; run bk auth login --org target"},
		{name: "rejected explicit target", flags: []string{"--template", "go"}, orgStatus: 401, wantErr: "credentials could not access organization \"target\""},
		{name: "forbidden explicit target", orgStatus: 403, wantErr: "credentials could not access organization \"target\""},
		{name: "no cluster", wantErr: "no matching cluster"},
		{name: "ambiguous cluster", clusters: 2, wantErr: "multiple clusters"},
		{name: "no queues", clusters: 1, wantErr: "no matching queue"},
		{name: "ambiguous queues", clusters: 1, queues: 2, wantErr: "multiple queues"},
		{name: "explicit flags", flags: []string{"--cluster-uuid", "cluster-2", "--queue", "chosen", "--template", "empty"}, clusters: 2, queues: 2},
		{name: "explicit unmatched queue", flags: []string{"--queue", "unavailable", "--template", "go"}, clusters: 1, queues: 1, wantErr: "no matching queue"},
		{name: "paused queue", flags: []string{"--template", "go"}, clusters: 1, queues: 1, paused: true, wantErr: "is paused"},
		{name: "missing local file", clusters: 1, queues: 1, wantErr: "no local pipeline file"},
		{name: "invalid YAML", file: "steps: [\n", clusters: 1, queues: 1, wantErr: "invalid YAML"},
		{name: "invalid schema", file: "steps: wrong\n", clusters: 1, queues: 1, wantErr: "pipeline validation failed"},
		{name: "offline wait and plugins", file: "steps:\n  - command: echo hello\n    plugins:\n      - docker#v5.13.0:\n          image: alpine\n  - wait\n", clusters: 1, queues: 1},
		{name: "dry-run no writes", flags: []string{"--dry-run", "--template", "go", "--remember-org", "--create-webhook"}, clusters: 1, queues: 1},
		{name: "remember only repository", flags: []string{"--remember-org", "--template", "empty"}, savedOrg: "original", clusters: 1, queues: 1},
		{name: "remember same repository", flags: []string{"--remember-org", "--template", "empty"}, savedOrg: "target", clusters: 1, queues: 1},
		{name: "failed create preserves repository preferences", file: "steps: []\n", flags: []string{"--remember-org"}, savedOrg: "original", clusters: 1, queues: 1, createStatus: 500, wantMutations: 1, wantErr: "create rejected"},
		{name: "remember after create despite webhook failure", file: "steps: []\n", flags: []string{"--remember-org", "--create-webhook"}, savedOrg: "original", clusters: 1, queues: 1, webhookFails: true, wantMutations: 2, wantErr: "pipeline created at https://buildkite.com/target/fixture but webhook creation failed"},
		{name: "named compatibility", flags: []string{"Named"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			repo, err := git.PlainInit(root, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{"git@github.com:acme/fixture.git"}}); err != nil {
				t.Fatal(err)
			}
			sub := filepath.Join(root, "src")
			os.Mkdir(sub, 0o755)
			t.Chdir(sub)
			configDir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", configDir)
			user := []byte("selected_org: original\norganizations:\n  original: {}\n")
			os.WriteFile(filepath.Join(configDir, "bk.yaml"), user, 0o600)
			t.Setenv("BUILDKITE_API_TOKEN", "fixture-token")
			if tc.missingCredentials {
				t.Setenv("BUILDKITE_API_TOKEN", "")
				// The saved default has credentials, but the explicit target does not.
				user = []byte("selected_org: original\norganizations:\n  original:\n    api_token: fixture-token\n")
				os.WriteFile(filepath.Join(configDir, "bk.yaml"), user, 0o600)
			}
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "original")
			t.Setenv("BUILDKITE_CREDENTIAL_STORE", "shm")
			t.Setenv("BUILDKITE_CREDENTIAL_STORE_PATH", filepath.Join(t.TempDir(), "credentials"))
			if tc.savedOrg != "" {
				os.WriteFile(filepath.Join(root, ".bk.yaml"), []byte("selected_org: "+tc.savedOrg+"\npipelines: [deploy]\noutput_format: yaml\nno_pager: true\n"), 0o600)
			}
			localBefore, _ := os.ReadFile(filepath.Join(root, ".bk.yaml"))
			existing := filepath.Join(root, "buildkite.yml")
			if tc.file != "" {
				os.WriteFile(existing, []byte(tc.file), 0o600)
			}
			var created buildkite.CreatePipeline
			mutations, verified := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method != http.MethodGet {
					mutations++
					if tc.webhookFails && r.URL.Path == "/graphql" {
						io.WriteString(w, `{"errors":[{"message":"webhook rejected"}]}`)
						return
					}
					if r.Method != http.MethodPost || r.URL.Path != "/v2/organizations/target/pipelines" {
						t.Errorf("unexpected mutation %s %s", r.Method, r.URL.Path)
					}
					local, _ := os.ReadFile(filepath.Join(root, ".bk.yaml"))
					if !bytes.Equal(localBefore, local) {
						t.Error("repository preferences changed before pipeline creation succeeded")
					}
					json.NewDecoder(r.Body).Decode(&created)
					if tc.createStatus != 0 {
						w.WriteHeader(tc.createStatus)
						io.WriteString(w, `{"message":"create rejected"}`)
						return
					}
					json.NewEncoder(w).Encode(buildkite.Pipeline{Name: created.Name, WebURL: "https://buildkite.com/target/fixture", Configuration: created.Configuration})
					return
				}
				switch {
				case r.URL.Path == "/v2/organizations/target":
					verified++
					if tc.orgStatus != 0 {
						w.WriteHeader(tc.orgStatus)
						io.WriteString(w, `{"message":"rejected"}`)
					} else {
						json.NewEncoder(w).Encode(buildkite.Organization{Name: "Target Organization", Slug: "target"})
					}
				case r.URL.Path == "/v2/organizations/target/clusters":
					clusters := []buildkite.Cluster{}
					if tc.clusters > 0 {
						clusters = append(clusters, buildkite.Cluster{ID: "cluster-1", Name: "First"})
					}
					if tc.clusters > 1 {
						clusters = append(clusters, buildkite.Cluster{ID: "cluster-2", Name: "Second"})
					}
					json.NewEncoder(w).Encode(clusters)
				case strings.HasSuffix(r.URL.Path, "/queues"):
					queues := []buildkite.ClusterQueue{}
					if tc.queues > 0 {
						queues = append(queues, buildkite.ClusterQueue{ID: "queue-1", Key: "chosen", DispatchPaused: tc.paused, DispatchPausedNote: "Maintenance"})
					}
					if tc.queues > 1 {
						queues = append(queues, buildkite.ClusterQueue{ID: "queue-2", Key: "other"})
					}
					json.NewEncoder(w).Encode(queues)
				case r.URL.Path == "/v2/organizations/target/agents":
					if r.URL.Query().Get("cluster_queue_id") != "queue-1" {
						t.Errorf("agent filter must identify the selected queue UUID: %s", r.URL.RawQuery)
					}
					agents := []buildkite.Agent{}
					if tc.busy {
						agents = append(agents, buildkite.Agent{ConnectedState: "connected", Job: &buildkite.Job{}})
					}
					json.NewEncoder(w).Encode(agents)
				default:
					w.WriteHeader(http.StatusNotFound)
					io.WriteString(w, `{"message":"not found"}`)
				}
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			t.Setenv("BUILDKITE_GRAPHQL_ENDPOINT", server.URL+"/graphql")
			var app struct {
				Create CreateCmd `cmd:""`
			}
			var stdout, stderr bytes.Buffer
			parser, err := kong.New(&app, kong.Writers(&stdout, &stderr), kong.Vars{"output_default_format": ""})
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := parser.Parse(append([]string{"create", "--json"}, tc.flags...))
			if err != nil {
				t.Fatal(err)
			}
			app.Create.Org = "target"
			err = app.Create.Run(ctx, cli.Globals{NoInput: true})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				if mutations != tc.wantMutations || stdout.Len() != 0 {
					t.Fatalf("failure created remote state or stdout: writes=%d, stdout=%s", mutations, stdout.String())
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				var output struct {
					Configuration string `json:"configuration"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
					t.Fatalf("structured stdout contaminated: %s: %v", stdout.String(), err)
				}
				if app.Create.Name != "Named" {
					var bootstrap struct {
						Steps []struct{ Agents map[string]string }
					}
					if err := yaml.Unmarshal([]byte(output.Configuration), &bootstrap); err != nil || len(bootstrap.Steps) != 1 || bootstrap.Steps[0].Agents["queue"] != "chosen" {
						t.Fatalf("bootstrap queue not routed: %q (%v)", output.Configuration, err)
					}
				} else if verified != 0 || strings.Contains(output.Configuration, "agents:") {
					t.Fatal("named invocation changed its prerequisite or bootstrap contract")
				}
				if app.Create.DryRun && mutations != 0 || !app.Create.DryRun && mutations != 1 {
					t.Fatalf("unexpected mutations: %d", mutations)
				}
				if tc.busy && (!strings.Contains(stderr.String(), "Observed 1 connected agents") || strings.Contains(stderr.String(), "no connected agents")) {
					t.Fatalf("busy agent reported unavailable: %s", stderr.String())
				}
			}
			if tc.file != "" {
				data, err := os.ReadFile(existing)
				if err != nil || string(data) != tc.file {
					t.Fatal("existing file was overwritten")
				}
			}
			starter, err := os.ReadFile(filepath.Join(root, ".buildkite", "pipeline.yaml"))
			shouldGenerate := app.Create.Template != "" && !app.Create.DryRun && tc.wantErr == ""
			if shouldGenerate {
				var pipeline struct{ Agents map[string]string }
				if err != nil || yaml.Unmarshal(starter, &pipeline) != nil || pipeline.Agents["queue"] != "chosen" {
					t.Fatalf("generated steps not routed: %s (%v)", starter, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("unexpected starter writes: %v", err)
			}
			if strings.Contains(stderr.String(), "Enter number") {
				t.Fatal("--no-input prompted")
			}
			after, err := os.ReadFile(filepath.Join(configDir, "bk.yaml"))
			if err != nil || !bytes.Equal(user, after) {
				t.Fatal("global configuration changed")
			}
			local, err := os.ReadFile(filepath.Join(root, ".bk.yaml"))
			if app.Create.RememberOrg && !app.Create.DryRun && created.Name != "" && tc.createStatus == 0 {
				if err != nil || !bytes.Contains(local, []byte("selected_org: target")) {
					t.Fatalf("repository preference not saved: %s (%v)", local, err)
				}
				if tc.savedOrg != "" {
					t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
					saved := config.New(nil, repo)
					pipelines := saved.PreferredPipelines()
					if tc.savedOrg != "target" && len(pipelines) != 0 {
						t.Fatalf("remembered org retargeted old cached pipelines: %#v", pipelines)
					}
					if tc.savedOrg == "target" && (len(pipelines) != 1 || pipelines[0].Org != "target" || pipelines[0].Name != "deploy") {
						t.Fatalf("unchanged org lost its cached pipelines: %#v", pipelines)
					}
					if saved.ConfiguredOutputFormat() != "yaml" || !saved.PagerDisabled() {
						t.Fatal("remembering an organization changed unrelated local settings")
					}
				}
			} else if tc.savedOrg != "" {
				if err != nil || !bytes.Equal(localBefore, local) {
					t.Fatal("failed create changed repository organization or cached pipelines")
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("repository preference saved without opt-in or during dry-run")
			}
		})
	}
}
