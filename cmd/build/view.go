package build

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/alecthomas/kong"
	buildResolver "github.com/buildkite/cli/v3/internal/build/resolver"
	"github.com/buildkite/cli/v3/internal/build/resolver/options"
	"github.com/buildkite/cli/v3/internal/build/view"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	pipelineResolver "github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/pkg/browser"
)

type ViewCmd struct {
	BuildNumber string `arg:"" optional:"" help:"Build number to view (omit for most recent build)"`
	Pipeline    string `help:"The pipeline to use. This can be a {pipeline slug} or in the format {org slug}/{pipeline slug}." short:"p"`
	Branch      string `help:"Filter builds to this branch." short:"b"`
	User        string `help:"Filter builds to this user. You can use name or email." short:"u" xor:"userfilter"`
	Mine        bool   `help:"Filter builds to only my user." xor:"userfilter"`
	Web         bool   `help:"Open the build in a web browser." short:"w"`
	Output      string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}" enum:"json,yaml,text"`
}

func (c *ViewCmd) Help() string {
	return `You can pass an optional build number to view. If omitted, the most recent build on the current branch will be resolved.

Examples:
  # By default, the most recent build for the current branch is shown
  $ bk build view

  # If not inside a repository or to use a specific pipeline, pass -p
  $ bk build view -p monolith

  # To view a specific build
  $ bk build view 429

  # Add -w to any command to open the build in your web browser instead
  $ bk build view -w 429

  # To view the most recent build on feature-x branch
  $ bk build view -b feature-y

  # You can filter by a user name or id
  $ bk build view -u "alice"

  # A shortcut to view your builds is --mine
  $ bk build view --mine

  # You can combine most of these flags
  # To view most recent build by greg on the deploy-pipeline
  $ bk build view -p deploy-pipeline -u "greg"`
}

func (c *ViewCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New()
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	var opts view.ViewOptions
	opts.Pipeline = c.Pipeline
	opts.Web = c.Web

	// Resolve pipeline first
	pipelineRes := pipelineResolver.NewAggregateResolver(
		pipelineResolver.ResolveFromFlag(opts.Pipeline, f.Config),
		pipelineResolver.ResolveFromConfig(f.Config, pipelineResolver.PickOneWithFactory(f)),
		pipelineResolver.ResolveFromRepository(f, pipelineResolver.CachedPicker(f.Config, pipelineResolver.PickOneWithFactory(f), f.GitRepository != nil)),
	)

	// Resolve build options
	optionsResolver := options.AggregateResolver{
		options.ResolveBranchFromFlag(c.Branch),
		options.ResolveBranchFromRepository(f.GitRepository),
	}.WithResolverWhen(
		c.User != "",
		options.ResolveUserFromFlag(c.User),
	).WithResolverWhen(
		c.Mine || c.User == "",
		options.ResolveCurrentUser(ctx, f),
	)

	// Resolve build
	args := []string{}
	if c.BuildNumber != "" {
		args = []string{c.BuildNumber}
	}
	buildRes := buildResolver.NewAggregateResolver(
		buildResolver.ResolveFromPositionalArgument(args, 0, pipelineRes.Resolve, f.Config),
		buildResolver.ResolveBuildWithOpts(f, pipelineRes.Resolve, optionsResolver...),
	)

	bld, err := buildRes.Resolve(ctx)
	if err != nil {
		return err
	}
	if bld == nil {
		fmt.Println("No build found.")
		return nil
	}

	opts.Organization = bld.Organization
	opts.Pipeline = bld.Pipeline
	opts.BuildNumber = bld.BuildNumber

	if err := opts.Validate(); err != nil {
		return err
	}

	if opts.Web {
		buildURL := fmt.Sprintf("https://buildkite.com/%s/%s/builds/%d",
			opts.Organization, opts.Pipeline, opts.BuildNumber)
		fmt.Printf("Opening %s in your browser\n", buildURL)
		return browser.OpenURL(buildURL)
	}

	var build buildkite.Build
	var artifacts []buildkite.Artifact
	var annotations []buildkite.Annotation
	var wg sync.WaitGroup
	var mu sync.Mutex

	spinErr := bkIO.SpinWhile(f, "Loading build information", func() {
		wg.Add(3)
		go func() {
			defer wg.Done()
			var apiErr error
			build, _, apiErr = f.RestAPIClient.Builds.Get(
				ctx,
				opts.Organization,
				opts.Pipeline,
				fmt.Sprint(opts.BuildNumber),
				nil,
			)
			if apiErr != nil {
				mu.Lock()
				err = apiErr
				mu.Unlock()
			}
		}()

		go func() {
			defer wg.Done()
			var apiErr error
			artifacts, _, apiErr = f.RestAPIClient.Artifacts.ListByBuild(
				ctx,
				opts.Organization,
				opts.Pipeline,
				fmt.Sprint(opts.BuildNumber),
				nil,
			)
			if apiErr != nil {
				mu.Lock()
				err = apiErr
				mu.Unlock()
			}
		}()

		go func() {
			defer wg.Done()
			var apiErr error
			annotations, _, apiErr = f.RestAPIClient.Annotations.ListByBuild(
				ctx,
				opts.Organization,
				opts.Pipeline,
				fmt.Sprint(opts.BuildNumber),
				nil,
			)
			if apiErr != nil {
				mu.Lock()
				err = apiErr
				mu.Unlock()
			}
		}()

		wg.Wait()
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return err
	}

	// Create a combined view for JSON/YAML output
	type BuildOutput struct {
		buildkite.Build
		Artifacts   []buildkite.Artifact   `json:"artifacts,omitempty"`
		Annotations []buildkite.Annotation `json:"annotations,omitempty"`
	}

	buildOutput := output.Viewable[BuildOutput]{
		Data: BuildOutput{
			Build:       build,
			Artifacts:   artifacts,
			Annotations: annotations,
		},
		Render: func(b BuildOutput) string {
			return view.NewBuildView(&b.Build, b.Artifacts, b.Annotations, opts.Organization, opts.Pipeline).Render()
		},
	}

	format := output.Format(c.Output)
	if format == output.FormatText {
		writer, cleanup := bkIO.Pager(f.NoPager)
		defer func() { _ = cleanup() }()

		_, err := fmt.Fprint(writer, buildOutput.TextOutput())
		return err
	}

	return output.Write(os.Stdout, buildOutput, format)
}
