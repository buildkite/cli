package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/config"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/pkg/keyring"
	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/spf13/afero"
)

func TestCommandTelemetry(t *testing.T) {
	for _, tt := range []struct {
		name, command, outcome string
		args                   []string
		runErr                 error
	}{
		{"positional", "build view", "success", []string{"build", "view", "private-pipeline/123"}, nil},
		{"flags and alias", "build list", "success", []string{"build", "ls", "--pipeline=private-pipeline", "--branch=secret"}, nil},
		{"single command", "version", "success", []string{"version"}, nil},
		{"runtime failure", "build view", "error", []string{"build", "view", "private-pipeline/123"}, errors.New("secret error details")},
		{"unknown root", "", "unknown_command", []string{"private-unknown-command"}, nil},
		{"unknown nested", "build", "unknown_command", []string{"build", "private-unknown-command"}, nil},
		{"suggestion", "", "unknown_command", []string{"buil"}, nil},
		{"unknown flag", "build view", "error", []string{"build", "view", "--private-flag=secret"}, nil},
		{"missing positional", "job ssh", "error", []string{"job", "ssh"}, nil},
		{"extra positional", "version", "error", []string{"version", "secret"}, nil},
		{"missing subcommand", "build", "error", []string{"build"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := newKongParser(&CLI{})
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := parser.Parse(tt.args)
			if tt.runErr != nil {
				if err != nil {
					t.Fatal(err)
				}
				err = tt.runErr
			}
			command, outcome := commandTelemetry(ctx, err)
			if command != tt.command || outcome != tt.outcome {
				t.Fatalf("got (%q, %q), want (%q, %q); error: %v", command, outcome, tt.command, tt.outcome, err)
			}
		})
	}
}

func TestGitHubActionsSecretMigrationCommandRegistration(t *testing.T) {
	const workflow = ".github/workflows/migrate.yml"
	for _, tt := range []struct {
		name, command string
		args          []string
		workflowPath  func(*CLI) string
	}{
		{
			name:    "prepare",
			command: "secret migrate github-actions prepare",
			args: []string{
				"secret", "migrate", "github-actions", "prepare",
				"--secret", "API_KEY", "--match", "DEPLOY_*",
				"--output", workflow,
			},
			workflowPath: func(cli *CLI) string { return cli.Secret.Migrate.GitHubActions.Prepare.Output },
		},
		{
			name:         "run",
			command:      "secret migrate github-actions run",
			args:         []string{"secret", "migrate", "github-actions", "run", "--workflow", workflow},
			workflowPath: func(cli *CLI) string { return cli.Secret.Migrate.GitHubActions.Run.Workflow },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := &CLI{}
			parser, err := newKongParser(cli)
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := parser.Parse(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if got := ctx.Command(); got != tt.command {
				t.Fatalf("command = %q", got)
			}
			if got := tt.workflowPath(cli); got != workflow {
				t.Fatalf("workflow path = %q, want repository-relative %q", got, workflow)
			}
		})
	}
}

