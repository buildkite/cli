package pipeline

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/pipeline"
	"github.com/buildkite/cli/v3/internal/pipeline/resolver"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type CopyCmd struct {
	Pipeline string `arg:"" help:"Source pipeline to copy (slug or org/slug). Uses current pipeline if not specified." optional:""`
	Target   string `help:"Name for the new pipeline, or org/name to copy to a different organization" short:"t"`
	Cluster  string `help:"Cluster name or ID for the new pipeline (required for cross-org copies if target org uses clusters)" short:"c"`
	DryRun   bool   `help:"Show what would be copied without creating the pipeline"`
	output.OutputFlags
}

// we store the target organization and pipeline name for a future go-buildkite call
type copyTarget struct {
	Org  string
	Name string
}

func (c *CopyCmd) Help() string {
	// returns the biggest help message ever seen
	return `Copy an existing pipeline's configuration to create a new pipeline.

This command copies all configuration from a source pipeline including:
- Pipeline steps (YAML configuration)
- Repository settings
- Branch configuration
- Build skipping/cancellation rules
- Provider settings (trigger mode, PR builds, commit statuses, etc.)
- Environment variables
- Tags and visibility

When copying to a different organization, cluster configuration is skipped
(clusters are organization-specific).

Examples:
  # Copy the current pipeline to a new pipeline
  $ bk pipeline cp --target "my-pipeline-v2"

  # Copy a specific pipeline
  $ bk pipeline cp my-existing-pipeline --target "my-new-pipeline"

  # Copy a pipeline from another org (if you have access)
  $ bk pipeline cp other-org/their-pipeline --target "my-copy"

  # Copy to a different organization
  $ bk pipeline cp my-pipeline --target "other-org/my-pipeline" --cluster "8302f0b-9b99-4663-23f3-2d64f88s693e"

  # Interactive mode - prompts for source and target
  $ bk pipeline cp

  # Preview what would be copied without creating
  $ bk pipeline cp my-pipeline --target "copy" --dry-run

  # Output the new pipeline details as JSON
  $ bk pipeline cp my-pipeline -t "new-pipeline" -o json
`
}

func (c *CopyCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	// Resolve source pipeline
	// looks at current project if no source provided, or tries to resolve it using the current selected org
	sourcePipeline, err := c.resolveSourcePipeline(ctx, f)
	if err != nil {
		return err
	}

	// Get target org and name
	// Spoiler: we use `/` as an indicator for org/pipeline split
	target, err := c.resolveTarget(f, sourcePipeline.Name)
	if err != nil {
		return err
	}

	source, err := c.fetchSourcePipeline(ctx, f, sourcePipeline.Org, sourcePipeline.Name)
	if err != nil {
		return err
	}

	// Determine if this is a cross-org copy
	isCrossOrg := target.Org != sourcePipeline.Org

	// Resolve cluster ID - required for cross-org copies
	clusterID, err := c.resolveCluster(f, source.ClusterID, isCrossOrg)
	if err != nil {
		return err
	}

	if c.DryRun {
		return c.runDryRun(kongCtx, f, source, target, isCrossOrg, clusterID)
	}

	return c.runCopy(kongCtx, f, source, target, isCrossOrg, clusterID)
}

func (c *CopyCmd) resolveSourcePipeline(ctx context.Context, f *factory.Factory) (*pipeline.Pipeline, error) {
	var args []string
	if c.Pipeline != "" {
		args = []string{c.Pipeline}
	}

	pipelineRes := resolver.NewAggregateResolver(
		resolver.ResolveFromPositionalArgument(args, 0, f.Config),
		resolver.ResolveFromConfig(f.Config, resolver.PickOneWithFactory(f)),
		resolver.ResolveFromRepository(f, resolver.CachedPicker(f.Config, resolver.PickOneWithFactory(f))),
	)

	p, err := pipelineRes.Resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not resolve source pipeline, ensure correct config is in use (`bk org ls`): %w", err)
	}

	return p, nil
}

func (c *CopyCmd) resolveTarget(f *factory.Factory, sourceName string) (*copyTarget, error) {
	targetStr := c.Target
	if targetStr == "" {
		// Interactive prompt for target name
		defaultName := fmt.Sprintf("%s-copy", sourceName)
		var err error
		targetStr, err = bkIO.PromptForInput("Target pipeline (or org/pipeline)", defaultName, f.NoInput)
		if err != nil {
			return nil, err
		}
	}

	// Parse target - could be "name" or "org/name"
	// we check to see if `/` is present for org name, if not we use the existing org selected
	return parseTarget(targetStr, f.Config.OrganizationSlug()), nil
}

// parseTarget parses a target string into org and name components.
// If no org is specified, defaultOrg is used which is the current selected org.
func parseTarget(target, defaultOrg string) *copyTarget {
	if strings.Contains(target, "/") {
		parts := strings.SplitN(target, "/", 2)
		return &copyTarget{
			Org:  parts[0],
			Name: parts[1],
		}
	}
	return &copyTarget{
		Org:  defaultOrg,
		Name: target,
	}
}

func (c *CopyCmd) resolveCluster(f *factory.Factory, sourceClusterID string, isCrossOrg bool) (string, error) {
	if c.Cluster != "" {
		return c.Cluster, nil
	}

	if !isCrossOrg {
		return sourceClusterID, nil
	}

	return bkIO.PromptForInput("Target cluster ID (required for cross-org copy)", "", f.NoInput)
}

