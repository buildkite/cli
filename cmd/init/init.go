package init

import (
	"os"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/internal/config"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/pipeline/setup"
	git "github.com/go-git/go-git/v5"
)

type InitCmd struct {
	Template string `help:"Starter template: go, node, pipenv, ruby or empty (defaults to project recommendation)" enum:",go,node,pipenv,ruby,empty" default:""`
}

func (c *InitCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	if err != nil && err != git.ErrRepositoryNotExists {
		return err
	}
	root, err := setup.Root(repo)
	if err != nil {
		return err
	}
	conf := config.New(nil, repo)
	interactive := !globals.DisableInput() && !conf.NoInput() && !bkIO.HasDataAvailable(os.Stdin)
	return setup.Ensure(root, c.Template, interactive, kongCtx.Stdout)
}
