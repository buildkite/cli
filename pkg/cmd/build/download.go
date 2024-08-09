package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildkite/cli/v3/internal/build"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func NewCmdBuildDownload(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "download [number [pipeline]] [flags]",
		Short:                 "Download resources for a build",
		Long:                  "Download allows you to download resources for a build.",
		RunE: func(cmd *cobra.Command, args []string) error {
			pipelineRes := pipelineResolver.NewAggregateResolver(
				pipelineResolver.ResolveFromPositionalArgument(args, 1, f.Config),
				pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOne),
				pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOne)),
			)

			buildRes := buildResolver.NewAggregateResolver(
				buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
				buildResolver.ResolveBuildFromCurrentBranch(f.GitRepository, pipelineRes.Resolve, f),
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