func TestCacheRegistryCommandRegistration(t *testing.T) {
	for _, tt := range []struct {
		name, command string
		args          []string
		assert        func(*testing.T, *CLI)
	}{
		{
			name:    "list",
			command: "cache registry list",
			args:    []string{"cache", "registry", "list", "cluster-uuid", "--per-page", "25", "--limit", "50"},
			assert: func(t *testing.T, cli *CLI) {
				if cli.Cache.Registry.List.ClusterUUID != "cluster-uuid" || cli.Cache.Registry.List.PerPage != 25 || cli.Cache.Registry.List.Limit != 50 {
					t.Fatalf("parsed list command = %#v", cli.Cache.Registry.List)
				}
			},
		},
		{
			name:    "list alias",
			command: "cache registry list",
			args:    []string{"cache", "registry", "ls", "cluster-uuid"},
			assert: func(t *testing.T, cli *CLI) {
				if cli.Cache.Registry.List.PerPage != 30 || cli.Cache.Registry.List.Limit != 100 {
					t.Fatalf("list defaults = (%d, %d)", cli.Cache.Registry.List.PerPage, cli.Cache.Registry.List.Limit)
				}
			},
		},
		{
			name:    "view",
			command: "cache registry view",
			args:    []string{"cache", "registry", "view", "cluster-uuid", "registry-uuid"},
			assert: func(t *testing.T, cli *CLI) {
				if cli.Cache.Registry.View.RegistryUUID != "registry-uuid" {
					t.Fatalf("registry UUID = %q", cli.Cache.Registry.View.RegistryUUID)
				}
			},
		},
		{
			name:    "create",
			command: "cache registry create",
			args:    []string{"cache", "registry", "create", "cluster-uuid", "--name", "Ruby gems", "--policy-file", "policy.yml"},
			assert: func(t *testing.T, cli *CLI) {
				if cli.Cache.Registry.Create.Name != "Ruby gems" || cli.Cache.Registry.Create.PolicyFile != "policy.yml" {
					t.Fatalf("parsed create command = %#v", cli.Cache.Registry.Create)
				}
			},
		},
		{
			name:    "update",
			command: "cache registry update",
			args:    []string{"cache", "registry", "update", "cluster-uuid", "registry-uuid", "--name", "Ruby packages", "--clear-policy"},
			assert: func(t *testing.T, cli *CLI) {
				if cli.Cache.Registry.Update.RegistryUUID != "registry-uuid" || cli.Cache.Registry.Update.Name == nil || *cli.Cache.Registry.Update.Name != "Ruby packages" || !cli.Cache.Registry.Update.ClearPolicy {
					t.Fatalf("parsed update command = %#v", cli.Cache.Registry.Update)
				}
			},
		},
		{
			name:    "delete alias",
			command: "cache registry delete",
			args:    []string{"cache", "registry", "rm", "cluster-uuid", "registry-uuid"},
			assert: func(t *testing.T, cli *CLI) {
				if cli.Cache.Registry.Delete.RegistryUUID != "registry-uuid" {
					t.Fatalf("registry UUID = %q", cli.Cache.Registry.Delete.RegistryUUID)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli := &CLI{}
			parser, err := newKongParser(cli)
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := parser.Parse(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if got := ctx.Command(); !strings.HasPrefix(got, tt.command) {
				t.Fatalf("command = %q, want prefix %q", got, tt.command)
			}
			command, outcome := commandTelemetry(ctx, nil)
			if command != tt.command || outcome != "success" {
				t.Fatalf("telemetry = (%q, %q)", command, outcome)
			}
			tt.assert(t, cli)
		})
	}
}

func TestHandleErrorPreservesExitCode(t *testing.T) {
	for _, tt := range []struct {
		err  error
		code int
	}{
		{errors.New("generic failure"), 1},
		{bkErrors.ErrAuthentication, 7},
		{bkErrors.ErrPreflightCompletedFailure, 9},
		{bkErrors.ErrUserAborted, 130},
	} {
		if got := handleError(tt.err); got != tt.code {
			t.Errorf("handleError(%v) = %d, want %d", tt.err, got, tt.code)
		}
	}
}

func TestListAssociatedOrganization(t *testing.T) {
	for _, command := range []struct {
		name string
		args []string
	}{
		{"build summary", []string{"build", "list", "--summary", "--json"}},
		{"build text", []string{"build", "list", "--text"}},
		{"build creator", []string{"build", "list", "--summary", "--json", "--creator=person@example.com"}},
		{"job search", []string{"job", "list", "--json"}},
		{"job text", []string{"job", "list", "--text"}},
		{"job build", []string{"job", "list", "--build=42", "--json"}},
	} {
		for _, tt := range []struct {
			name, pipeline, selected, wantOrg, wantToken string
			noTargetToken, envToken                      bool
		}{
			{name: "qualified", pipeline: "other/widgets", selected: "acme", wantOrg: "other", wantToken: "other-token"},
			{name: "URL", pipeline: "https://buildkite.com/other/widgets", selected: "acme", wantOrg: "other", wantToken: "other-token"},
			{name: "bare slug", pipeline: "widgets", selected: "acme", wantOrg: "acme", wantToken: "acme-token"},
			{name: "organization-wide", selected: "acme", wantOrg: "acme", wantToken: "acme-token"},
			{name: "no selected org", pipeline: "other/widgets", wantOrg: "other", wantToken: "other-token"},
			{name: "missing target credentials", pipeline: "other/widgets", selected: "acme", wantOrg: "other", noTargetToken: true},
			{name: "environment token without stored credentials", pipeline: "other/widgets", selected: "acme", wantOrg: "other", wantToken: "env-token", noTargetToken: true, envToken: true},
			{name: "environment token", pipeline: "other/widgets", selected: "acme", wantOrg: "other", wantToken: "env-token", envToken: true},
		} {
			if tt.pipeline == "" && command.name == "job build" {
				continue // A known build requires a pipeline.
			}
			t.Run(command.name+"/"+tt.name, func(t *testing.T) {
				t.Chdir(t.TempDir())
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
				if err := config.New(nil, nil).SelectOrganization(tt.selected, false); err != nil {
					t.Fatal(err)
				}
				t.Setenv("BUILDKITE_API_TOKEN", "")
				t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
				keyring.MockForTesting()
				kr := keyring.New()
				if err := kr.Set("acme", "acme-token"); err != nil {
					t.Fatal(err)
				}
				if !tt.noTargetToken {
					if err := kr.Set("other", "other-token"); err != nil {
						t.Fatal(err)
					}
				}
				if tt.envToken {
					t.Setenv("BUILDKITE_API_TOKEN", "env-token")
				}
				requests, lookups := 0, 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer "+tt.wantToken {
						t.Error("request used the wrong organization's credential")
					}
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Path == "/graphql" {
						lookups++
						var query struct{ Variables map[string]any }
						if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
							t.Error(err)
						}
						if query.Variables["organization"] != tt.wantOrg {
							t.Errorf("creator lookup org = %v, want %s", query.Variables["organization"], tt.wantOrg)
						}
						id := base64.StdEncoding.EncodeToString([]byte("User---creator-uuid"))
						fmt.Fprintf(w, `{"data":{"organization":{"members":{"edges":[{"node":{"user":{"id":%q}}}]}}}}`, id)
						return
					}
					requests++
					path := "/v2/organizations/" + tt.wantOrg + "/pipelines/widgets/builds"
					if tt.pipeline == "" {
						path = "/v2/organizations/" + tt.wantOrg + "/builds"
					}
					if command.name == "job build" {
						path += "/42/jobs"
					}
					if r.Method != http.MethodGet || r.URL.Path != path {
						t.Errorf("request = %s %s, want GET %s", r.Method, r.URL.Path, path)
					}
					if command.name == "build creator" && r.URL.Query().Get("creator") != "creator-uuid" {
						t.Errorf("creator query = %q", r.URL.RawQuery)
					}
					if command.name == "job build" {
						fmt.Fprint(w, `{"items":[{"id":"job-id","state":"failed"}],"links":{}}`)
					} else {
						fmt.Fprint(w, `[{"number":42,"state":"failed","jobs":[{"id":"job-id","state":"failed"}]}]`)
					}
				}))
				defer server.Close()
				t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
				t.Setenv("BUILDKITE_GRAPHQL_ENDPOINT", server.URL+"/graphql")
				before := config.New(nil, nil).OrganizationSlug()
				out, err := os.CreateTemp(t.TempDir(), "stdout")
				if err != nil {
					t.Fatal(err)
				}
				defer out.Close()
				stdout := os.Stdout
				os.Stdout = out
				defer func() { os.Stdout = stdout }()
				var cmd CLI
				parser, err := newKongParser(&cmd)
				if err != nil {
					t.Fatal(err)
				}
				args := append(append([]string{}, command.args...), "--pipeline", tt.pipeline, "--limit=1")
				ctx, err := parser.Parse(args)
				if err != nil {
					t.Fatal(err)
				}
				globals := cli.Globals{NoInput: true, Quiet: true, NoPager: true}
				if command.args[0] == "build" {
					err = cmd.Build.List.Run(ctx, globals)
				} else {
					err = cmd.Job.List.Run(ctx, globals)
				}
				if tt.noTargetToken && !tt.envToken {
					if err == nil || !strings.Contains(err.Error(), "bk auth login --org other") || !strings.Contains(err.Error(), "BUILDKITE_API_TOKEN") {
						t.Fatalf("expected actionable missing-credential error, got %v", err)
					}
					if requests != 0 || lookups != 0 {
						t.Fatalf("unauthenticated target made %d API requests and %d lookups", requests, lookups)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if requests != 1 || (command.name == "build creator" && lookups != 1) {
					t.Fatalf("requests = %d, creator lookups = %d", requests, lookups)
				}
				data, err := os.ReadFile(out.Name())
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(command.name, "text") {
					target := tt.wantOrg
					if tt.pipeline != "" {
						target += "/widgets"
					}
					if !strings.Contains(string(data), "for "+target+"\n") {
						t.Fatalf("wrong text identity: %s", data)
					}
				} else {
					var rows []map[string]any
					if err := json.Unmarshal(data, &rows); err != nil || len(rows) != 1 {
						t.Fatalf("output = %s, error = %v", data, err)
					}
					if command.args[0] == "build" && (rows[0]["organization"] != tt.wantOrg || (tt.pipeline != "" && rows[0]["pipeline"] != "widgets")) {
						t.Fatalf("wrong summary identity: %s", data)
					}
				}
				if got := config.New(nil, nil).OrganizationSlug(); got != before {
					t.Fatalf("selected org changed from %q to %q", before, got)
				}
			})
		}
	}
}

