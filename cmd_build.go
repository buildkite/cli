package cli

import (
	"fmt"
	"path/filepath"

	"github.com/buildkite/cli/git"
	"github.com/buildkite/cli/github"
	"github.com/buildkite/cli/graphql"

	"github.com/fatih/color"
)

type BuildCommandContext struct {
	TerminalContext
	KeyringContext

	Debug     bool
	DebugHTTP bool
	Dir       string
}

func BuildCommand(ctx BuildCommandContext) error {
	dir, err := filepath.Abs(ctx.Dir)
	if err != nil {
		return NewExitError(err, 1)
	}

	pipelineTry := ctx.Try()
	pipelineTry.Start("Detecting buildkite pipeline")

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

	bk, err := ctx.BuildkiteGraphQLClient()
	if err != nil {
		pipelineTry.Failure(err.Error())
		return NewExitError(err, 1)
	}

	pipeline, err := getBuildkitePipeline(bk, org, repo)
	if err != nil {
		if err == errPipelineDoesntExist {
			pipelineTry.Failure(fmt.Sprintf("Pipeline doesn't exist! Try `bk init`"))
		}
		return NewExitError(err, 1)
	}

	pipelineTry.Success(pipeline.URL)

	buildTry := ctx.Try()
	buildTry.Start("Triggering a build")

	build, err := createBuildkiteBuild(bk, org, repo, buildkiteBuildParams{})
	if err != nil {
		buildTry.Failure(err.Error())
		return NewExitError(err, 1)
	}

	buildTry.Success(fmt.Sprintf("Created #%d", build.Number))

	ctx.Printf(color.GreenString("\nCheck out your build at %s 🚀\n"), build.URL)
	return nil
}

func getBuildkitePipelineID(client *graphql.Client, org, pipeline string) (string, error) {
	resp, err := client.Do(`
		query($slug:ID!) {
			pipeline(slug: $slug) {
				id
			}
		}
	`, map[string]interface{}{
		"slug": fmt.Sprintf("%s/%s", org, pipeline),
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
		return "", fmt.Errorf("Failed to find pipeline id for %s/%s", org, pipeline)
	}

	return parsedResp.Data.Pipeline.ID, nil
}

type buildkiteBuildDetails struct {
	URL    string
	Number int
}

type buildkiteBuildParams struct {
	URL string
}

func createBuildkiteBuild(client *graphql.Client, org, pipeline string, params buildkiteBuildParams) (buildkiteBuildDetails, error) {
	pipelineID, err := getBuildkitePipelineID(client, org, pipeline)
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
			"message":    ":egg:",
			"commit":     "HEAD",
			"branch":     "master",
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
