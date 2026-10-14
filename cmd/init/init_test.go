package init

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	git "github.com/go-git/go-git/v5"
)

func TestInitFromSubdirectory(t *testing.T) {
	for _, linked := range []bool{false, true} {
		name := "repository"
		if linked {
			name = "linked worktree"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if _, err := git.PlainInit(root, false); err != nil {
				t.Fatal(err)
			}
			if linked {
				worktree := filepath.Join(t.TempDir(), "worktree")
				for _, args := range [][]string{
					{"-C", root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "Fixture"},
					{"-C", root, "worktree", "add", "--detach", worktree, "HEAD"},
				} {
					if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
						t.Fatalf("git fixture: %s: %v", out, err)
					}
				}
				root = worktree
			}
			sub := filepath.Join(root, "src")
			if err := os.Mkdir(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Chdir(sub)
			var out bytes.Buffer
			cmd := InitCmd{Template: "empty"}
			ctx := &kong.Context{Kong: &kong.Kong{Stdout: &out}}
			if err := cmd.Run(ctx, cli.Globals{NoInput: true}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, ".buildkite", "pipeline.yaml")
			data, err := os.ReadFile(path)
			if err != nil || !bytes.Contains(data, []byte("steps: []")) {
				t.Fatalf("expected empty starter at repository root, got %q, %v", data, err)
			}
			cmd.Template = "go"
			if err := cmd.Run(ctx, cli.Globals{NoInput: true}); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatalf("existing file changed: %q, %v", after, err)
			}
			if _, err := os.Stat(filepath.Join(sub, ".buildkite")); !os.IsNotExist(err) {
				t.Fatalf("init wrote inside subdirectory: %v", err)
			}
		})
	}
}
