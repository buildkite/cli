package setup_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/pipeline/setup"
	"github.com/goccy/go-yaml"
)

func TestEnsurePreservesUploadLocations(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"buildkite.yml", "buildkite.yaml", "buildkite.json",
		".buildkite/pipeline.yml", ".buildkite/pipeline.yaml", ".buildkite/pipeline.json",
		"buildkite/pipeline.yml", "buildkite/pipeline.yaml", "buildkite/pipeline.json",
	} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			file := filepath.Join(root, path)
			os.MkdirAll(filepath.Dir(file), 0o755)
			os.WriteFile(file, []byte("existing pipeline"), 0o600)
			// Interactive=true must not prompt when a pipeline already exists.
			if err := setup.Ensure(root, "", true, io.Discard); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(file)
			if err != nil || string(data) != "existing pipeline" {
				t.Fatalf("existing file not preserved: %q, %v", data, err)
			}
			if path != ".buildkite/pipeline.yaml" {
				if _, err := os.Stat(filepath.Join(root, ".buildkite", "pipeline.yaml")); !os.IsNotExist(err) {
					t.Fatalf("created competing pipeline: %v", err)
				}
			}
		})
	}
}

func TestProjectStarters(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		files    map[string]string
		want     string
		commands []string
	}{
		{"go", map[string]string{"go.mod": "module example.com/app\ngo 1.26\n"}, "go", []string{"go test -v ./...", "go vet ./..."}},
		{"node without tests", map[string]string{"package.json": `{"scripts":{"build":"tsc"}}`}, "empty", nil},
		{"npm placeholder", map[string]string{"package.json": `{"scripts":{"test":"echo \"Error: no test specified\" && exit 1"}}`}, "empty", nil},
		{"npm unlocked", map[string]string{"package.json": `{"scripts":{"test":"mocha"}}`}, "node", []string{"npm install", "npm test"}},
		{"npm locked", map[string]string{"package.json": `{"scripts":{"test":"mocha"}}`, "package-lock.json": "{}"}, "node", []string{"npm ci", "npm test"}},
		{"pnpm", map[string]string{"package.json": `{"scripts":{"test":"vitest"}}`, "pnpm-lock.yaml": ""}, "node", []string{"pnpm install --frozen-lockfile", "pnpm test"}},
		{"yarn classic", map[string]string{"package.json": `{"scripts":{"test":"jest"}}`, "yarn.lock": ""}, "node", []string{"yarn install --frozen-lockfile", "yarn test"}},
		{"yarn modern", map[string]string{"package.json": `{"scripts":{"test":"jest"}}`, "yarn.lock": "__metadata:\n  version: 8\n"}, "node", []string{"yarn install --immutable", "yarn test"}},
		{"bun script", map[string]string{"package.json": `{"scripts":{"test":"vitest run"}}`, "bun.lock": ""}, "node", []string{"bun install --frozen-lockfile", "bun run test"}},
		{"declared manager", map[string]string{"package.json": `{"packageManager":"pnpm@10.0.0","scripts":{"test":"vitest"}}`}, "node", []string{"pnpm install", "pnpm test"}},
		{"conflicting locks", map[string]string{"package.json": `{"scripts":{"test":"jest"}}`, "yarn.lock": "", "package-lock.json": "{}"}, "empty", nil},
		{"manager mismatch", map[string]string{"package.json": `{"packageManager":"pnpm@10.0.0","scripts":{"test":"vitest"}}`, "package-lock.json": "{}"}, "empty", nil},
		{"python marker only", map[string]string{"pyproject.toml": ""}, "empty", nil},
		{"pipenv without pytest", map[string]string{"Pipfile": "[dev-packages]\n", "Pipfile.lock": "{}"}, "empty", nil},
		{"pipenv pytest", map[string]string{"Pipfile": "[dev-packages]\npytest = \"*\"\n", "Pipfile.lock": "{}"}, "pipenv", []string{"pipenv install --deploy --dev", "pipenv run pytest"}},
		{"ruby without rspec", map[string]string{"Gemfile": "source 'https://rubygems.org'", "Gemfile.lock": ""}, "empty", nil},
		{"rspec", map[string]string{"Gemfile": "gem 'rspec'\n", "Gemfile.lock": ""}, "ruby", []string{"bundle install", "bundle exec rspec"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for name, data := range tc.files {
				os.WriteFile(filepath.Join(root, name), []byte(data), 0o600)
			}
			if got := setup.Recommended(root); got != tc.want {
				t.Fatalf("recommendation = %s, want %s", got, tc.want)
			}
			if err := setup.Ensure(root, "", false, io.Discard); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(root, ".buildkite", "pipeline.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			var pipeline struct {
				Steps []struct {
					Commands []string `yaml:"commands"`
				} `yaml:"steps"`
			}
			if err := yaml.Unmarshal(data, &pipeline); err != nil {
				t.Fatal(err)
			}
			if tc.want == "empty" {
				if len(pipeline.Steps) != 0 {
					t.Fatal("unsupported project should get empty steps")
				}
				return
			}
			if len(pipeline.Steps) != 1 || strings.Join(pipeline.Steps[0].Commands, "\n") != strings.Join(tc.commands, "\n") {
				t.Fatalf("unexpected commands: %s", data)
			}
			if !strings.Contains(string(data), "https://github.com/buildkite/") || !strings.Contains(string(data), "# Requires:") {
				t.Fatal("starter must include source and runtime prerequisites")
			}
		})
	}
}
