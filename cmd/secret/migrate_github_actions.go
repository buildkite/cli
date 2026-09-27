package secret

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

const (
	maxMigrationFileBytes = 1 << 20
	maxMigrationSecrets   = 40
)

var migrationGrantIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,200}$`)

type MigrateGitHubActionsCmd struct {
	Prepare MigrateGitHubActionsPrepareCmd `cmd:"" help:"Prepare a reviewed GitHub Actions migration workflow."`
	Run     MigrateGitHubActionsRunCmd     `cmd:"" help:"Verify and dispatch a prepared migration workflow."`
}

type MigrateGitHubActionsPrepareCmd struct {
	Organization string   `help:"Destination Buildkite organization slug."`
	Cluster      string   `help:"Assert the destination cluster UUID."`
	Pipeline     string   `help:"Destination Buildkite pipeline slug."`
	PolicyFile   string   `help:"Buildkite secret access policy YAML file." type:"path"`
	SecretNames  []string `help:"Exact GitHub Actions secret name to migrate (repeatable)." name:"secret"`
	Matches      []string `help:"Glob matching GitHub Actions secret names (repeatable)." name:"match"`
	Output       string   `help:"Write the workflow without replacing an existing file." type:"path"`
}

type MigrateGitHubActionsRunCmd struct {
	Workflow string `help:"Prepared workflow committed to the repository default branch." required:"" type:"path"`
}

type githubRepository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Owner         struct {
		ID int64 `json:"id"`
	} `json:"owner"`
}

type migrationGrantRequest struct {
	Policy            string   `json:"policy"`
	SecretNames       []string `json:"secret_names"`
	RepositoryID      int64    `json:"repository_id"`
	RepositoryOwnerID int64    `json:"repository_owner_id"`
	WorkflowPath      string   `json:"workflow_path"`
	DefaultBranchRef  string   `json:"default_branch_ref"`
	WorkflowSHA       string   `json:"workflow_sha"`
}

type migrationGrant struct {
	ID           string `json:"id"`
	MigrationURL string `json:"migration_url"`
	Audience     string `json:"audience"`
}

type githubCommandRunner interface {
	Run(context.Context, []string, []byte) ([]byte, error)
}

type execGitHubRunner struct {
	stderr io.Writer
}

func (r execGitHubRunner) Run(ctx context.Context, args []string, stdin []byte) ([]byte, error) {
	command := exec.CommandContext(ctx, "gh", args...)
	command.Stdin = bytes.NewReader(stdin)
	command.Stderr = r.stderr
	output, err := command.Output()
	if err != nil {
		return output, err
	}
	return output, nil
}

type migrationBuildkiteClient interface {
	GetPipeline(context.Context, string, string) (buildkite.Pipeline, error)
	ListPipelines(context.Context, string, string, int) ([]buildkite.Pipeline, error)
	ListSecrets(context.Context, string, string, int) ([]buildkite.ClusterSecret, error)
	CreateGrant(context.Context, string, string, migrationGrantRequest) (migrationGrant, error)
}

type migrationBuildkiteAPI struct {
	client *buildkite.Client
}

func (api migrationBuildkiteAPI) GetPipeline(ctx context.Context, organization, pipeline string) (buildkite.Pipeline, error) {
	result, _, err := api.client.Pipelines.Get(ctx, organization, pipeline)
	return result, err
}

func (api migrationBuildkiteAPI) ListPipelines(ctx context.Context, organization, repository string, page int) ([]buildkite.Pipeline, error) {
	result, _, err := api.client.Pipelines.List(ctx, organization, &buildkite.PipelineListOptions{
		Repository:  repository,
		ListOptions: buildkite.ListOptions{Page: page, PerPage: 100},
	})
	return result, err
}

func (api migrationBuildkiteAPI) ListSecrets(ctx context.Context, organization, cluster string, page int) ([]buildkite.ClusterSecret, error) {
	result, _, err := api.client.ClusterSecrets.List(ctx, organization, cluster, &buildkite.ClusterSecretsListOptions{
		ListOptions: buildkite.ListOptions{Page: page, PerPage: 100},
	})
	return result, err
}

func (api migrationBuildkiteAPI) CreateGrant(ctx context.Context, organization, cluster string, input migrationGrantRequest) (migrationGrant, error) {
	endpoint := fmt.Sprintf("v2/organizations/%s/clusters/%s/github-actions-secret-migrations", url.PathEscape(organization), url.PathEscape(cluster))
	request, err := api.client.NewRequest(ctx, http.MethodPost, endpoint, input)
	if err != nil {
		return migrationGrant{}, err
	}
	request.Header.Set("Accept", "application/json")
	var grant migrationGrant
	_, err = api.client.Do(request, &grant)
	return grant, err
}

func (c *MigrateGitHubActionsPrepareCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	if err := c.validate(); err != nil {
		return err
	}
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()), factory.WithOrgOverride(c.Organization))
	if err != nil {
		return err
	}
	if err := validation.ValidateConfigurationForOrg(f.Config, kongCtx.Command(), c.Organization); err != nil {
		return err
	}
	organization := c.Organization
	if organization == "" {
		organization = f.Config.OrganizationSlug()
	}
	return c.prepare(context.Background(), organization, globals.DisableInput(), migrationBuildkiteAPI{client: f.RestAPIClient}, execGitHubRunner{stderr: os.Stderr}, bufio.NewReader(os.Stdin), os.Stdout, os.Stderr)
}

func (c *MigrateGitHubActionsRunCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	if err := validateMigrationWorkflowPath(c.Workflow); err != nil {
		return fmt.Errorf("--workflow: %w", err)
	}
	workflow, err := readMigrationWorkflow(c.Workflow)
	if err != nil {
		return err
	}
	manifest, err := decodeMigrationManifest(workflow)
	if err != nil {
		return err
	}
	// The reviewed manifest owns the destination. Resolve that organization's
	// credential rather than silently using whichever organization is selected.
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()), factory.WithOrgOverride(manifest.Organization))
	if err != nil {
		return err
	}
	if err := validation.ValidateConfigurationForOrg(f.Config, kongCtx.Command(), manifest.Organization); err != nil {
		return err
	}
	return c.run(context.Background(), migrationBuildkiteAPI{client: f.RestAPIClient}, execGitHubRunner{stderr: os.Stderr}, os.Stdout)
}

func (c *MigrateGitHubActionsPrepareCmd) validate() error {
	if c.Organization != "" && !organizationSlugPattern.MatchString(c.Organization) {
		return errors.New("--organization must be a lowercase Buildkite organization slug")
	}
	if c.Cluster != "" && !uuidPattern.MatchString(c.Cluster) {
		return errors.New("--cluster must be a Buildkite cluster UUID")
	}
	if c.Pipeline != "" && !organizationSlugPattern.MatchString(c.Pipeline) {
		return errors.New("--pipeline must be a lowercase Buildkite pipeline slug")
	}
	if c.PolicyFile == "-" {
		return errors.New("--policy-file must be a file path, not stdin")
	}
	for _, pattern := range c.Matches {
		if _, err := path.Match(pattern, "NAME"); err != nil {
			return fmt.Errorf("invalid --match glob %q: %w", pattern, err)
		}
	}
	if c.Output != "" {
		if err := validateMigrationWorkflowPath(c.Output); err != nil {
			return fmt.Errorf("--output: %w", err)
		}
	}
	return nil
}

func (c *MigrateGitHubActionsPrepareCmd) prepare(ctx context.Context, organization string, noInput bool, bk migrationBuildkiteClient, gh githubCommandRunner, input *bufio.Reader, stdout, stderr io.Writer) error {
	repository, err := inspectGitHubRepository(ctx, gh)
	if err != nil {
		return err
	}
	availableNames, err := listGitHubSecretNames(ctx, gh)
	if err != nil {
		return err
	}
	if len(availableNames) == 0 {
		return errors.New("the GitHub repository has no Actions secrets")
	}
	selectedNames, err := selectMigrationSecretNames(availableNames, c.SecretNames, c.Matches, noInput, input, stderr)
	if err != nil {
		return err
	}
	pipeline, err := selectBuildkitePipeline(ctx, bk, organization, c.Pipeline, repository.FullName, noInput, input, stderr)
	if err != nil {
		return err
	}
	if !uuidPattern.MatchString(pipeline.ClusterID) {
		return fmt.Errorf("buildkite pipeline %s does not report a cluster UUID", pipeline.Slug)
	}
	if c.Cluster != "" && c.Cluster != pipeline.ClusterID {
		return fmt.Errorf("--cluster %s does not match pipeline %s cluster %s", c.Cluster, pipeline.Slug, pipeline.ClusterID)
	}

	policy := "- pipeline_id: " + pipeline.ID + "\n"
	if c.PolicyFile != "" {
		policy, err = readSecretsPolicy(c.PolicyFile)
		if err != nil {
			return err
		}
	}
	if err := rejectExistingBuildkiteSecrets(ctx, bk, organization, pipeline.ClusterID, selectedNames); err != nil {
		return err
	}
	manifest := migrationManifest{
		Version: 1, Organization: organization, Cluster: pipeline.ClusterID, Policy: policy,
		Repository: repository.FullName, RepositoryID: repository.ID, RepositoryOwnerID: repository.Owner.ID,
		DefaultBranch: repository.DefaultBranch, SecretNames: selectedNames,
	}
	workflow, err := renderOIDCSecretsMigrationWorkflow(manifest)
	if err != nil {
		return err
	}
	if c.Output == "" {
		_, err = stdout.Write(workflow)
		return err
	}
	file, err := os.OpenFile(c.Output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create workflow: %w", err)
	}
	_, writeErr := file.Write(workflow)
	writeErr = errors.Join(writeErr, file.Close())
	if writeErr != nil {
		_ = os.Remove(c.Output)
		return fmt.Errorf("write workflow: %w", writeErr)
	}
	fmt.Fprintf(stdout, "Wrote %s with %d explicitly named secrets. Review and commit it to %s, then run:\n  bk secret migrate github-actions run --workflow %s\n", c.Output, len(selectedNames), repository.DefaultBranch, c.Output)
	return nil
}

func inspectGitHubRepository(ctx context.Context, runner githubCommandRunner) (githubRepository, error) {
	output, err := runner.Run(ctx, []string{"api", "repos/{owner}/{repo}"}, nil)
	if err != nil {
		return githubRepository{}, fmt.Errorf("inspect GitHub repository with gh: %w", err)
	}
	var repository githubRepository
	if err := json.Unmarshal(output, &repository); err != nil {
		return repository, fmt.Errorf("decode gh repository response: %w", err)
	}
	if repository.ID == 0 || repository.Owner.ID == 0 || repository.FullName == "" || repository.DefaultBranch == "" {
		return repository, errors.New("gh repository response is missing identity or default branch")
	}
	if !strings.EqualFold(strings.TrimSuffix(repository.HTMLURL, "/"), "https://github.com/"+repository.FullName) {
		return repository, errors.New("GitHub Enterprise Server repositories are not supported")
	}
	return repository, nil
}

func listGitHubSecretNames(ctx context.Context, runner githubCommandRunner) ([]string, error) {
	output, err := runner.Run(ctx, []string{"api", "--paginate", "--jq", ".secrets[].name", "repos/{owner}/{repo}/actions/secrets?per_page=100"}, nil)
	if err != nil {
		return nil, fmt.Errorf("list GitHub Actions secrets with gh: %w", err)
	}
	names := strings.Fields(string(output))
	for _, name := range names {
		if !githubSecretPattern.MatchString(name) {
			return nil, fmt.Errorf("gh returned invalid secret name %q", name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

func selectMigrationSecretNames(available, explicit, matches []string, noInput bool, input *bufio.Reader, prompt io.Writer) ([]string, error) {
	availableSet := make(map[string]bool, len(available))
	for _, name := range available {
		availableSet[strings.ToUpper(name)] = true
	}
	selected := map[string]bool{}
	for _, name := range explicit {
		name = strings.ToUpper(name)
		if !availableSet[name] {
			return nil, fmt.Errorf("GitHub Actions secret %q does not exist", name)
		}
		selected[name] = true
	}
	for _, pattern := range matches {
		matched := false
		for name := range availableSet {
			if ok, _ := path.Match(strings.ToUpper(pattern), name); ok {
				selected[name], matched = true, true
			}
		}
		if !matched {
			return nil, fmt.Errorf("--match %q selected no GitHub Actions secrets", pattern)
		}
	}
	if len(explicit) == 0 && len(matches) == 0 {
		if noInput {
			return nil, errors.New("--secret or --match is required when --no-input is set")
		}
		fmt.Fprintln(prompt, "GitHub Actions repository secrets:")
		for index, name := range available {
			fmt.Fprintf(prompt, "  %d) %s\n", index+1, name)
		}
		fmt.Fprint(prompt, "Select secrets by number (comma-separated), or type all: ")
		line, err := input.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("read secret selection: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "all" {
			for name := range availableSet {
				selected[name] = true
			}
		} else {
			for _, field := range strings.Split(line, ",") {
				index, parseErr := strconv.Atoi(strings.TrimSpace(field))
				if parseErr != nil || index < 1 || index > len(available) {
					return nil, fmt.Errorf("invalid secret selection %q", strings.TrimSpace(field))
				}
				selected[strings.ToUpper(available[index-1])] = true
			}
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("at least one secret must be selected")
	}
	names := make([]string, 0, len(selected))
	for name := range selected {
		if err := validateMigrationSecretName(name); err != nil {
			return nil, err
		}
		if name == "GITHUB_TOKEN" {
			return nil, errors.New("GITHUB_TOKEN cannot be migrated because it is a permission-scoped workflow token, not a repository secret")
		}
		names = append(names, name)
	}
	slices.Sort(names)
	if len(names) > maxMigrationSecrets {
		return nil, fmt.Errorf("at most %d secrets can be migrated at once; use multiple workflows for larger sets", maxMigrationSecrets)
	}
	return names, nil
}

func selectBuildkitePipeline(ctx context.Context, client migrationBuildkiteClient, organization, requested, repository string, noInput bool, input *bufio.Reader, prompt io.Writer) (buildkite.Pipeline, error) {
	if requested != "" {
		pipeline, err := client.GetPipeline(ctx, organization, requested)
		if err != nil {
			return pipeline, fmt.Errorf("inspect Buildkite pipeline: %w", err)
		}
		if !uuidPattern.MatchString(pipeline.ID) || pipeline.Slug != requested {
			return pipeline, fmt.Errorf("buildkite returned an invalid pipeline for %q", requested)
		}
		return pipeline, nil
	}
	var pipelines []buildkite.Pipeline
	for page := 1; len(pipelines) < 3000; page++ {
		batch, err := client.ListPipelines(ctx, organization, repository, page)
		if err != nil {
			return buildkite.Pipeline{}, fmt.Errorf("list Buildkite pipelines: %w", err)
		}
		pipelines = append(pipelines, batch...)
		if len(batch) < 100 {
			break
		}
	}
	for index := len(pipelines) - 1; index >= 0; index-- {
		pipeline := pipelines[index]
		if !uuidPattern.MatchString(pipeline.ID) || pipeline.Slug == "" || !uuidPattern.MatchString(pipeline.ClusterID) || !buildkitePipelineMatchesGitHubRepository(pipeline.Repository, repository) {
			pipelines = slices.Delete(pipelines, index, index+1)
		}
	}
	if len(pipelines) == 0 {
		return buildkite.Pipeline{}, fmt.Errorf("no Buildkite pipeline found for repository %s; pass --pipeline", repository)
	}
	slices.SortFunc(pipelines, func(left, right buildkite.Pipeline) int { return strings.Compare(left.Slug, right.Slug) })
	if len(pipelines) == 1 {
		fmt.Fprintf(prompt, "Using Buildkite pipeline %s.\n", pipelines[0].Slug)
		return pipelines[0], nil
	}
	if noInput {
		return buildkite.Pipeline{}, errors.New("multiple Buildkite pipelines match; pass --pipeline when --no-input is set")
	}
	fmt.Fprintln(prompt, "Buildkite pipelines:")
	for index, pipeline := range pipelines {
		fmt.Fprintf(prompt, "  %d) %s\n", index+1, pipeline.Slug)
	}
	fmt.Fprint(prompt, "Select a pipeline by number: ")
	line, readErr := input.ReadString('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return buildkite.Pipeline{}, fmt.Errorf("read pipeline selection: %w", readErr)
	}
	index, parseErr := strconv.Atoi(strings.TrimSpace(line))
	if parseErr != nil || index < 1 || index > len(pipelines) {
		return buildkite.Pipeline{}, fmt.Errorf("invalid pipeline selection %q", strings.TrimSpace(line))
	}
	return pipelines[index-1], nil
}

func buildkitePipelineMatchesGitHubRepository(pipelineRepository, repository string) bool {
	normalized := strings.TrimSuffix(pipelineRepository, ".git")
	for _, prefix := range []string{"https://github.com/", "http://github.com/", "ssh://git@github.com/", "git@github.com:"} {
		if strings.HasPrefix(normalized, prefix) {
			return strings.EqualFold(strings.TrimPrefix(normalized, prefix), repository)
		}
	}
	return false
}

func rejectExistingBuildkiteSecrets(ctx context.Context, client migrationBuildkiteClient, organization, cluster string, selected []string) error {
	existing := map[string]bool{}
	for page := 1; ; page++ {
		secrets, err := client.ListSecrets(ctx, organization, cluster, page)
		if err != nil {
			return fmt.Errorf("list Buildkite secrets: %w", err)
		}
		for _, secret := range secrets {
			existing[strings.ToUpper(secret.Key)] = true
		}
		if len(secrets) < 100 {
			break
		}
	}
	var conflicts []string
	for _, name := range selected {
		if existing[name] {
			conflicts = append(conflicts, name)
		}
	}
	if len(conflicts) != 0 {
		return fmt.Errorf("destination keys already exist and will not be overwritten: %s", strings.Join(conflicts, ", "))
	}
	return nil
}

func validateMigrationWorkflowPath(workflowPath string) error {
	clean := filepath.ToSlash(filepath.Clean(workflowPath))
	if clean != workflowPath || !strings.HasPrefix(clean, ".github/workflows/") || strings.Count(strings.TrimPrefix(clean, ".github/workflows/"), "/") != 0 {
		return errors.New("workflow must be a clean path directly under .github/workflows")
	}
	extension := filepath.Ext(clean)
	if extension != ".yml" && extension != ".yaml" {
		return errors.New("workflow must have a .yml or .yaml extension")
	}
	return nil
}

func (c *MigrateGitHubActionsRunCmd) run(ctx context.Context, bk migrationBuildkiteClient, gh githubCommandRunner, stdout io.Writer) error {
	workflow, err := readMigrationWorkflow(c.Workflow)
	if err != nil {
		return err
	}
	manifest, err := decodeMigrationManifest(workflow)
	if err != nil {
		return err
	}
	expectedWorkflow, err := renderOIDCSecretsMigrationWorkflow(manifest)
	if err != nil || !bytes.Equal(workflow, expectedWorkflow) {
		return errors.New("workflow differs from the deterministic secrets migration output")
	}
	repository, err := inspectGitHubRepository(ctx, gh)
	if err != nil {
		return err
	}
	if repository.ID != manifest.RepositoryID || repository.Owner.ID != manifest.RepositoryOwnerID || repository.FullName != manifest.Repository || repository.DefaultBranch != manifest.DefaultBranch {
		return errors.New("workflow migration manifest does not match the current GitHub repository")
	}
	remoteWorkflow, commitSHA, err := readDefaultBranchWorkflow(ctx, gh, repository, c.Workflow)
	if err != nil {
		return err
	}
	if !bytes.Equal(workflow, remoteWorkflow) {
		return errors.New("local workflow differs from the file on the GitHub default branch")
	}
	grant, err := bk.CreateGrant(ctx, manifest.Organization, manifest.Cluster, migrationGrantRequest{
		Policy: manifest.Policy, SecretNames: manifest.SecretNames, RepositoryID: manifest.RepositoryID,
		RepositoryOwnerID: manifest.RepositoryOwnerID, WorkflowPath: c.Workflow,
		DefaultBranchRef: "refs/heads/" + manifest.DefaultBranch, WorkflowSHA: commitSHA,
	})
	if err != nil {
		return fmt.Errorf("create Buildkite migration grant: %w", err)
	}
	if !migrationGrantIDPattern.MatchString(grant.ID) {
		return errors.New("buildkite returned an invalid migration grant")
	}
	expectedURL := fmt.Sprintf("https://api.buildkite.com/v2/organizations/%s/clusters/%s/github-actions-secret-migrations/%s/secrets", manifest.Organization, manifest.Cluster, grant.ID)
	if grant.MigrationURL != expectedURL || grant.Audience != expectedURL {
		return errors.New("buildkite returned an invalid migration grant URL or audience")
	}
	dispatchInput, _ := json.Marshal(map[string]string{"grant_id": grant.ID})
	dispatchOutput, err := gh.Run(ctx, []string{"workflow", "run", c.Workflow, "--repo", manifest.Repository, "--ref", manifest.DefaultBranch, "--json"}, dispatchInput)
	if err != nil {
		return fmt.Errorf("dispatch migration workflow with gh: %w; one-use grant %s will expire unused, then rerun this command to create a replacement", err, grant.ID)
	}
	workflowName := strings.TrimPrefix(c.Workflow, ".github/workflows/")
	actionsURL := fmt.Sprintf("https://github.com/%s/actions/workflows/%s", manifest.Repository, url.PathEscape(workflowName))
	if runURL := strings.TrimSpace(string(dispatchOutput)); strings.HasPrefix(runURL, "https://github.com/"+manifest.Repository+"/actions/runs/") && !strings.ContainsAny(runURL, "\r\n") {
		actionsURL = runURL
	}
	fmt.Fprintf(stdout, "Dispatch accepted for %s on %s with %d secrets. The migration is not complete until the GitHub Actions run succeeds.\nActions: %s\nGrant: %s (short-lived and one-use; if it expires or the run fails before consuming it, rerun this command for a replacement)\nAfter the run succeeds, remove the workflow from the default branch.\n", c.Workflow, manifest.DefaultBranch, len(manifest.SecretNames), actionsURL, grant.ID)
	return nil
}

func readMigrationWorkflow(workflowPath string) ([]byte, error) {
	file, err := os.Open(workflowPath)
	if err != nil {
		return nil, fmt.Errorf("read workflow: %w", err)
	}
	workflow, readErr := io.ReadAll(io.LimitReader(file, maxMigrationFileBytes+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, fmt.Errorf("read workflow: %w", err)
	}
	if len(workflow) > maxMigrationFileBytes {
		return nil, fmt.Errorf("workflow exceeds %d bytes", maxMigrationFileBytes)
	}
	return bytes.ReplaceAll(workflow, []byte("\r\n"), []byte("\n")), nil
}

func readDefaultBranchWorkflow(ctx context.Context, runner githubCommandRunner, repository githubRepository, workflowPath string) ([]byte, string, error) {
	commitEndpoint := fmt.Sprintf("repos/%s/commits/%s", repository.FullName, url.PathEscape(repository.DefaultBranch))
	commitOutput, err := runner.Run(ctx, []string{"api", "--jq", ".sha", commitEndpoint}, nil)
	if err != nil {
		return nil, "", fmt.Errorf("resolve GitHub default-branch commit with gh: %w", err)
	}
	commitSHA := strings.TrimSpace(string(commitOutput))
	if len(commitSHA) != 40 {
		return nil, "", errors.New("GitHub returned an invalid default-branch commit")
	}
	workflowName := strings.TrimPrefix(workflowPath, ".github/workflows/")
	endpoint := fmt.Sprintf("repos/%s/contents/.github/workflows/%s?ref=%s", repository.FullName, url.PathEscape(workflowName), url.QueryEscape(commitSHA))
	output, err := runner.Run(ctx, []string{"api", endpoint}, nil)
	if err != nil {
		return nil, "", fmt.Errorf("read workflow from GitHub default branch with gh: %w", err)
	}
	var content struct {
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(output, &content); err != nil || content.Encoding != "base64" {
		return nil, "", errors.New("GitHub returned an invalid workflow file")
	}
	workflow, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content.Content, "\n", ""))
	if err != nil {
		return nil, "", errors.New("GitHub returned an invalid workflow file")
	}
	return workflow, commitSHA, nil
}
