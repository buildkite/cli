package cli

import (
	"fmt"
	"path/filepath"

	"github.com/buildkite/cli/git"
	"github.com/buildkite/cli/github"
	"github.com/buildkite/cli/graphql"

	"github.com/fatih/color"
)

type CreateBuildCommandContext struct {
	TerminalContext
	KeyringContext

	Debug     bool
	DebugHTTP bool

	Dir      string
	Pipeline string

	Branch  string
	Commit  string
	Message string
}

func CreateBuildCommand(ctx CreateBuildCommandContext) error {
	params := buildkiteBuildParams{
		Slug:    ctx.Pipeline,
		Branch:  ctx.Branch,
		Commit:  ctx.Commit,
		Message: ctx.Message,
	}

	if ctx.Pipeline == "" {
		if err := loadBuildParamsFromDir(ctx, &params); err != nil {
			return NewExitError(err, 1)
		}
	}

	bk, err := ctx.BuildkiteGraphQLClient()
	if err != nil {
		ctx.Failure(err.Error())
		return NewExitError(err, 1)
	}

	if params.Branch == "" {
		params.Branch = `master`
	}

	if params.Commit == "" {
		params.Commit = `HEAD`
	}

	if params.Message == "" {
		params.Message = `Build triggered with bk cli :rocket:`
	}

	debugf("Build will use branch=%q, commit=%q, message=%q",
		params.Branch, params.Commit, params.Message)

	buildTry := ctx.Try()
	buildTry.Start(fmt.Sprintf("Triggering a build on %s", params.Slug))

	build, err := createBuildkiteBuild(bk, params)
	if err != nil {
		buildTry.Failure(err.Error())
		return NewExitError(err, 1)
	}

	buildTry.Success(fmt.Sprintf("Created #%d", build.Number))

	ctx.Printf(color.GreenString("\nCheck out your build at %s 🚀\n"), build.URL)
	return nil
}

func loadBuildParamsFromDir(ctx CreateBuildCommandContext, params *buildkiteBuildParams) error {
	dir, err := filepath.Abs(ctx.Dir)
	if err != nil {
		return NewExitError(err, 1)
	}

	pipelineTry := ctx.Try()
	pipelineTry.Start("Detecting buildkite pipeline from dir")

	gitRemote, err := git.Remote(dir)
	if err != nil {
		pipelineTry.Failure(err.Error())
		return NewExitError(err, 1)
	}

	org, repo, err := github.ParseGithubRemote(gitRemote)
	if err != nil {
		pipelineTry.Failure(err.Error())
		return NewExitError(err, 1)
	}

	params.Slug = fmt.Sprintf("%s/%s", org, repo)
	pipelineTry.Success(params.Slug)

	if params.Branch == "" {
		var gitErr error
		params.Branch, gitErr = git.Branch(dir)
		if gitErr != nil {
			return NewExitError(err, 1)
		}
	}

	if params.Commit == "" {
		var gitErr error
		params.Commit, gitErr = git.Commit(dir)
		if gitErr != nil {
			return NewExitError(err, 1)
		}
	}

	if params.Message == "" {
		var gitErr error
		params.Message, gitErr = git.Message(dir)
		if gitErr != nil {
			return NewExitError(err, 1)
		}
	}

	return nil
}

func getBuildkitePipelineID(client *graphql.Client, slug string) (string, error) {
	resp, err := client.Do(`
		query($slug:ID!) {
			pipeline(slug: $slug) {
				id
			}
		}
	`, map[string]interface{}{
		"slug": slug,
	})
	if err != nil {
		return "", err
	}

	var parsedResp struct {
		Data struct {
			Pipeline struct {
				ID string `json:"id"`
			} `json:"pipeline"`
		} `json:"data"`
	}

	if err = resp.DecodeInto(&parsedResp); err != nil {
		return "", fmt.Errorf("Failed to parse GraphQL response: %v", err)
	}

	if parsedResp.Data.Pipeline.ID == "" {
		return "", fmt.Errorf("Failed to find pipeline id for %s", slug)
	}

	return parsedResp.Data.Pipeline.ID, nil
}

type buildkiteBuildDetails struct {
	URL    string
	Number int
}

type buildkiteBuildParams struct {
	Slug    string
	Commit  string
	Branch  string
	Message string
}

func createBuildkiteBuild(client *graphql.Client, params buildkiteBuildParams) (buildkiteBuildDetails, error) {
	pipelineID, err := getBuildkitePipelineID(client, params.Slug)
	if err != nil {
		return buildkiteBuildDetails{}, err
	}

	resp, err := client.Do(`
		mutation($input: BuildCreateInput!) {
			buildCreate(input: $input) {
				build {
					url
					number
				}
			}
		}
	`, map[string]interface{}{
		"input": map[string]interface{}{
			"pipelineID": pipelineID,
			"message":    params.Message,
			"commit":     params.Commit,
			"branch":     params.Branch,
		}})
	if err != nil {
		return buildkiteBuildDetails{}, err
	}

	var parsedResp struct {
		Data struct {
			BuildCreate struct {
				Build struct {
					URL    string `json:"url"`
					Number int    `json:"number"`
				} `json:"build"`
			} `json:"buildCreate"`
		} `json:"data"`
	}

	if err = resp.DecodeInto(&parsedResp); err != nil {
		return buildkiteBuildDetails{},
			fmt.Errorf("Failed to parse GraphQL response: %v", err)
	}

	return buildkiteBuildDetails{
		URL:    parsedResp.Data.BuildCreate.Build.URL,
		Number: parsedResp.Data.BuildCreate.Build.Number,
	}, nil
}
