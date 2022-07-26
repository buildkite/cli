package cli

import (
	"path/filepath"

	"github.com/buildkite/cli/git"
	"github.com/skratchdot/open-golang/open"
)

type BrowseCommandContext struct {
	TerminalContext
	ConfigContext

	Debug     bool
	DebugHTTP bool

	Dir    string
	Branch string
}

func BrowseCommand(ctx BrowseCommandContext) error {
	dir, err := filepath.Abs(ctx.Dir)
	if err != nil {
		return NewExitError(err, 1)
	}

	branch := ctx.Branch
	if branch == "" {
		var gitErr error
		branch, gitErr = git.Branch(dir)
		if gitErr != nil {
			return NewExitError(err, 1)
		}
	}

	gitRemote, err := git.Remote(dir)
	if err != nil {
		return NewExitError(err, 1)
	}

	bk, err := ctx.BuildkiteGraphQLClient()
	if err != nil {
		return NewExitError(err, 1)
	}

	allPipelines, err := listPipelines(bk)
	if err != nil {
		return NewExitError(err, 1)
	}

	ps := pipelineSelect{
		Pipelines: allPipelines,
		Filter: func(p pipeline) bool {
			return git.MatchRemotes(p.RepositoryURL, gitRemote)
		},
	}

	pipeline, err := ps.Run()
	if err != nil {
		return NewExitError(err, 1)
	}

	pipelineURL := pipeline.URL

	if pipelineURL != "" {
		pipelineURL += "/builds?branch=" + branch
	}

	if err := open.Run(pipelineURL); err != nil {
		return NewExitError(err, 1)
	}

	return nil
}