func TestGlobalOrganization(t *testing.T) {
	for _, tt := range []struct {
		name, path, response, envToken   string
		args                             []string
		latest, missingToken, noDefaults bool
		conflict                         bool
	}{
		{name: "list organization", args: []string{"build", "list", "--org", "ExampleOrg", "--summary", "--json"}, path: "/builds"},
		{name: "list pipeline and leading flag", args: []string{"--org=ExampleOrg", "build", "list", "--pipeline", "widgets", "--summary", "--json"}, path: "/pipelines/widgets/builds"},
		{name: "conflicting list target", args: []string{"build", "list", "--org=different", "--pipeline=ExampleOrg/widgets", "--summary", "--json"}, conflict: true},
		{name: "matching qualified list", args: []string{"build", "list", "--org=ExampleOrg", "--pipeline=ExampleOrg/widgets", "--summary", "--json"}, path: "/pipelines/widgets/builds"},
		{name: "no default organization", args: []string{"build", "list", "--org=ExampleOrg", "--summary", "--json"}, path: "/builds", noDefaults: true},
		{name: "view number", args: []string{"build", "--org", "ExampleOrg", "view", "42", "--pipeline", "widgets", "--summary", "--json"}, path: "/pipelines/widgets/builds/42", response: `{"number":42,"state":"passed"}`},
		{name: "view preferred pipeline", args: []string{"build", "view", "42", "--org=ExampleOrg", "--summary", "--json"}, path: "/pipelines/widgets/builds/42", response: `{"number":42,"state":"passed"}`},
		{name: "view latest", args: []string{"build", "view", "--org=ExampleOrg", "--pipeline=widgets", "--branch=feature", "--summary", "--json"}, path: "/pipelines/widgets/builds/42", response: `{"number":42,"state":"passed"}`, latest: true},
		{name: "job list", args: []string{"job", "list", "--org=ExampleOrg", "--json"}, path: "/builds"},
		{name: "agent list", args: []string{"agent", "list", "--org=ExampleOrg", "--json"}, path: "/agents"},
		{name: "api", args: []string{"api", "/pipelines", "--org=ExampleOrg"}, path: "/pipelines"},
		{name: "pipeline list", args: []string{"pipeline", "list", "--org=ExampleOrg", "--json"}, path: "/pipelines"},
		{name: "conflicting pipeline target", args: []string{"--org=ExampleOrg", "pipeline", "view", "different/widgets", "--json"}, conflict: true},
		{name: "matching qualified pipeline", args: []string{"--org=ExampleOrg", "pipeline", "view", "ExampleOrg/widgets", "--json"}, path: "/pipelines/widgets", response: `{"slug":"widgets"}`},
		{name: "environment token", args: []string{"build", "list", "--org=ExampleOrg", "--summary", "--json"}, path: "/builds", envToken: "env-token"},
		{name: "missing credential", args: []string{"build", "list", "--org=ExampleOrg", "--summary", "--json"}, missingToken: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "environment-org")
			t.Setenv("BUILDKITE_API_TOKEN", tt.envToken)
			t.Setenv("BK_TELEMETRY", "false")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			keyring.MockForTesting()
			t.Cleanup(keyring.ResetForTesting)
			kr := keyring.New()
			if err := kr.Set("environment-org", "wrong-token"); err != nil {
				t.Fatal(err)
			}
			if !tt.missingToken {
				if err := kr.Set("ExampleOrg", "target-token"); err != nil {
					t.Fatal(err)
				}
			}
			files := map[string]string{
				filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "bk.yaml"): "selected_org: user-org\n",
				".bk.yaml": "selected_org: local-org\npipelines: [widgets]\n",
			}
			if tt.name == "view preferred pipeline" {
				files[".bk.yaml"] = "selected_org: ExampleOrg\npipelines: [widgets]\n"
			}
			if tt.noDefaults {
				unsetEnv(t, "BUILDKITE_ORGANIZATION_SLUG")
				for path := range files {
					files[path] = "{}\n"
				}
			}
			beforeEnv, hadEnv := os.LookupEnv("BUILDKITE_ORGANIZATION_SLUG")
			for path, contents := range files {
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				wantToken := "target-token"
				if tt.envToken != "" {
					wantToken = tt.envToken
				}
				if r.Header.Get("Authorization") != "Bearer "+wantToken {
					t.Error("request used the wrong organization's credentials")
				}
				path := "/v2/organizations/ExampleOrg" + tt.path
				response := tt.response
				if tt.latest && requests == 1 {
					path = "/v2/organizations/ExampleOrg/pipelines/widgets/builds"
					response = `[{"number":42}]`
				}
				if r.Method != http.MethodGet || r.URL.Path != path {
					t.Errorf("request = %s %s, want GET %s", r.Method, r.URL.Path, path)
				}
				if response == "" {
					response = "[]"
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, response)
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			args := append([]string{"--no-input", "--quiet", "--no-pager"}, tt.args...)
			code, stdout, stderr := runCLI(t, args...)
			wantCode, wantRequests := 0, 1
			if tt.latest {
				wantRequests = 2
			}
			if tt.missingToken {
				wantCode, wantRequests = 1, 0
				if !strings.Contains(stderr, "bk auth login --org ExampleOrg") {
					t.Fatalf("missing actionable credential error: %s", stderr)
				}
			}
			if tt.conflict {
				wantCode, wantRequests = 1, 0
				if !strings.Contains(stderr, "conflicts with organization") {
					t.Fatalf("missing organization conflict error: %s", stderr)
				}
			}
			if code != wantCode || requests != wantRequests {
				t.Fatalf("exit=%d, requests=%d; want %d, %d; stderr=%s", code, requests, wantCode, wantRequests, stderr)
			}
			if strings.HasPrefix(tt.name, "view") && !strings.Contains(stdout, `"organization": "ExampleOrg"`) {
				t.Fatalf("wrong build identity: %s", stdout)
			}
			if got, exists := os.LookupEnv("BUILDKITE_ORGANIZATION_SLUG"); got != beforeEnv || exists != hadEnv {
				t.Fatalf("environment changed to %q", got)
			}
			for path, before := range files {
				after, err := os.ReadFile(path)
				if err != nil || string(after) != before {
					t.Fatalf("configuration file %s changed: %v", path, err)
				}
			}
		})
	}
}

