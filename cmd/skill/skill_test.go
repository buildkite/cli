package skill

import (
	"archive/zip"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

func TestInstallSkillsToTargets(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "skills.zip")
	createZip(t, archive, map[string]string{
		"skills-main/skills/buildkite-api/SKILL.md":    "# Buildkite API",
		"skills-main/skills/buildkite-api/docs/ref.md": "reference",
		"skills-main/skills/.bk-skill/SKILL.md":        "# Buildkite API",
		"skills-main/skills/.bk-skill/docs/ref.md":     "reference",
	})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, archive)
	}))
	defer server.Close()
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.ServerName = "example.com"
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	defer transport.CloseIdleConnections()
	originalTransport := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	for _, tc := range []struct {
		name        string
		force       bool
		missing     bool
		crossDevice bool
		update      bool
	}{
		{name: "add"},
		{name: "force", force: true},
		{name: "missing preserves installed skill", force: true, missing: true},
		{name: "cross-device add", crossDevice: true},
		{name: "cross-device force", force: true, crossDevice: true},
		{name: "update ignores abandoned staging", force: true, update: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.crossDevice {
				tmp, err := os.MkdirTemp("/dev/shm", "bk-skill-test-*")
				if err != nil {
					t.Skipf("cross-device temporary directory unavailable: %v", err)
				}
				t.Cleanup(func() { os.RemoveAll(tmp) })
				probe := filepath.Join(tmp, "probe")
				if err := os.WriteFile(probe, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(probe, filepath.Join(root, "probe")); !errors.Is(err, syscall.EXDEV) {
					t.Skipf("temporary directory is not on a separate filesystem: %v", err)
				}
				t.Setenv("TMPDIR", tmp)
			}
			name := "buildkite-api"
			if tc.missing {
				name = "missing"
			} else if tc.update {
				// A dot-prefixed skill is valid; only staging directories
				// with the full .bk-skill- prefix should be ignored.
				name = ".bk-skill"
			}
			targets := []target{
				{agent: "claude", root: filepath.Join(root, ".claude")},
				{agent: "custom", skillsDir: filepath.Join(root, "custom-skills")},
			}
			for _, target := range targets {
				if tc.force {
					dest := filepath.Join(target.SkillsDir(), name)
					if err := os.MkdirAll(dest, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dest, "old.md"), []byte("old skill"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				if tc.update {
					if err := os.Mkdir(filepath.Join(target.SkillsDir(), ".bk-skill-abandoned"), 0o700); err != nil {
						t.Fatal(err)
					}
				}
			}
			var err error
			if tc.update {
				for _, target := range targets {
					cmd := UpdateCmd{Path: target.SkillsDir(), Repo: "buildkite/skills", Branch: "main"}
					if err = cmd.Run(); err != nil {
						break
					}
				}
			} else {
				err = installSkillToTargets(name, targets, tc.force, "buildkite/skills", "main")
			}
			if tc.missing {
				if err == nil || !strings.Contains(err.Error(), `skill "missing" not found`) {
					t.Fatalf("expected missing skill error, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			for _, target := range targets {
				if tc.missing {
					got, err := os.ReadFile(filepath.Join(target.SkillsDir(), name, "old.md"))
					if err != nil || string(got) != "old skill" {
						t.Fatalf("existing skill changed: content = %q, error = %v", got, err)
					}
				} else {
					for path, want := range map[string]string{"SKILL.md": "# Buildkite API", "docs/ref.md": "reference"} {
						got, err := os.ReadFile(filepath.Join(target.SkillsDir(), name, path))
						if err != nil || string(got) != want {
							t.Fatalf("%s: content = %q, want %q, error = %v", path, got, want, err)
						}
					}
					if _, err := os.Stat(filepath.Join(target.SkillsDir(), name, "old.md")); !os.IsNotExist(err) {
						t.Fatalf("obsolete file remains: %v", err)
					}
				}
				entries, err := os.ReadDir(target.SkillsDir())
				wantEntries := 1
				if tc.update {
					wantEntries = 2
				}
				if err != nil || len(entries) != wantEntries || entries[0].Name() != name {
					t.Fatalf("unexpected skills directory contents: %v, error = %v", entries, err)
				}
				if tc.update && entries[1].Name() != ".bk-skill-abandoned" {
					t.Fatalf("abandoned staging directory changed: %v", entries)
				}
			}
		})
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
