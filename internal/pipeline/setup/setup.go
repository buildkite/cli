// Package setup shares local pipeline setup between bk init and pipeline create.
package setup

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	bkIO "github.com/buildkite/cli/v3/internal/io"
	git "github.com/go-git/go-git/v5"
)

// Root returns the worktree root, or the current directory outside Git.
func Root(repo *git.Repository) (string, error) {
	if repo != nil {
		wt, err := repo.Worktree()
		if err != nil {
			return "", err
		}
		return wt.Filesystem.Root(), nil
	}
	return os.Getwd()
}

// Find checks the standard upload locations in buildkite-agent's search order.
func Find(root string) (string, error) {
	for _, path := range []string{
		"buildkite.yml", "buildkite.yaml", "buildkite.json",
		".buildkite/pipeline.yml", ".buildkite/pipeline.yaml", ".buildkite/pipeline.json",
		"buildkite/pipeline.yml", "buildkite/pipeline.yaml", "buildkite/pipeline.json",
	} {
		path = filepath.Join(root, filepath.FromSlash(path))
		if _, err := os.Lstat(path); err == nil {
			return path, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", nil
}

type starter struct {
	id, label, source, requires, commands string
}

var starters = []starter{
	{"go", "Go test and vet", "https://github.com/buildkite/golang-example/blob/main/.buildkite/pipeline.yml", "Go matching go.mod, with access to module dependencies", "    - go test -v ./...\n    - go vet ./...\n"},
	{"node", "Node test script", "https://github.com/buildkite/nodejs-example/blob/main/.buildkite/pipeline.yml", "Node.js and the project's package manager; package.json must define a test script", ""},
	{"pipenv", "Pipenv / pytest", "https://github.com/buildkite/python-pipenv-example/blob/main/.buildkite/pipeline.yml", "Python, Pipenv and pytest in Pipfile dev-packages", "    - pipenv install --dev\n    - pipenv run pytest\n"},
	{"ruby", "Bundler / RSpec", "https://github.com/buildkite/ruby-rbenv-example/blob/main/.buildkite/pipeline.yml", "Ruby, Bundler and RSpec in Gemfile; uses RSpec's default spec directory", "    - bundle install\n    - bundle exec rspec\n"},
	{"empty", "Empty starter (edit before building)", "", "Add your own steps before building", ""},
}

// Recommended uses project evidence, not just a language filename.
func Recommended(root string) string {
	if exists(root, "go.mod") {
		return "go"
	}
	if _, err := nodeCommands(root); err == nil {
		return "node"
	}
	pipfile, _ := os.ReadFile(filepath.Join(root, "Pipfile"))
	if exists(root, "Pipfile.lock") && regexp.MustCompile(`(?ms)^\[dev-packages\][^\[]*?^\s*["']?pytest["']?\s*=`).Match(pipfile) {
		return "pipenv"
	}
	gemfile, _ := os.ReadFile(filepath.Join(root, "Gemfile"))
	if exists(root, "Gemfile.lock") && regexp.MustCompile(`(?m)^\s*gem\s+["']rspec["']`).Match(gemfile) {
		return "ruby"
	}
	return "empty"
}

func exists(root, path string) bool {
	info, err := os.Stat(filepath.Join(root, path))
	return err == nil && !info.IsDir()
}

func nodeCommands(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return "", fmt.Errorf("node starter requires package.json: %w", err)
	}
	var pkg struct {
		Scripts        map[string]string `json:"scripts"`
		PackageManager string            `json:"packageManager"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", fmt.Errorf("reading package.json: %w", err)
	}
	if strings.TrimSpace(pkg.Scripts["test"]) == "" || strings.Contains(pkg.Scripts["test"], "Error: no test specified") {
		return "", fmt.Errorf("node starter requires a test script in package.json")
	}
	manager := ""
	locked := false
	for _, candidate := range []struct {
		name  string
		locks []string
	}{
		{"npm", []string{"package-lock.json", "npm-shrinkwrap.json"}},
		{"pnpm", []string{"pnpm-lock.yaml"}},
		{"yarn", []string{"yarn.lock"}},
		{"bun", []string{"bun.lock", "bun.lockb"}},
	} {
		for _, lock := range candidate.locks {
			if exists(root, lock) {
				if manager != "" && manager != candidate.name {
					return "", fmt.Errorf("conflicting package-manager lockfiles; keep the lockfile for the intended manager")
				}
				manager, locked = candidate.name, true
			}
		}
	}
	declared, _, _ := strings.Cut(pkg.PackageManager, "@")
	if declared != "" {
		if manager != "" && manager != declared {
			return "", fmt.Errorf("packageManager conflicts with the package-manager lockfile")
		}
		manager = declared
	}
	if manager == "" {
		manager = "npm"
	}
	install := manager + " install"
	if locked {
		switch manager {
		case "npm":
			install = "npm ci"
		case "pnpm", "bun":
			install += " --frozen-lockfile"
		case "yarn":
			lock, _ := os.ReadFile(filepath.Join(root, "yarn.lock"))
			if strings.Contains(string(lock), "__metadata:") {
				install += " --immutable"
			} else {
				install += " --frozen-lockfile"
			}
		}
	}
	switch manager {
	case "npm", "pnpm", "yarn":
		return fmt.Sprintf("    - %s\n    - %s test\n", install, manager), nil
	case "bun":
		return fmt.Sprintf("    - %s\n    - bun run test\n", install), nil
	default:
		return "", fmt.Errorf("unsupported package manager %q", manager)
	}
}

// Ensure preserves existing files and otherwise writes a reviewed starter.
// An empty template selects the recommendation, optionally offering alternatives.
// Queue applies only to newly generated files, never existing user configuration.
func Ensure(root, template, queue string, interactive bool, out io.Writer) error {
	path, err := Find(root)
	if err != nil {
		return err
	}
	if path != "" {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Pipeline file found at %s; leaving it unchanged.\n", filepath.ToSlash(relative))
		return nil
	}
	if template == "" {
		template = Recommended(root)
		if interactive {
			var options []string
			var choices []starter
			for _, s := range starters {
				if s.id == template {
					choices = append(choices, s)
				}
			}
			for _, s := range starters {
				if s.id != template {
					choices = append(choices, s)
				}
			}
			for i, s := range choices {
				label := s.label + " — requires: " + s.requires
				if i == 0 {
					label += " (recommended)"
				}
				options = append(options, label)
			}
			choice, err := bkIO.PromptForOneTo(out, "starter", options, false)
			if err != nil {
				return err
			}
			for i, option := range options {
				if choice == option {
					template = choices[i].id
				}
			}
		}
	}
	var content string
	for _, s := range starters {
		if s.id != template {
			continue
		}
		commands := s.commands
		if s.id == "empty" {
			content = "# Add your own steps before building.\nsteps: []\n"
			break
		}
		if s.id == "node" {
			commands, err = nodeCommands(root)
			if err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "Starter requires: %s. Verify these on your agents; agent presence does not prove runtime availability.\n", s.requires)
		if s.id == "pipenv" && exists(root, "Pipfile.lock") {
			commands = strings.Replace(commands, "install --dev", "install --deploy --dev", 1)
		}
		content = fmt.Sprintf("# Adapted from %s\n# Requires: %s.\nsteps:\n  - label: \"Tests\"\n    commands:\n%s", s.source, s.requires, commands)
	}
	if content == "" {
		return fmt.Errorf("unknown starter %q; choose go, node, pipenv, ruby or empty", template)
	}
	if queue != "" {
		quoted, _ := json.Marshal(queue)
		content += fmt.Sprintf("agents:\n  queue: %s\n", quoted)
	}
	path = filepath.Join(root, ".buildkite", "pipeline.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// O_EXCL also protects a file created between detection and writing.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o660)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Fprintln(out, "Pipeline file created at .buildkite/pipeline.yaml. Review it, then commit and push it before building; remote builds cannot see local files.")
	return nil
}