func TestGlobalOrganizationRepositoryDiscovery(t *testing.T) {
	for _, tt := range []struct {
		name, override string
		noMatch        bool
	}{
		{name: "saved default"},
		{name: "matching override", override: "saved-org"},
		{name: "different override", override: "other-org"},
		{name: "no matching pipeline", override: "other-org", noMatch: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			override := tt.override
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
			t.Setenv("BUILDKITE_API_TOKEN", "test-token")
			t.Setenv("BK_TELEMETRY", "false")
			repo, err := git.PlainInit(".", false)
			if err != nil {
				t.Fatal(err)
			}
			const repository = "https://github.com/example/widgets.git"
			if _, err := repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{repository}}); err != nil {
				t.Fatal(err)
			}
			before := "selected_org: saved-org\n"
			if override == "other-org" {
				before += "pipelines: [deploy]\n"
			}
			if err := os.WriteFile(".bk.yaml", []byte(before), 0o600); err != nil {
				t.Fatal(err)
			}
			org := "saved-org"
			if override != "" {
				org = override
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v2/organizations/" + org + "/pipelines":
					if r.URL.Query().Get("repository") != repository {
						t.Errorf("repository query = %q", r.URL.RawQuery)
					}
					if tt.noMatch {
						fmt.Fprint(w, `[]`)
						return
					}
					fmt.Fprintf(w, `[{"slug":"discovered","repository":%q}]`, repository)
				case "/v2/organizations/" + org + "/pipelines/discovered/builds/42":
					fmt.Fprint(w, `{"number":42,"state":"passed"}`)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
			args := []string{"build", "view", "42", "--summary", "--json", "--no-input", "--quiet"}
			if override != "" {
				args = append(args, "--org", override)
			}
			code, stdout, stderr := runCLI(t, args...)
			if tt.noMatch {
				if code != 1 || requests != 1 || !strings.Contains(stderr, `could not resolve a pipeline in "other-org"; use --pipeline`) || strings.Contains(stderr, "unable to parse") {
					t.Fatalf("exit=%d, requests=%d, stdout=%s, stderr=%s", code, requests, stdout, stderr)
				}
			} else if code != 0 || requests != 2 || !strings.Contains(stdout, `"pipeline": "discovered"`) {
				t.Fatalf("exit=%d, requests=%d, stdout=%s, stderr=%s", code, requests, stdout, stderr)
			}
			after, err := os.ReadFile(".bk.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if override != "" {
				if string(after) != before {
					t.Fatalf("temporary organization changed local configuration: %s", after)
				}
			} else if pipelines := config.New(nil, nil).PreferredPipelines(); len(pipelines) != 1 || pipelines[0].Name != "discovered" {
				t.Fatalf("default organization did not cache discovered pipeline: %s", after)
			}
		})
	}
}

