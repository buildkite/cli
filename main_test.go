package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/config"
	bkErrors "github.com/buildkite/cli/v3/internal/errors"
	"github.com/buildkite/cli/v3/pkg/keyring"
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
