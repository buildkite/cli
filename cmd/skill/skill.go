package skill

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultRepo   = "buildkite/skills"
	defaultBranch = "main"
)

type AddCmd struct {
	Name   string `arg:"" help:"Name of the Buildkite skill to install (for example, buildkite-api)."`
	Agent  string `help:"Agent/harness to install for (claude or cursor). Auto-detected from .claude or .cursor by default." optional:""`
	Global bool   `help:"Install globally in your home directory instead of the current project."`
	Path   string `help:"Custom skills directory to install into, for agents such as Amp or Pi." type:"path" optional:"" aliases:"to,location"`
	Force  bool   `help:"Overwrite an existing installed skill."`
	Repo   string `help:"GitHub repository to install skills from." default:"${skill_repo}" hidden:""`
	Branch string `help:"Git branch to install skills from." default:"${skill_branch}" hidden:""`
}

func (c *AddCmd) Help() string {
	return `Install a Buildkite skill from github.com/buildkite/skills.

By default, the target is auto-detected from a project .claude or .cursor
folder. Use --agent to choose a target explicitly, --global to install to all
existing global agent directories (~/.claude and/or ~/.cursor), or --path for
another agent's skills directory.

Examples:
  # Install buildkite-api into the detected project agent
  $ bk skill add buildkite-api

  # Install for Claude Code in this project
  $ bk skill add buildkite-api --agent claude

  # Install globally for Cursor
  $ bk skill add buildkite-api --agent cursor --global

  # Install into a custom skills directory, such as Amp or Pi
  $ bk skill add buildkite-api --path ~/.amp/skills
`
}

func (c *AddCmd) Run() error {
	if err := validateSkillName(c.Name); err != nil {
		return err
	}
	return installSkill(c.Name, c.Agent, c.Global, c.Path, c.Force, c.Repo, c.Branch)
}

type UpdateCmd struct {
	Name   string `arg:"" help:"Name of the installed Buildkite skill to update. If omitted, all installed skills are updated." optional:""`
	Agent  string `help:"Agent/harness to update for (claude or cursor). Auto-detected from .claude or .cursor by default." optional:""`
	Global bool   `help:"Update the globally installed skill instead of the current project."`
	Path   string `help:"Custom skills directory to update, for agents such as Amp or Pi." type:"path" optional:""`
	Repo   string `help:"GitHub repository to install skills from." default:"${skill_repo}" hidden:""`
	Branch string `help:"Git branch to install skills from." default:"${skill_branch}" hidden:""`
}

func (c *UpdateCmd) Help() string {
	return `Update installed Buildkite skills from github.com/buildkite/skills.

If no skill name is provided, all currently installed skills for the target agent
are updated.

Examples:
  $ bk skill update
  $ bk skill update buildkite-api
  $ bk skill update buildkite-api --agent claude --global
  $ bk skill update --path ~/.amp/skills
`
}

func (c *UpdateCmd) Run() error {
	if c.Name != "" {
		if err := validateSkillName(c.Name); err != nil {
			return err
		}
	}

	targets, err := resolveTargets(c.Agent, c.Global, c.Path)
	if err != nil {
		return err
	}

	if c.Name != "" {
		var installedTargets []target
		for _, target := range targets {
			if info, err := os.Stat(filepath.Join(target.SkillsDir(), c.Name)); err == nil && info.IsDir() {
				installedTargets = append(installedTargets, target)
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if len(installedTargets) == 0 {
			return fmt.Errorf("skill %q is not installed for any selected target", c.Name)
		}
		return installSkillToTargets(c.Name, installedTargets, true, c.Repo, c.Branch)
	}

	plan := map[string][]target{}
	for _, target := range targets {
		entries, err := os.ReadDir(target.SkillsDir())
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				plan[entry.Name()] = append(plan[entry.Name()], target)
			}
		}
	}
	if len(plan) == 0 {
		return fmt.Errorf("no skills are installed for any selected target")
	}

	return installSkillsToTargets(plan, true, c.Repo, c.Branch)
}

type DeleteCmd struct {
	Name   string `arg:"" help:"Name of the installed Buildkite skill to delete."`
	Agent  string `help:"Agent/harness to delete from (claude or cursor). Auto-detected from .claude or .cursor by default." optional:""`
	Global bool   `help:"Delete the globally installed skill instead of the current project."`
	Path   string `help:"Custom skills directory to delete from, for agents such as Amp or Pi." type:"path" optional:""`
}

func (c *DeleteCmd) Help() string {
	return `Delete an installed Buildkite skill.

Examples:
  $ bk skill delete buildkite-api
  $ bk skill delete buildkite-api --agent cursor --global
  $ bk skill delete buildkite-api --path ~/.amp/skills
`
}