func TestGlobalOrganizationQualifiedTargets(t *testing.T) {
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "ambient-org")
	for _, args := range [][]string{
		{"build", "list", "--pipeline=SourceOrg/widgets"},
		{"job", "list", "--pipeline=https://buildkite.com/SourceOrg/widgets"},
		{"pipeline", "view", "SourceOrg/widgets"},
		{"pipeline", "view", "--pipeline=SourceOrg/widgets"},
		{"pipeline", "copy", "https://buildkite.com/SourceOrg/widgets", "--target=destination/copy"},
		{"build", "create", "--pipeline=SourceOrg/widgets"},
		{"build", "view", "SourceOrg/widgets/42"},
		{"build", "cancel", "https://buildkite.com/SourceOrg/widgets/builds/42"},
		{"artifacts", "list", "SourceOrg/widgets/42"},
		{"artifacts", "download", "--build=SourceOrg/widgets/42"},
		{"browse", "SourceOrg/widgets/42"},
		{"preflight", "--pipeline=SourceOrg/widgets"},
	} {
		for _, org := range []string{"", "SourceOrg", "other-org"} {
			t.Run(strings.Join(args, " ")+"/"+org, func(t *testing.T) {
				parser, err := newKongParser(&CLI{})
				if err != nil {
					t.Fatal(err)
				}
				input := append([]string{}, args...)
				if org != "" {
					input = append([]string{"--org=" + org}, input...)
				}
				_, err = parser.Parse(input)
				if org == "other-org" {
					if err == nil || !strings.Contains(err.Error(), "conflicts with organization") {
						t.Fatalf("expected target conflict, got %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestGlobalOrganizationSwitchPersists(t *testing.T) {
	for _, command := range [][]string{{"auth", "switch"}, {"auth", "use"}, {"use"}} {
		for _, inRepo := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/repo=%t", strings.Join(command, " "), inRepo), func(t *testing.T) {
				t.Chdir(t.TempDir())
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
				t.Setenv("BUILDKITE_API_TOKEN", "test-token")
				t.Setenv("BK_TELEMETRY", "false")
				if inRepo {
					if _, err := git.PlainInit(".", false); err != nil {
						t.Fatal(err)
					}
				}
				conf := config.New(nil, nil)
				if err := conf.EnsureOrganization("other-org"); err != nil {
					t.Fatal(err)
				}
				if err := conf.SelectOrganization("saved-org", inRepo); err != nil {
					t.Fatal(err)
				}
				args := append([]string{"--org=other-org", "--no-input"}, command...)
				args = append(args, "other-org")
				code, _, stderr := runCLI(t, args...)
				if code != 0 {
					t.Fatalf("exit=%d, stderr=%s", code, stderr)
				}
				if got := config.New(nil, nil).OrganizationSlug(); got != "other-org" {
					t.Fatalf("switch did not persist after override was restored: %q", got)
				}
			})
		}
	}
}

func TestConfigureExplicitEmptyOrganization(t *testing.T) {
	for _, explicitEmpty := range []bool{false, true} {
		t.Run(fmt.Sprintf("explicit-empty=%t", explicitEmpty), func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "ambient-org")
			t.Setenv("BUILDKITE_API_TOKEN", "")
			t.Setenv("BK_TELEMETRY", "false")
			t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
			keyring.MockForTesting()
			t.Cleanup(keyring.ResetForTesting)
			kr := keyring.New()
			if err := kr.Set("ambient-org", "original-token"); err != nil {
				t.Fatal(err)
			}
			if err := kr.SetRefreshToken("ambient-org", "original-refresh"); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "bk.yaml")
			const before = "selected_org: saved-org\n"
			if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
				t.Fatal(err)
			}
			stdin, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			oldStdin := os.Stdin
			os.Stdin = stdin
			t.Cleanup(func() { os.Stdin = oldStdin; stdin.Close() })
			args := []string{"configure", "add", "--token=replacement"}
			if explicitEmpty {
				args = append(args, "--org=")
			}
			code, _, stderr := runCLI(t, args...)
			wantCode, wantToken := 0, "replacement"
			if explicitEmpty {
				wantCode, wantToken = 1, "original-token"
			}
			if code != wantCode {
				t.Fatalf("exit=%d, want %d; stderr=%s", code, wantCode, stderr)
			}
			if token, err := kr.Get("ambient-org"); err != nil || token != wantToken {
				t.Fatalf("unexpected credential after configure: %v", err)
			}
			if explicitEmpty {
				if refresh, err := kr.GetRefreshToken("ambient-org"); err != nil || refresh != "original-refresh" {
					t.Fatalf("refresh credential changed: %v", err)
				}
				if after, err := os.ReadFile(path); err != nil || string(after) != before {
					t.Fatalf("configuration changed: %v", err)
				}
			} else if got := config.New(nil, nil).SavedOrganizationSlug(); got != "ambient-org" {
				t.Fatalf("omitted flag did not persist environment organization: %q", got)
			}
		})
	}
}

