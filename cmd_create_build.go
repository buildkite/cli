package cli

import (
	"fmt"

	"github.com/buildkite/cli/git"
	"github.com/buildkite/cli/graphql"
	"github.com/fatih/color"
)

type CreateBuildCommandContext struct {
	TerminalContext
	KeyringContext

	Debug     bool
	DebugHTTP bool

	Dir          string
	PipelineSlug string

	Branch  string
	Commit  string
	Message string
}

func CreateBuildCommand(ctx CreateBuildCommandContext) error {
	params := buildkiteBuildParams{
		Branch:  ctx.Branch,
		Commit:  ctx.Commit,
		Message: ctx.Message,
	}

	bk, err := ctx.BuildkiteGraphQLClient()
	if err != nil {
		ctx.Failure(err.Error())
		return NewExitError(err, 1)
	}

	pipelineSlug := ctx.PipelineSlug

	if pipelineSlug == "" {
		gitRemote, err := git.Remote(ctx.Dir)
		if err != nil {
			return err
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

		params.PipelineID = pipeline.ID
		pipelineSlug = fmt.Sprintf("%s/%s", pipeline.Org, pipeline.Slug)

		if params.Branch == "" {
			params.Branch, err = git.Branch(ctx.Dir)
			if err != nil {
				return NewExitError(err, 1)
			}
		}

		if params.Commit == "" {
			params.Commit, err = git.Commit(ctx.Dir)
			if err != nil {
				return NewExitError(err, 1)
			}
		}

		if params.Message == "" {
			params.Message, err = git.Message(ctx.Dir)
			if err != nil {
				return NewExitError(err, 1)
			}
		}
	} else {
		params.PipelineID, err = getBuildkitePipelineID(bk, ctx.PipelineSlug)
		if err != nil {
			return NewExitError(err, 1)
		}
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
	buildTry.Start(fmt.Sprintf("Triggering a build on pipeline %s", pipelineSlug))

	build, err := createBuildkiteBuild(bk, params)
	if err != nil {
		buildTry.Failure(err.Error())
		return NewExitError(err, 1)
	}

	buildTry.Success(fmt.Sprintf("Created #%d", build.Number))

	ctx.Printf(color.GreenString("\nCheck out your build at %s 🚀\n"), build.URL)
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
	PipelineID string
	Commit     string
	Branch     string
	Message    string
}

func createBuildkiteBuild(client *graphql.Client, params buildkiteBuildParams) (buildkiteBuildDetails, error) {
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
			"pipelineID": params.PipelineID,
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