func (c *DeleteCmd) Run() error {
	if err := validateSkillName(c.Name); err != nil {
		return err
	}
	targets, err := resolveTargets(c.Agent, c.Global, c.Path)
	if err != nil {
		return err
	}

	var installedTargets []target
	for _, target := range targets {
		dest := filepath.Join(target.SkillsDir(), c.Name)
		if info, err := os.Stat(dest); err == nil && info.IsDir() {
			installedTargets = append(installedTargets, target)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if len(installedTargets) == 0 {
		return fmt.Errorf("skill %q is not installed for any selected target", c.Name)
	}

	for _, target := range installedTargets {
		dest := filepath.Join(target.SkillsDir(), c.Name)
		if err := os.RemoveAll(dest); err != nil {
			return fmt.Errorf("deleting skill %q: %w", c.Name, err)
		}
		fmt.Printf("Deleted %s skill %q from %s\n", target.agent, c.Name, dest)
	}
	return nil
}

type target struct {
	agent     string
	root      string
	skillsDir string
}

func (t target) SkillsDir() string {
	if t.skillsDir != "" {
		return t.skillsDir
	}
	return filepath.Join(t.root, "skills")
}

func resolveTarget(agent string, global bool, customPath string) (target, error) {
	targets, err := resolveTargets(agent, global, customPath)
	if err != nil {
		return target{}, err
	}
	return targets[0], nil
}

func resolveTargets(agent string, global bool, customPath string) ([]target, error) {
	if global && customPath != "" {
		return nil, fmt.Errorf("--global and --path cannot be used together")
	}
	if customPath == "" && agent != "" && agent != "claude" && agent != "cursor" {
		return nil, fmt.Errorf("unsupported --agent %q (expected claude or cursor, or use --path for a custom agent)", agent)
	}

	if customPath != "" {
		abs, err := filepath.Abs(customPath)
		if err != nil {
			return nil, err
		}
		if agent == "" {
			agent = "custom"
		}
		return []target{{agent: agent, skillsDir: abs}}, nil
	}

	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		if agent != "" {
			root := filepath.Join(home, "."+agent)
			if !dirExists(root) {
				return nil, fmt.Errorf("global %s directory does not exist at %s", agent, root)
			}
			return []target{{agent: agent, root: root}}, nil
		}

		var targets []target
		for _, candidate := range []string{"claude", "cursor"} {
			root := filepath.Join(home, "."+candidate)
			if dirExists(root) {
				targets = append(targets, target{agent: candidate, root: root})
			}
		}
		if len(targets) == 0 {
			return nil, fmt.Errorf("no global agent directories found at ~/.claude or ~/.cursor")
		}
		return targets, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if agent == "" {
		if dirExists(filepath.Join(wd, ".claude")) {
			agent = "claude"
		} else if dirExists(filepath.Join(wd, ".cursor")) {
			agent = "cursor"
		} else {
			return nil, fmt.Errorf("could not detect an agent target: create .claude or .cursor, or pass --agent claude|cursor")
		}
	}
	return []target{{agent: agent, root: filepath.Join(wd, "."+agent)}}, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func installSkill(name, agent string, global bool, customPath string, force bool, repo, branch string) error {
	targets, err := resolveTargets(agent, global, customPath)
	if err != nil {
		return err
	}
	return installSkillToTargets(name, targets, force, repo, branch)
}

func installSkillToTargets(name string, targets []target, force bool, repo, branch string) error {
	return installSkillsToTargets(map[string][]target{name: targets}, force, repo, branch)
}

func installSkillsToTargets(plan map[string][]target, force bool, repo, branch string) error {
	for name, targets := range plan {
		if err := validateSkillName(name); err != nil {
			return err
		}
		for _, target := range targets {
			dest := filepath.Join(target.SkillsDir(), name)
			if !force {
				if _, err := os.Stat(dest); err == nil {
					return fmt.Errorf("skill %q is already installed at %s (use --force or bk skill update)", name, dest)
				} else if !os.IsNotExist(err) {
					return err
				}
			}
		}
	}

	tmpDir, err := os.MkdirTemp("", "bk-skill-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archive := filepath.Join(tmpDir, "skills.zip")
	if err := downloadRepoArchive(repo, branch, archive); err != nil {
		return err
	}

	counter := 0
	for name, targets := range plan {
		for _, target := range targets {
			dest := filepath.Join(target.SkillsDir(), name)
			extracted := filepath.Join(tmpDir, fmt.Sprintf("%s-%d", name, counter))
			counter++
			if err := extractSkill(archive, name, extracted); err != nil {
				return err
			}
			if err := os.MkdirAll(target.SkillsDir(), 0o755); err != nil {
				return err
			}
			if err := os.RemoveAll(dest); err != nil {
				return err
			}
			if err := os.Rename(extracted, dest); err != nil {
				return err
			}
			fmt.Printf("Installed %s skill %q to %s\n", target.agent, name, dest)
		}
	}
	return nil
}

func validateSkillName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid skill name %q", name)
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("invalid skill name %q: use a literal skill name, not a path, URL, or pattern", name)
	}
	return nil
}

func downloadRepoArchive(repo, branch, dest string) error {
	url := fmt.Sprintf("https://codeload.github.com/%s/zip/refs/heads/%s", repo, branch)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: unexpected HTTP status %s", url, resp.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func extractSkill(archive, skillName, dest string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("opening downloaded skills archive: %w", err)
	}
	defer r.Close()

	found := false
	for _, f := range r.File {
		parts := strings.SplitN(f.Name, "/", 4)
		var rel string
		switch {
		case len(parts) >= 3 && parts[1] == skillName:
			rel = parts[2]
		case len(parts) >= 4 && parts[1] == "skills" && parts[2] == skillName:
			rel = parts[3]
		default:
			continue
		}
		if rel == "" {
			continue
		}
		found = true
		path := filepath.Join(dest, filepath.FromSlash(rel))
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("archive contains invalid path %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if !found {
		return fmt.Errorf("skill %q not found in github.com/%s", skillName, defaultRepo)
	}
	return nil
}