func TestGlobalOrganizationCompatibility(t *testing.T) {
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "environment-org")
	for _, tt := range []struct {
		args []string
		org  func(*CLI) string
	}{
		{[]string{"auth", "login", "--token=test"}, func(c *CLI) string { return c.Auth.Login.Org }},
		{[]string{"auth", "logout"}, func(c *CLI) string { return c.Auth.Logout.Org }},
		{[]string{"pipeline", "list"}, func(c *CLI) string { return c.Pipeline.List.Org }},
		{[]string{"pipeline", "view", "widgets"}, func(c *CLI) string { return c.Pipeline.View.Org }},
		{[]string{"pipeline", "create", "widgets"}, func(c *CLI) string { return c.Pipeline.Create.Org }},
		{[]string{"pipeline", "copy", "widgets"}, func(c *CLI) string { return c.Pipeline.Copy.Org }},
		{[]string{"configure", "--token=test"}, func(c *CLI) string { return c.Configure.Org }},
		{[]string{"configure", "add", "--token=test"}, func(c *CLI) string { return c.Configure.Org }},
	} {
		for _, org := range []string{"", "FlagOrg"} {
			t.Run(strings.Join(tt.args, " ")+"/"+org, func(t *testing.T) {
				app := &CLI{}
				parser, err := newKongParser(app)
				if err != nil {
					t.Fatal(err)
				}
				args := append([]string{}, tt.args...)
				if org != "" {
					args = append(args, "--org", org)
				}
				if _, err := parser.Parse(args); err != nil {
					t.Fatal(err)
				}
				want := org
				if org == "" && tt.args[0] == "configure" {
					want = "environment-org"
				}
				if got := tt.org(app); got != want {
					t.Fatalf("organization = %q, want %q", got, want)
				}
			})
		}
	}
	for _, flags := range [][]string{{"--all", "--org=other"}, {"--all", "--org="}, {"--all=false", "--org=other"}} {
		parser, err := newKongParser(&CLI{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse(append([]string{"auth", "logout"}, flags...)); err == nil || !strings.Contains(err.Error(), "--org") || !strings.Contains(err.Error(), "--all") {
			t.Fatalf("expected conflicting logout flags %v to fail, got %v", flags, err)
		}
	}
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	oldArgs, oldOut, oldErr := os.Args, os.Stdout, os.Stderr
	os.Args, os.Stdout, os.Stderr = append([]string{"bk"}, args...), stdout, stderr
	defer func() { os.Args, os.Stdout, os.Stderr = oldArgs, oldOut, oldErr }()
	code := run()
	out, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	errOut, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	return code, string(out), string(errOut)
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	original, had := os.LookupEnv(key)
	if had {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("failed to unset env %s: %v", key, err)
		}
	}
	t.Cleanup(func() {
		var err error
		if had {
			err = os.Setenv(key, original)
		} else {
			err = os.Unsetenv(key)
		}
		if err != nil {
			t.Fatalf("failed to restore env %s: %v", key, err)
		}
	})
}

