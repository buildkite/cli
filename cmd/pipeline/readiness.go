package pipeline

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/buildkite/cli/v3/cmd/auth"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/pipeline/setup"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func (c *CreateCmd) newFactory(globals cli.GlobalFlags) (*factory.Factory, error) {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()), factory.WithOrgOverride(c.Org), factory.WithReadOnlyAuth(c.DryRun))
	if err != nil {
		return nil, err
	}
	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = f.NoInput || globals.DisableInput()
	f.Quiet = f.Quiet || globals.IsQuiet()
	return f, nil
}

func verifyOrganization(ctx context.Context, f *factory.Factory, slug string) (buildkite.Organization, error) {
	if slug == "" {
		return buildkite.Organization{}, fmt.Errorf("no destination organization; supply --org <slug>")
	}
	if f.Config.APITokenForOrg(slug) == "" {
		return buildkite.Organization{}, fmt.Errorf("no credentials for organization %q; run bk auth login --org %s", slug, slug)
	}
	org, resp, err := f.RestAPIClient.Organizations.Get(ctx, slug)
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound) {
			return org, fmt.Errorf("credentials could not access organization %q; run bk auth login --org %s or check token permissions: %w", slug, slug, err)
		}
		return org, fmt.Errorf("could not verify organization %q: %w", slug, err)
	}
	if org.Slug != slug {
		return org, fmt.Errorf("organization response did not match requested destination %q", slug)
	}
	return org, nil
}

// confirmDestination verifies each target before displaying it. Selection is
// invocation-scoped; even an explicit --org is never replaced without a choice.
func (c *CreateCmd) confirmDestination(ctx context.Context, f *factory.Factory, globals cli.GlobalFlags, interactive bool, out io.Writer) (*factory.Factory, error) {
	c.Org = c.orgSlug(f.Config)
	for {
		org, verifyErr := verifyOrganization(ctx, f, c.Org)
		if !interactive || c.DryRun {
			return f, verifyErr
		}
		options := []string{"Continue", "Choose another already-authenticated organization", "Log in to another organization", "Cancel"}
		if verifyErr == nil {
			fmt.Fprintf(out, "Destination organization: %s (%s)\n", org.Name, org.Slug)
		} else {
			fmt.Fprintln(out, verifyErr)
			options[0] = "Log in to the target organization and resume"
		}
		choice, err := bkIO.PromptForOneTo(out, "destination", options, false)
		if err != nil {
			return nil, err
		}
		switch choice {
		case "Continue":
			return f, nil
		case "Cancel":
			return nil, fmt.Errorf("pipeline creation cancelled; no pipeline files or remote pipeline were created")
		case "Choose another already-authenticated organization":
			var slugs []string
			var labels []string
			candidates := f.Config.ConfiguredOrganizations()
			sort.Strings(candidates)
			for _, slug := range candidates {
				if slug == c.Org || (len(slugs) > 0 && slug == slugs[len(slugs)-1]) {
					continue
				}
				candidate, err := factory.New(factory.WithOrgOverride(slug), factory.WithDebug(f.Debug))
				if err != nil {
					return nil, err
				}
				org, err := verifyOrganization(ctx, candidate, slug)
				if err != nil {
					fmt.Fprintf(out, "Not offering %s: %v\n", slug, err)
					continue
				}
				slugs = append(slugs, slug)
				labels = append(labels, fmt.Sprintf("%s (%s)", org.Name, slug))
			}
			if len(slugs) == 0 {
				fmt.Fprintln(out, "No other organizations with verified credentials; choose login instead.")
				continue
			}
			selected, err := bkIO.PromptForOneTo(out, "organization", labels, false)
			if err != nil {
				return nil, err
			}
			for i, label := range labels {
				if selected == label {
					c.Org = slugs[i]
				}
			}
		default:
			slug := c.Org
			if choice == "Log in to another organization" || slug == "" {
				slug, err = bkIO.PromptForInputTo(out, "Organization slug to log in to", "", false)
				if err != nil {
					return nil, err
				}
			}
			c.Org, err = auth.LoginForOrganization(ctx, f, slug, out)
			if err != nil {
				return nil, err
			}
		}
		// Rebuild both REST and GraphQL clients for the selected identity. Cluster
		// and queue resolution happens only after the final destination is accepted.
		f, err = c.newFactory(globals)
		if err != nil {
			return nil, err
		}
	}
}

