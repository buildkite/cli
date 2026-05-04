package skill

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTargetDetectsProjectAgent(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(".cursor", 0o755); err != nil {
		t.Fatal(err)
	}

	target, err := resolveTarget("", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.agent != "cursor" {
		t.Fatalf("agent = %q, want cursor", target.agent)
	}
	if want := filepath.Join(wd, ".cursor", "skills"); target.SkillsDir() != want {
		t.Fatalf("skills dir = %q, want %q", target.SkillsDir(), want)
	}
}

func TestResolveTargetErrorsWithoutProjectAgent(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveTarget("", false, ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveTargetUsesCustomPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "amp-skills")
	target, err := resolveTarget("", false, dir)
	if err != nil {
		t.Fatal(err)
	}
	if target.agent != "custom" {
		t.Fatalf("agent = %q, want custom", target.agent)
	}
	want, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if target.SkillsDir() != want {
		t.Fatalf("skills dir = %q, want %q", target.SkillsDir(), want)
	}
}

func TestResolveTargetsGlobalUsesAllExistingAgentDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}

	targets, err := resolveTargets("", true, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("got %d targets, want 2", len(targets))
	}
	if targets[0].agent != "claude" || targets[1].agent != "cursor" {
		t.Fatalf("targets = %#v, want claude then cursor", targets)
	}
}

func TestResolveTargetsGlobalDoesNotCreateAgentDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := resolveTargets("", true, ""); err == nil {
		t.Fatal("expected error")
	}
	if dirExists(filepath.Join(home, ".claude")) || dirExists(filepath.Join(home, ".cursor")) {
		t.Fatal("global detection created agent directories")
	}
}

func TestValidateSkillNameRejectsPathsURLsAndPatterns(t *testing.T) {
	for _, name := range []string{"../skill", "skills/buildkite-api", "https://example.com/skill", "buildkite-*", ""} {
		if err := validateSkillName(name); err == nil {
			t.Fatalf("validateSkillName(%q) succeeded, want error", name)
		}
	}
}

func TestDeleteErrorsWhenSkillIsNotInstalled(t *testing.T) {
	dir := t.TempDir()
	cmd := DeleteCmd{Name: "missing", Path: dir}
	if err := cmd.Run(); err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractSkill(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "skills.zip")
	createZip(t, archive, map[string]string{
		"skills-main/skills/buildkite-api/SKILL.md":    "# Buildkite API",
		"skills-main/skills/buildkite-api/docs/ref.md": "reference",
		"skills-main/skills/other/SKILL.md":            "# Other",
	})

	dest := filepath.Join(t.TempDir(), "buildkite-api")
	if err := extractSkill(archive, "buildkite-api", dest); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Buildkite API" {
		t.Fatalf("SKILL.md = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "docs", "ref.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "..", "other")); err == nil {
		t.Fatal("extracted another skill")
	}
}

func createZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
