package release_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// External writes are replaced with local command fixtures. The real gh binary
// evaluates the release PR query against an HTTP fixture, not GitHub.
func releaseFixture(t *testing.T, env map[string]string, prChanges map[string]any) (string, func(string, ...string) (string, error)) {
	t.Helper()
	gh, err := exec.LookPath("gh")
	if err != nil {
		t.Fatal("Run release tests with MISE_ENV=release mise exec -- go test ./.buildkite")
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".buildkite"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"release-pr.sh", "release.sh", "upload-packages.sh"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".buildkite", name), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	version := "3.70.0"
	if v, ok := env["VERSION"]; ok {
		version = v
	}
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte(version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pr := map[string]any{
		"title": "release: v3.70.0", "body": "## Fixed\n\nKeep `$(echo notes)` literal.\n",
		"base":   map[string]any{"ref": "main", "repo": map[string]any{"full_name": "buildkite/cli"}},
		"labels": []map[string]string{{"name": "release"}}, "merged_at": "2026-11-17T00:00:00Z", "merge_commit_sha": "abc123",
	}
	for key, value := range prChanges {
		pr[key] = value
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Unexpected HTTP write: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		var body any = pr
		if strings.Contains(r.URL.Path, "/commits/") {
			body = []any{pr}
		}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	const tool = `#!/usr/bin/env bash
set -euo pipefail
case "$(basename "$0")" in
  git)
    case "$1" in
      rev-parse) if [[ "$2" == HEAD ]]; then echo abc123; else echo "${TAG_COMMIT:-abc123}"; fi ;;
      merge-base) echo base ;;
      show) echo "${PREVIOUS_VERSION:-3.69.0}" ;;
      fetch) exit 0 ;;
      ls-remote) [[ "${REMOTE_ERROR:-0}" == 0 ]] || exit 128; printf '%s\n' "${REMOTE_TAGS:-abc000 refs/tags/v3.69.0}" ;;
      tag|push) echo "git $*" >> "$ACTION_LOG" ;;
      *) exit 99 ;;
    esac ;;
  gh)
    if [[ "$1" == api ]]; then
      shift; endpoint="$1"; shift
      exec "$REAL_GH" api "$API_URL/$endpoint" "$@"
    fi ;;
  buildkite-agent)
    case "$1 $2" in
      'step get') if [[ "$5" == "${FAILED_STEP:-}" ]]; then echo failed; else echo passed; fi ;;
      'oidc request-token') echo synthetic-token ;;
      'meta-data exists') [[ ! -f "$PWD/$3" ]] || exit 0; exit 100 ;;
      'meta-data get') cat "$PWD/$3" ;;
      'meta-data set') printf '%s' "$4" > "$PWD/$3" ;;
      *) exit 99 ;;
    esac ;;
  goreleaser)
    if [[ "$1" == check ]]; then exit "${CONFIG_STATUS:-0}"; fi
    echo "goreleaser $*" >> "$ACTION_LOG"
    while [[ $# -gt 0 ]]; do
      if [[ "$1" == --release-notes ]]; then cp "$2" published-notes; fi
      shift
    done ;;
  curl)
    echo "curl $*" >> "$ACTION_LOG"
    [[ "$*" != *"${FAILED_PACKAGE:-never-match}"* ]] || exit 22 ;;
  ko) echo "ko $*" >> "$ACTION_LOG" ;;
esac
`
	for _, name := range []string{"git", "gh", "goreleaser", "buildkite-agent", "curl", "ko"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(tool), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir, func(script string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command("bash", append([]string{filepath.Join(".buildkite", script)}, args...)...)
		cmd.Dir = dir
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "BUILDKITE_") && !strings.HasPrefix(entry, "GH_") && !strings.HasPrefix(entry, "GITHUB_") && !strings.HasPrefix(entry, "DOCKERHUB_") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		defaults := map[string]string{
			"PATH": bin + string(os.PathListSeparator) + os.Getenv("PATH"), "REAL_GH": gh,
			"API_URL": server.URL, "ACTION_LOG": filepath.Join(dir, "actions"), "GH_TOKEN": "synthetic-token",
			"BUILDKITE_PIPELINE_SLUG": "buildkite-cli", "BUILDKITE_BRANCH": "main", "BUILDKITE_COMMIT": "abc123", "BUILDKITE_PULL_REQUEST": "false",
		}
		for key, value := range env {
			defaults[key] = value
		}
		for key, value := range defaults {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
}

func TestReleasePaths(t *testing.T) {
	for _, tt := range []struct {
		name, script, action string
		args                 []string
		env                  map[string]string
		pr                   map[string]any
		wantError            bool
	}{
		{name: "merged release tags exact commit", script: "release-pr.sh", args: []string{"tag"}, action: "git push origin abc123:refs/tags/v3.70.0"},
		{name: "PR cannot tag even from main", script: "release-pr.sh", args: []string{"tag"}, env: map[string]string{"BUILDKITE_PULL_REQUEST": "42"}, wantError: true},
		{name: "feature branch cannot tag", script: "release-pr.sh", args: []string{"tag"}, env: map[string]string{"BUILDKITE_BRANCH": "feature"}, wantError: true},
		{name: "released version does nothing", script: "release-pr.sh", args: []string{"tag"}, env: map[string]string{"VERSION": "3.69.0"}},
		{name: "remote failure does not tag", script: "release-pr.sh", args: []string{"tag"}, env: map[string]string{"REMOTE_ERROR": "1"}, wantError: true},
		{name: "ordinary merge cannot release a pending version", script: "release-pr.sh", args: []string{"tag"}, pr: map[string]any{"title": "Fix a bug"}, wantError: true},
		{name: "PR validates release", script: "release-pr.sh", args: []string{"check"}, env: map[string]string{"BUILDKITE_PULL_REQUEST": "42", "CONFIG_STATUS": "2"}},
		{name: "PR without bump needs no release metadata", script: "release-pr.sh", args: []string{"check"}, env: map[string]string{"VERSION": "3.69.0", "BUILDKITE_PULL_REQUEST": "42"}, pr: map[string]any{"title": "Fix a bug"}},
		{name: "invalid config fails", script: "release-pr.sh", args: []string{"check"}, env: map[string]string{"CONFIG_STATUS": "1"}, wantError: true},
		{name: "invalid version fails", script: "release-pr.sh", args: []string{"check"}, env: map[string]string{"VERSION": "v3.70.0"}, wantError: true},
		{name: "old version fails", script: "release-pr.sh", args: []string{"check"}, env: map[string]string{"VERSION": "3.9.0", "BUILDKITE_PULL_REQUEST": "42"}, wantError: true},
		{name: "cannot lower pending main version", script: "release-pr.sh", args: []string{"check"}, env: map[string]string{"PREVIOUS_VERSION": "3.71.0", "BUILDKITE_PULL_REQUEST": "42"}, wantError: true},
		{name: "missing release label fails", script: "release-pr.sh", args: []string{"check"}, env: map[string]string{"BUILDKITE_PULL_REQUEST": "42"}, pr: map[string]any{"labels": []any{}}, wantError: true},
		{name: "blank notes fail", script: "release-pr.sh", args: []string{"check"}, env: map[string]string{"BUILDKITE_PULL_REQUEST": "42"}, pr: map[string]any{"body": " \r\n\t"}, wantError: true},
		{name: "PR cannot publish", script: "release.sh", args: []string{"release", "--clean"}, env: map[string]string{"BUILDKITE_PIPELINE_SLUG": "buildkite-cli-release", "BUILDKITE_TAG": "v3.70.0", "BUILDKITE_PULL_REQUEST": "42"}, wantError: true},
		{name: "main cannot publish directly", script: "release.sh", args: []string{"release", "--clean"}, wantError: true},
		{name: "snapshot cannot publish", script: "release.sh", args: []string{"release", "--clean", "--snapshot"}, action: "goreleaser release --clean --snapshot"},
		{name: "tag publishes reviewed notes including on retry", script: "release.sh", args: []string{"release", "--clean"}, env: map[string]string{"BUILDKITE_PIPELINE_SLUG": "buildkite-cli-release", "BUILDKITE_TAG": "v3.70.0", "BUILDKITE_RETRY_COUNT": "1"}, action: "goreleaser release --clean --release-notes"},
		{name: "tag without merged PR cannot publish", script: "release.sh", args: []string{"release", "--clean"}, env: map[string]string{"BUILDKITE_PIPELINE_SLUG": "buildkite-cli-release", "BUILDKITE_TAG": "v3.70.0"}, pr: map[string]any{"merged_at": nil}, wantError: true},
		{name: "mismatched tag commit cannot publish", script: "release.sh", args: []string{"release", "--clean"}, env: map[string]string{"BUILDKITE_PIPELINE_SLUG": "buildkite-cli-release", "BUILDKITE_TAG": "v3.70.0", "TAG_COMMIT": "other-commit"}, wantError: true},
		{name: "PR cannot upload packages", script: "upload-packages.sh", args: []string{"deb"}, env: map[string]string{"BUILDKITE_PIPELINE_SLUG": "buildkite-cli-release", "BUILDKITE_TAG": "v3.70.0", "BUILDKITE_PULL_REQUEST": "42"}, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir, run := releaseFixture(t, tt.env, tt.pr)
			out, err := run(tt.script, tt.args...)
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, wantError = %v\n%s", err, tt.wantError, out)
			}
			actions, _ := os.ReadFile(filepath.Join(dir, "actions"))
			if tt.action == "" && len(actions) != 0 || tt.action != "" && !strings.Contains(string(actions), tt.action) {
				t.Fatalf("unexpected external actions: %s\n%s", actions, out)
			}
			if strings.Contains(tt.action, "--release-notes") {
				notes, err := os.ReadFile(filepath.Join(dir, "published-notes"))
				if err != nil || strings.TrimSpace(string(notes)) != "## Fixed\n\nKeep `$(echo notes)` literal." {
					t.Fatalf("reviewed notes changed: %q, %v", notes, err)
				}
			}
		})
	}
	for _, step := range []string{"lint", "test", "vulncheck", "build", "release_check"} {
		t.Run("failed prerequisite "+step, func(t *testing.T) {
			dir, run := releaseFixture(t, map[string]string{"FAILED_STEP": step}, nil)
			if out, err := run("release-pr.sh", "tag"); err == nil {
				t.Fatalf("tagging succeeded with failed %s: %s", step, out)
			}
			if _, err := os.Stat(filepath.Join(dir, "actions")); !os.IsNotExist(err) {
				t.Fatal("failed prerequisite allowed external actions")
			}
		})
	}
}

func TestPackageRetry(t *testing.T) {
	env := map[string]string{"BUILDKITE_PIPELINE_SLUG": "buildkite-cli-release", "BUILDKITE_TAG": "v3.70.0", "FAILED_PACKAGE": "arm64"}
	dir, run := releaseFixture(t, env, nil)
	if err := os.Mkdir(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if err := os.WriteFile(filepath.Join(dir, "dist", "bk_3.70.0_"+arch+".deb"), []byte(arch), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := run("upload-packages.sh", "deb"); err == nil {
		t.Fatalf("expected partial upload failure: %s", out)
	}
	env["FAILED_PACKAGE"] = "never-match"
	if out, err := run("upload-packages.sh", "deb"); err != nil {
		t.Fatalf("retry failed: %v\n%s", err, out)
	}
	actions, err := os.ReadFile(filepath.Join(dir, "actions"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(actions), "amd64.deb") != 1 || strings.Count(string(actions), "arm64.deb") != 2 {
		t.Fatalf("retry must skip successful files and retry failed ones: %s", actions)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist", "bk_3.70.0_amd64.deb"), []byte("changed package"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := run("upload-packages.sh", "deb"); err == nil || !strings.Contains(out, "changed after publication") {
		t.Fatalf("changed package must not be resent: %v\n%s", err, out)
	}
}

func TestReleasePipelineDependencies(t *testing.T) {
	data, err := os.ReadFile("pipeline.yml")
	if err != nil {
		t.Fatal(err)
	}
	var pipeline struct {
		Steps []struct {
			Group string `yaml:"group"`
			If    string `yaml:"if"`
			Steps []struct {
				Key       string   `yaml:"key"`
				DependsOn []string `yaml:"depends_on"`
			} `yaml:"steps"`
		} `yaml:"steps"`
	}
	if err := yaml.Unmarshal(data, &pipeline); err != nil {
		t.Fatal(err)
	}
	for _, group := range pipeline.Steps {
		for _, step := range group.Steps {
			if step.Key != "tag_release" {
				continue
			}
			if group.If != `build.branch == "main" && build.pull_request.id == null && build.tag == null` {
				t.Fatal("release credentials must be inside the non-PR main group")
			}
			for _, prerequisite := range []string{"lint", "test", "vulncheck", "build", "release_check"} {
				if !strings.Contains(strings.Join(step.DependsOn, ","), prerequisite) {
					t.Errorf("release does not depend on %s", prerequisite)
				}
			}
			return
		}
	}
	t.Fatal("release tagging step missing")
}