func TestVNCCommandRegistration(t *testing.T) {
	cli := &CLI{}
	parser, err := newKongParser(cli)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	if _, err := parser.Parse([]string{"job", "vnc", "0190046e-e199-453b-a302-a21a4d649d31"}); err != nil {
		t.Fatalf("failed to parse job vnc command: %v", err)
	}
	if cli.Job.VNC.JobID != "0190046e-e199-453b-a302-a21a4d649d31" {
		t.Errorf("VNC job ID = %q", cli.Job.VNC.JobID)
	}

	for _, command := range parser.Model.Children {
		if command.Name != "job" {
			continue
		}
		for _, subcommand := range command.Children {
			if subcommand.Name == "vnc" {
				if subcommand.Hidden {
					t.Error("job vnc should be visible")
				}
				return
			}
		}
		t.Fatal("job vnc command not found")
	}

	t.Fatal("job command not found")
}

func TestSSHCommandRegistration(t *testing.T) {
	cli := &CLI{}
	parser, err := newKongParser(cli)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}

	if _, err := parser.Parse([]string{"job", "ssh", "0190046e-e199-453b-a302-a21a4d649d31"}); err != nil {
		t.Fatalf("failed to parse job ssh command: %v", err)
	}
	if cli.Job.SSH.JobID != "0190046e-e199-453b-a302-a21a4d649d31" {
		t.Errorf("SSH job ID = %q", cli.Job.SSH.JobID)
	}

	for _, command := range parser.Model.Children {
		if command.Name != "job" {
			continue
		}
		for _, subcommand := range command.Children {
			if subcommand.Name == "ssh" {
				if subcommand.Hidden {
					t.Error("job ssh should be visible")
				}
				if !strings.Contains(subcommand.Help, "hosted job") || strings.Contains(subcommand.Help, "macOS") {
					t.Errorf("job ssh help = %q, want platform-neutral hosted job scope", subcommand.Help)
				}
				return
			}
		}
		t.Fatal("job ssh command not found")
	}

	t.Fatal("job command not found")
}

