package options

import (
	"os/exec"
	"path/filepath"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v4"
	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
)

func TestResolveBranchFromGitFallback(t *testing.T) {
	repo := testRepository(t, "https://github.com/buildkite/cli.git")
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree returned error: %v", err)
	}
	root := wt.Filesystem.Root()
	t.Chdir(root)

	commitFile := filepath.Join(root, "README.md")
	if err := exec.Command("sh", "-c", "printf 'hello\n' > README.md").Run(); err != nil {
		t.Fatalf("creating file returned error: %v", err)
	}
	if err := exec.Command("git", "add", filepath.Base(commitFile)).Run(); err != nil {
		t.Fatalf("git add returned error: %v", err)
	}
	if err := exec.Command("git", "-c", "user.name=Person Example", "-c", "user.email=person@example.com", "commit", "-m", "initial").Run(); err != nil {
		t.Fatalf("git commit returned error: %v", err)
	}
	if err := exec.Command("git", "checkout", "-b", "feature/test").Run(); err != nil {
		t.Fatalf("git checkout returned error: %v", err)
	}

	options := &buildkite.BuildsListOptions{}
	err = ResolveBranchFromRepository(nil)(options)
	if err != nil {
		t.Fatalf("ResolveBranchFromRepository returned error: %v", err)
	}
	if len(options.Branch) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(options.Branch))
	}
	if options.Branch[0] != "feature/test" {
		t.Fatalf("expected branch feature/test, got %q", options.Branch[0])
	}
}

func testRepository(t *testing.T, remoteURLs ...string) *git.Repository {
	t.Helper()

	repo, err := git.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit returned error: %v", err)
	}
	if len(remoteURLs) == 0 {
		return repo
	}

	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: remoteURLs})
	if err != nil {
		t.Fatalf("CreateRemote returned error: %v", err)
	}

	return repo
}
