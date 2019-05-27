package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/manifoldco/promptui"

	"github.com/fatih/color"

	"github.com/buildkite/cli/github"
	"github.com/buildkite/cli/graphql"
	githubclient "github.com/google/go-github/github"
)

type RotateWebhooksCommandContext struct {
	TerminalContext
	KeyringContext

	Debug     bool
	DebugHTTP bool

	Prompt   bool
	DryRun   bool
	Pipeline string
	OrgSlug  string
}

func RotateWebhookCommand(ctx RotateWebhooksCommandContext) error {
	gh, err := ctx.GithubClient()
	if err != nil {
		return NewExitError(err, 1)
	}

	bk, err := ctx.BuildkiteGraphQLClient()
	if err != nil {
		return NewExitError(err, 1)
	}

	pipelines, err := listPipelines(bk)
	if err != nil {
		return NewExitError(err, 1)
	}

	var lineCounter int

	// Loop through all pipelines available
	for _, pipeline := range pipelines {
		if ctx.OrgSlug != "" && pipeline.Org != ctx.OrgSlug {
			continue
		} else if ctx.Pipeline != "" && pipeline.Slug != ctx.Pipeline {
			continue
		}

		if lineCounter > 0 {
			ctx.Println()
		}
		lineCounter++

		ctx.Header(fmt.Sprintf("Processing pipeline %s/%s", pipeline.Org, pipeline.Slug))
		ctx.Printf("Pipeline URL is https://buildkite.com/%s/%s\n", pipeline.Org, pipeline.Slug)
		ctx.Printf("Pipeline GitHub Settings is at https://buildkite.com/%s/%s/settings/setup/github\n", pipeline.Org, pipeline.Slug)

		ctx.Printf("GitHub Repository URL is %s\n", pipeline.RepositoryURL)

		if github.IsGistRemote(pipeline.RepositoryURL) {
			ctx.Println(color.YellowString("Skipping gist repository ⚠️"))
			continue
		}

		org, repo, err := github.ParseRemote(pipeline.RepositoryURL)
		if err != nil {
			ctx.Printf("Err: %v\n", err)
			continue
		}

		ctx.Printf("GitHub Repository Settings is https://github.com/%s/%s/settings/hooks\n", org, repo)

		githubWebhooksTry := ctx.Try()
		githubWebhooksTry.Start(fmt.Sprintf("Searching github %s/%s for webhooks", org, repo))

		hooks, _, err := gh.Repositories.ListHooks(context.Background(), org, repo, &githubclient.ListOptions{})
		if err != nil {
			githubWebhooksTry.Failure(err.Error())
			continue
		}

		buildkiteWebhooks := []*githubclient.Hook{}
		for _, hook := range hooks {
			wehookURL, ok := hook.Config["url"].(string)
			if ok && strings.Contains(wehookURL, "webhook.buildbox.io") || strings.Contains(wehookURL, "webhook.buildkite.com") {
				buildkiteWebhooks = append(buildkiteWebhooks, hook)
			}
		}

		if len(buildkiteWebhooks) == 0 {
			githubWebhooksTry.Finish(color.YellowString("None found ⚠️"))
			continue
		} else if len(buildkiteWebhooks) == 1 {
			githubWebhooksTry.Finish("Found")
			ctx.Printf("Current Webhook: %s\n", buildkiteWebhooks[0].Config["url"].(string))
		} else {
			githubWebhooksTry.Finish(color.YellowString("Found many ⚠️"))

			ctx.Printf(color.YellowString("\nThe github repository has many webhooks, you will need to manually update them:\n\n"))
			for _, hook := range buildkiteWebhooks {
				ctx.Printf("- %s\n", hook.Config["url"].(string))
			}
			continue
		}

		if ctx.Prompt {
			ctx.Println()

			prompt := promptui.Prompt{
				Label:     fmt.Sprintf("Rotate webhook"),
				IsConfirm: true,
				Default:   "y",
			}

			result, _ := prompt.Run()
			ctx.Println()

			if result == "n" {
				continue
			}
		}

		hook := buildkiteWebhooks[0]

		pipelineID, err := getBuildkitePipelineID(bk, buildkitePipelineSlug(pipeline.Org, pipeline.Slug))
		if err != nil {
			return NewExitError(err, 1)
		}

		rotateBuildkiteWebhookTry := ctx.Try()
		rotateBuildkiteWebhookTry.Start("Rotating buildkite webhook")

		details, err := rotateBuildkiteWebhook(bk, pipelineID)
		if err != nil {
			rotateBuildkiteWebhookTry.Failure(err.Error())
			return NewExitError(err, 1)
		}

		rotateBuildkiteWebhookTry.Finish("Done")

		ctx.Printf("New Buildkite Webhook: %s\n", details.WebhookURL)

		deleteWebhookTry := ctx.Try()
		deleteWebhookTry.Start(fmt.Sprintf("Deleting github webhook #%d", *hook.ID))

		if !ctx.DryRun {
			// https://developer.github.com/v3/repos/hooks/#delete-a-hook
			_, err := gh.Repositories.DeleteHook(context.Background(), org, repo, *hook.ID)
			if err != nil {
				deleteWebhookTry.Failure(err.Error())
				return NewExitError(err, 1)
			}
		}

		deleteWebhookTry.Finish("Done")

		createGithubWebhooksTry := ctx.Try()
		createGithubWebhooksTry.Start("Creating new github webook")

		if !ctx.DryRun {
			// https://developer.github.com/v3/repos/hooks/#create-a-hook
			_, _, err := gh.Repositories.CreateHook(context.Background(), org, repo, &githubclient.Hook{
				Name:   githubclient.String(`web`),
				Events: []string{`push`, `pull_request`, `deployment`},
				Config: map[string]interface{}{
					"url":          details.WebhookURL,
					"content_type": "json",
				},
			})
			if err != nil {
				createGithubWebhooksTry.Failure(err.Error())
				return NewExitError(err, 1)
			}
		}

		createGithubWebhooksTry.Finish("Done")
		ctx.Printf(color.GreenString("%s ✅\n"), "Webhook rotated")
	}

	return nil
}

type buildkiteRotateDetails struct {
	WebhookURL string
}

func rotateBuildkiteWebhook(client *graphql.Client, pipelineID string) (buildkiteRotateDetails, error) {
	resp, err := client.Do(`
		mutation($input: PipelineRotateWebhookURLInput!) {
			pipelineRotateWebhookURL(input: $input) {
				pipeline {
					webhookURL
				}
			}
		}
	`, map[string]interface{}{
		"input": map[string]interface{}{
			"id": pipelineID,
		}})
	if err != nil {
		return buildkiteRotateDetails{}, err
	}

	var parsedResp struct {
		Data struct {
			PipelineRotateWebhookURL struct {
				Pipeline struct {
					WebhookURL string `json:"webhookURL"`
				} `json:"pipeline"`
			} `json:"pipelineRotateWebhookURL"`
		} `json:"data"`
	}

	if err = resp.DecodeInto(&parsedResp); err != nil {
		return buildkiteRotateDetails{},
			fmt.Errorf("Failed to parse GraphQL response: %v", err)
	}

	return buildkiteRotateDetails{
		WebhookURL: parsedResp.Data.PipelineRotateWebhookURL.Pipeline.WebhookURL,
	}, nil
}