func TestApplyExperiments(t *testing.T) {
	t.Run("preflight visible by default", func(t *testing.T) {
		unsetEnv(t, "BUILDKITE_EXPERIMENTS")
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		applyExperiments(parser, conf)

		for _, node := range parser.Model.Children {
			if node.Name == "preflight" {
				if node.Hidden {
					t.Error("preflight should be visible by default")
				}
				return
			}
		}
		t.Fatal("preflight command not found in parser")
	})

	t.Run("preflight hidden when experiment disabled", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "alpha")
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		applyExperiments(parser, conf)

		for _, node := range parser.Model.Children {
			if node.Name == "preflight" {
				if !node.Hidden {
					t.Error("preflight should be hidden when experiment is disabled")
				}
				return
			}
		}
		t.Fatal("preflight command not found in parser")
	})

	t.Run("preflight hidden when experiments override is empty", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "")
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		applyExperiments(parser, conf)

		for _, node := range parser.Model.Children {
			if node.Name == "preflight" {
				if !node.Hidden {
					t.Error("preflight should be hidden when experiments override is empty")
				}
				return
			}
		}
		t.Fatal("preflight command not found in parser")
	})

	t.Run("preflight visible when experiment enabled explicitly", func(t *testing.T) {
		t.Setenv("BUILDKITE_EXPERIMENTS", "preflight")
		fs := afero.NewMemMapFs()
		conf := config.New(fs, nil)

		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		applyExperiments(parser, conf)

		for _, node := range parser.Model.Children {
			if node.Name == "preflight" {
				if node.Hidden {
					t.Error("preflight should be visible when experiment is enabled")
				}
				return
			}
		}
		t.Fatal("preflight command not found in parser")
	})

	t.Run("preflight root still parses with default subcommand", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight"}); err != nil {
			t.Fatalf("failed to parse preflight root command: %v", err)
		}
	})

	t.Run("preflight await-test-results parses without a value", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight", "--await-test-results"}); err != nil {
			t.Fatalf("failed to parse preflight await-test-results flag: %v", err)
		}
		if !cli.Preflight.Run.AwaitTestResults.Enabled {
			t.Fatal("expected await-test-results to be enabled")
		}
		if cli.Preflight.Run.AwaitTestResults.Duration != 30*time.Second {
			t.Fatalf("expected default await-test-results duration, got %s", cli.Preflight.Run.AwaitTestResults.Duration)
		}
	})

	t.Run("preflight await-test-results parses with an explicit duration", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight", "--await-test-results=45s"}); err != nil {
			t.Fatalf("failed to parse preflight await-test-results duration: %v", err)
		}
		if !cli.Preflight.Run.AwaitTestResults.Enabled {
			t.Fatal("expected await-test-results to be enabled")
		}
		if cli.Preflight.Run.AwaitTestResults.Duration != 45*time.Second {
			t.Fatalf("expected explicit await-test-results duration, got %s", cli.Preflight.Run.AwaitTestResults.Duration)
		}
	})

	t.Run("preflight exit-on parses repeated flags", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight", "--exit-on=build-failing", "--exit-on=build-failing"}); err != nil {
			t.Fatalf("failed to parse repeated preflight exit-on flags: %v", err)
		}
		if len(cli.Preflight.Run.ExitOn) != 2 {
			t.Fatalf("expected 2 exit-on values, got %d", len(cli.Preflight.Run.ExitOn))
		}
	})

	t.Run("preflight exit-on rejects unknown values", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight", "--exit-on=test-failed:3"}); err == nil {
			t.Fatal("expected parse error for invalid exit-on value")
		}
	})

	t.Run("preflight exit-on rejects incompatible combinations", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight", "--exit-on=build-failing", "--exit-on=build-terminal"}); err == nil {
			t.Fatal("expected parse error for incompatible exit-on values")
		}
	})

	t.Run("preflight run subcommand still parses", func(t *testing.T) {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatalf("failed to create parser: %v", err)
		}

		if _, err := parser.Parse([]string{"preflight", "run", "--await-test-results=45s"}); err != nil {
			t.Fatalf("failed to parse preflight run subcommand: %v", err)
		}
		if !cli.Preflight.Run.AwaitTestResults.Enabled {
			t.Fatal("expected run subcommand await-test-results to be enabled")
		}
		if cli.Preflight.Run.AwaitTestResults.Duration != 45*time.Second {
			t.Fatalf("expected explicit run subcommand await-test-results duration, got %s", cli.Preflight.Run.AwaitTestResults.Duration)
		}
	})

	t.Run("preflight help includes mirrored run flags", func(t *testing.T) {
		help, err := renderPreflightHelp()
		if err != nil {
			t.Fatalf("failed to render preflight help: %v", err)
		}
		for _, want := range []string{
			"--[no-]watch",
			"--exit-on=EXIT-ON,...",
			"--await-test-results",
			"--no-cleanup",
			"preflight cleanup [flags]",
		} {
			if !strings.Contains(help, want) {
				t.Fatalf("expected preflight help to contain %q, got:\n%s", want, help)
			}
		}
	})

	t.Run("preflight help requests are detected", func(t *testing.T) {
		tests := []struct {
			args []string
			want bool
		}{
			{args: []string{"preflight", "--help"}, want: true},
			{args: []string{"preflight", "-h"}, want: true},
			{args: []string{"help", "preflight"}, want: true},
			{args: []string{"preflight", "run", "--help"}, want: false},
		}

		for _, tt := range tests {
			if got := isPreflightHelpRequest(tt.args); got != tt.want {
				t.Fatalf("isPreflightHelpRequest(%q) = %v, want %v", tt.args, got, tt.want)
			}
		}
	})
}