func (c *CreateCmd) resolveQueue(ctx context.Context, f *factory.Factory, interactive bool, out io.Writer) error {
	var clusters []buildkite.Cluster
	for page := 1; page != 0; {
		batch, resp, err := f.RestAPIClient.Clusters.List(ctx, c.Org, &buildkite.ClustersListOptions{ListOptions: buildkite.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			return fmt.Errorf("cannot list clusters in %s (requires read_clusters): %w", c.Org, err)
		}
		for _, cluster := range batch {
			if (c.ClusterUUID == "" || c.ClusterUUID == cluster.ID) && (c.ClusterName == "" || c.ClusterName == cluster.Name) {
				clusters = append(clusters, cluster)
			}
		}
		page = resp.NextPage
	}
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].Name+clusters[i].ID < clusters[j].Name+clusters[j].ID })
	if len(clusters) == 0 {
		return fmt.Errorf("no matching cluster in %s; check --cluster-uuid/--cluster-name or ask an administrator to configure a cluster", c.Org)
	}
	cluster := clusters[0]
	if len(clusters) > 1 {
		if !interactive {
			return fmt.Errorf("multiple clusters in %s; supply --cluster-uuid or --cluster-name", c.Org)
		}
		var labels []string
		for _, cluster := range clusters {
			labels = append(labels, fmt.Sprintf("%s (%s)", cluster.Name, cluster.ID))
		}
		label, err := bkIO.PromptForOneTo(out, "cluster", labels, false)
		if err != nil {
			return err
		}
		for i, candidate := range labels {
			if label == candidate {
				cluster = clusters[i]
			}
		}
	}
	name := cluster.Name
	c.ClusterUUID = cluster.ID
	// A previously supplied name has now been resolved for this target.
	c.ClusterName = ""
	var queues []buildkite.ClusterQueue
	for page := 1; page != 0; {
		batch, resp, err := f.RestAPIClient.ClusterQueues.List(ctx, c.Org, c.ClusterUUID, &buildkite.ClusterQueuesListOptions{ListOptions: buildkite.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			return fmt.Errorf("cannot list queues in cluster %s: %w", name, err)
		}
		for _, q := range batch {
			if c.Queue == "" || q.Key == c.Queue {
				queues = append(queues, q)
			}
		}
		page = resp.NextPage
	}
	if len(queues) == 0 {
		return fmt.Errorf("no matching queue in cluster %s; check --queue or ask an administrator to configure a queue", name)
	}
	sort.Slice(queues, func(i, j int) bool { return queues[i].Key < queues[j].Key })
	selected := queues[0]
	if len(queues) > 1 {
		if !interactive {
			return fmt.Errorf("multiple queues in cluster %s; supply --queue <key>", name)
		}
		var keys []string
		for _, q := range queues {
			label := q.Key
			if q.DispatchPaused {
				label += " (paused)"
			}
			keys = append(keys, label)
		}
		key, err := bkIO.PromptForOneTo(out, "queue", keys, false)
		if err != nil {
			return err
		}
		for i, label := range keys {
			if label == key {
				selected = queues[i]
			}
		}
	}
	if selected.DispatchPaused {
		return fmt.Errorf("queue %q in cluster %s is paused (%s); resume dispatch in Buildkite or choose another --queue before creating the pipeline", selected.Key, name, selected.DispatchPausedNote)
	}
	c.Queue = selected.Key
	fmt.Fprintf(out, "Target cluster: %s (%s); queue: %s\n", name, c.ClusterUUID, c.Queue)
	connected, unpaused := 0, 0
	for page := 1; page != 0; {
		agents, resp, err := f.RestAPIClient.Agents.List(ctx, c.Org, &buildkite.AgentListOptions{ClusterQueueID: selected.ID, ListOptions: buildkite.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			fmt.Fprintf(out, "Warning: agent availability is unknown (requires read_agents): %v. Check agents for queue %s in cluster %s.\n", err, c.Queue, name)
			break
		}
		for _, a := range agents {
			if a.ConnectedState == "connected" {
				connected++
				if a.Paused == nil || !*a.Paused {
					unpaused++
				}
			}
		}
		page = resp.NextPage
		if page == 0 {
			switch {
			case connected == 0:
				fmt.Fprintf(out, "Warning: no connected agents observed for queue %s. Connect agents to this cluster/queue or check hosted/autoscaling capacity before building; this observation does not prove the queue cannot run.\n", c.Queue)
			case unpaused == 0:
				fmt.Fprintf(out, "Warning: all %d connected agents are paused; resume agents or provide capacity for queue %s before building.\n", connected, c.Queue)
			default:
				fmt.Fprintf(out, "Observed %d connected agents for queue %s; busy agents may run work later. Agent presence does not verify template runtimes.\n", connected, c.Queue)
			}
		}
	}
	return nil
}

func (c *CreateCmd) checkLocalPipeline(f *factory.Factory, root string, out io.Writer) error {
	path, err := setup.Find(root)
	if err != nil || path == "" {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := validatePipelineData(out, rel, data); err != nil {
		return err
	}
	fmt.Fprintf(out, "Bootstrap upload targets queue %s. Existing file routing is unchanged; review its agents/queue selectors. Agent presence does not verify the runtimes or dependencies required by these steps.\n", c.Queue)
	if f.GitRepository == nil {
		fmt.Fprintf(out, "Local file %s is outside Git; place it in the pipeline repository, then commit and push it before building.\n", rel)
		return nil
	}
	if c.Repository != getRepositoryURL(f, "") {
		fmt.Fprintln(out, "The destination repository differs from origin; local Git checks cannot confirm this file exists there. Put the file in the destination repository and commit/push it before building.")
	}
	wt, err := f.GitRepository.Worktree()
	if err != nil {
		return err
	}
	status, err := wt.Status()
	if err != nil {
		return err
	}
	if s, ok := status[rel]; ok && (s.Staging != ' ' || s.Worktree != ' ') {
		fmt.Fprintf(out, "Pipeline file %s has uncommitted changes. From the repository root: git add -- %s; git commit; git push. Review the file before committing.\n", rel, rel)
		return nil
	}
	head, err := f.GitRepository.Head()
	if err != nil || !head.Name().IsBranch() {
		fmt.Fprintln(out, "Cannot determine a tracking branch; ensure this pipeline file is committed and pushed to the repository before building.")
		return nil
	}
	cfg, err := f.GitRepository.Config()
	if err != nil {
		return err
	}
	branch := cfg.Branches[head.Name().Short()]
	if branch == nil || branch.Remote == "" || branch.Merge == "" {
		fmt.Fprintf(out, "No upstream configured for %s. Push this branch to the pipeline repository and set its upstream (git push -u <remote> %s).\n", head.Name().Short(), head.Name().Short())
		return nil
	}
	ref, err := f.GitRepository.Reference(plumbing.NewRemoteReferenceName(branch.Remote, branch.Merge.Short()), true)
	if err == nil {
		commit, commitErr := f.GitRepository.CommitObject(ref.Hash())
		if commitErr == nil {
			file, fileErr := commit.File(rel)
			if fileErr == nil {
				contents, contentErr := file.Contents()
				if contentErr == nil && contents == string(data) {
					fmt.Fprintln(out, "Pipeline file matches the local upstream tracking ref; run git fetch to verify current remote state. No fetch or push was performed.")
					return nil
				}
			}
		}
	}
	fmt.Fprintf(out, "Pipeline file is not confirmed in the local upstream tracking ref. Run git fetch %s, review, then git push %s %s before building.\n", branch.Remote, branch.Remote, head.Name().Short())
	return nil
}

func (c *CreateCmd) offerWebhook(out io.Writer) error {
	if c.CreateWebhook || !isGitHubURL(c.Repository) {
		return nil
	}
	fmt.Fprintln(out, "GitHub webhook setup is optional and requires permission to manage the repository webhook. It does not commit or push local files.")
	fmt.Fprintln(out, "  1. Skip webhook setup\n  2. Set up a GitHub webhook")
	choice, err := bkIO.PromptForInputTo(out, "Webhook setup (1-2)", "1", false)
	if err != nil {
		return err
	}
	if choice != "1" && choice != "2" {
		return fmt.Errorf("invalid webhook selection; enter 1 to skip or 2 to set up")
	}
	c.CreateWebhook = choice == "2"
	return nil
}
