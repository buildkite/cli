package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildkite/cli/v3/internal/build"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func NewCmdBuildDownload(f *factory.Factory) *cobra.Command {
	var mine bool
	var branch, pipeline, user string

	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "download [number] [flags]",
		Short:                 "Download resources for a build",
		Long:                  "Download allows you to download resources for a build.",
		Args:                  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// we find the pipeline based on the following rules:
			// 1. an explicit flag is passed
			// 2. a configured pipeline for this directory
			// 3. find pipelines matching the current repository from the API
			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromFlag(pipeline, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne)),
			)

			// we resolve a build based on the following rules:
			// 1. an optional argument
			// 2. resolve from API using some context
			//    a. filter by branch if --branch or use current repo
			//    b. filter by user if --user or --mine given
			optionsResolver := options.AggregateResolver{
				options.ResolveBranchFromFlag(branch),
				options.ResolveBranchFromRepository(f.GitRepository),
			}.WithResolverWhen(
				user != "",
				options.ResolveUserFromFlag(user),
			).WithResolverWhen(
				mine || user == "",
				options.ResolveCurrentUser(f),
			)
			buildRes := buildResolver.NewAggregateResolver(
				buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
				buildResolver.ResolveBuildWithOpts(f, pipelineRes.Resolve, optionsResolver...),
			)

			bld, err := buildRes.Resolve(cmd.Context())
			if err != nil {
				return err
			}
			if bld == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "No build found.\n")
				return nil
			}

			var dir string
			spinErr := spinner.New().
				Title("Downloading build resources").
				Action(func() {
					dir, err = download(cmd.Context(), *bld, f)
				}).
				Run()
			if spinErr != nil {
				return spinErr
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Downloaded build to: %s\n", dir)

			return err
		},
	}

	cmd.Flags().BoolVarP(&mine, "mine", "m", false, "Filter builds to only my user.")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Filter builds to this branch.")
	cmd.Flags().StringVarP(&user, "user", "u", "", "Filter builds to this user. You can use name or email.")
	cmd.Flags().StringVarP(&pipeline, "pipeline", "p", "", "The pipeline to view. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}.\n"+
		"If omitted, it will be resolved using the current directory.",
	)
	// can only supply --user or --mine
	cmd.MarkFlagsMutuallyExclusive("mine", "user")
	cmd.Flags().SortFlags = false

	return &cmd
}

func download(ctx context.Context, build build.Build, f *factory.Factory) (string, error) {
	b, _, err := f.RestAPIClient.Builds.Get(build.Organization, build.Pipeline, fmt.Sprint(build.BuildNumber), nil)
	if err != nil {
		return "", err
	}
	if b == nil {
		return "", fmt.Errorf("could not find build for %s #%d", build.Pipeline, build.BuildNumber)
	}

	directory := fmt.Sprintf("build-%s", *b.ID)
	err = os.MkdirAll(directory, os.ModePerm)
	if err != nil {
		return "", err
	}

	eg, _ := errgroup.WithContext(ctx)

	for _, job := range b.Jobs {
		job := job
		// only script (command) jobs will have logs
		if job == nil || *job.Type != "script" {
			continue
		}

		eg.Go(func() error {
			log, _, err := f.RestAPIClient.Jobs.GetJobLog(build.Organization, build.Pipeline, *b.ID, *job.ID)
			if err != nil {
				return err
			}
			if log == nil {
				return fmt.Errorf("could not get logs for job %s", *job.ID)
			}

			err = os.WriteFile(filepath.Join(directory, *job.ID), []byte(*log.Content), 0o644)
			if err != nil {
				return err
			}
			return nil
		})
	}

	artifacts, _, err := f.RestAPIClient.Artifacts.ListByBuild(build.Organization, build.Pipeline, fmt.Sprint(build.BuildNumber), nil)
	if err != nil {
		return "", err
	}

	for _, artifact := range artifacts {
		artifact := artifact
		eg.Go(func() error {
			out, err := os.Create(filepath.Join(directory, fmt.Sprintf("artifact-%s-%s", *artifact.ID, *artifact.Filename)))
			if err != nil {
				return err
			}
			_, err = f.RestAPIClient.Artifacts.DownloadArtifactByURL(*artifact.DownloadURL, out)
			if err != nil {
				return err
			}
			return nil
		})
	}

	err = eg.Wait()
	if err != nil {
		return "", err
	}

	return directory, nil
}