func (c *CopyCmd) fetchSourcePipeline(ctx context.Context, f *factory.Factory, org, slug string) (*buildkite.Pipeline, error) {
	var pipeline buildkite.Pipeline
	var resp *buildkite.Response
	var err error

	spinErr := bkIO.SpinWhile(f, fmt.Sprintf("Fetching pipeline %s/%s", org, slug), func() {
		pipeline, resp, err = f.RestAPIClient.Pipelines.Get(ctx, org, slug)
	})

	if spinErr != nil {
		return nil, spinErr
	}

	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("pipeline %s/%s not found", org, slug)
		}
		return nil, fmt.Errorf("failed to fetch pipeline: %w", err)
	}

	return &pipeline, nil
}

// runDryRun allows a user to validate what their changes will do, based on the current `--dry-run` flag in Create
func (c *CopyCmd) runDryRun(kongCtx *kong.Context, f *factory.Factory, source *buildkite.Pipeline, target *copyTarget, isCrossOrg bool, clusterID string) error {
	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	createReq := c.buildCreatePipeline(source, target.Name, isCrossOrg, clusterID)

	// For dry-run, default to JSON if text format requested
	if format == output.FormatText {
		format = output.FormatJSON
	}

	return output.Write(kongCtx.Stdout, createReq, format)
}

func (c *CopyCmd) runCopy(kongCtx *kong.Context, f *factory.Factory, source *buildkite.Pipeline, target *copyTarget, isCrossOrg bool, clusterID string) error {
	ctx := context.Background()
	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	// For cross-org copies, we need a client authenticated for the target org
	targetClient := f.RestAPIClient
	if isCrossOrg {
		var err error
		targetClient, err = c.getClientForOrg(f, target.Org)
		if err != nil {
			return err
		}
	}

	createReq := c.buildCreatePipeline(source, target.Name, isCrossOrg, clusterID)

	var newPipeline buildkite.Pipeline
	var resp *buildkite.Response
	var err error

	spinErr := bkIO.SpinWhile(f, fmt.Sprintf("Creating pipeline %s/%s", target.Org, target.Name), func() {
		newPipeline, resp, err = targetClient.Pipelines.Create(ctx, target.Org, createReq)
	})

	if spinErr != nil {
		return spinErr
	}

	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnprocessableEntity {
			// Check if a pipeline with this name already exists and error out if it does (not fussed with adding -1, -2 etc)
			if existing := c.findPipelineByName(ctx, targetClient, target); existing != nil {
				return fmt.Errorf("a pipeline with the name '%s' already exists: %s", target.Name, existing.WebURL)
			}
		}
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	if format != output.FormatText {
		return output.Write(kongCtx.Stdout, newPipeline, format)
	}

	fmt.Printf("%s\n", newPipeline.WebURL)
	return nil
}

// getClientForOrg creates a Buildkite client authenticated for the specified organization
func (c *CopyCmd) getClientForOrg(f *factory.Factory, org string) (*buildkite.Client, error) {
	token := f.Config.GetTokenForOrg(org)
	if token == "" {
		return nil, fmt.Errorf("no API token configured for organization %q. Run 'bk configure' to add it", org)
	}

	return buildkite.NewOpts(
		buildkite.WithBaseURL(f.Config.RESTAPIEndpoint()),
		buildkite.WithTokenAuth(token),
	)
}

func (c *CopyCmd) buildCreatePipeline(source *buildkite.Pipeline, targetName string, isCrossOrg bool, clusterID string) buildkite.CreatePipeline {
	create := buildkite.CreatePipeline{
		Name:          targetName,
		Repository:    source.Repository,
		Configuration: source.Configuration,

		// Branch and build settings
		DefaultBranch:                   source.DefaultBranch,
		Description:                     source.Description,
		BranchConfiguration:             source.BranchConfiguration,
		SkipQueuedBranchBuilds:          source.SkipQueuedBranchBuilds,
		SkipQueuedBranchBuildsFilter:    source.SkipQueuedBranchBuildsFilter,
		CancelRunningBranchBuilds:       source.CancelRunningBranchBuilds,
		CancelRunningBranchBuildsFilter: source.CancelRunningBranchBuildsFilter,

		// Visibility and tags
		Visibility: source.Visibility,
		Tags:       source.Tags,

		// Provider settings (trigger mode, PR builds, commit statuses, etc.)
		ProviderSettings: source.Provider.Settings,
	}

	// Use explicit cluster if provided, otherwise copy from source for same-org copies
	if clusterID != "" {
		create.ClusterID = clusterID
	} else if !isCrossOrg {
		create.ClusterID = source.ClusterID
	}

	// Convert environment variables (map[string]any -> map[string]string)
	if len(source.Env) > 0 {
		create.Env = make(map[string]string, len(source.Env))
		for k, v := range source.Env {
			create.Env[k] = fmt.Sprintf("%v", v)
		}
	}

	return create
}

func (c *CopyCmd) findPipelineByName(ctx context.Context, client *buildkite.Client, target *copyTarget) *buildkite.Pipeline {
	opts := buildkite.PipelineListOptions{
		ListOptions: buildkite.ListOptions{
			PerPage: 100,
		},
	}

	pipelines, _, err := client.Pipelines.List(ctx, target.Org, &opts)
	if err != nil {
		return nil
	}

	for _, p := range pipelines {
		if p.Name == target.Name {
			return &p
		}
	}

	return nil
}
