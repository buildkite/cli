package build

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/buildkite/cli/v3/internal/build/view"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type recursiveFailure struct {
	Build       string   `json:"build" yaml:"build"`
	ID          string   `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	State       string   `json:"state" yaml:"state"`
	WebURL      string   `json:"web_url" yaml:"web_url"`
	LogURL      string   `json:"log_url" yaml:"log_url"`
	RawLogURL   string   `json:"raw_log_url" yaml:"raw_log_url"`
	TriggerPath []string `json:"trigger_path" yaml:"trigger_path"`
}

type traversalIssue struct {
	Build   string `json:"build" yaml:"build"`
	JobID   string `json:"job_id,omitempty" yaml:"job_id,omitempty"`
	Message string `json:"message" yaml:"message"`
}

type recursiveFailures struct {
	RootBuild string             `json:"root_build" yaml:"root_build"`
	Complete  bool               `json:"complete" yaml:"complete"`
	Failures  []recursiveFailure `json:"failures" yaml:"failures"`
	Issues    []traversalIssue   `json:"issues" yaml:"issues"`
}

func (r recursiveFailures) TextOutput() string {
	var text strings.Builder
	fmt.Fprintf(&text, "Recursive failures from %s (complete: %t)\n", r.RootBuild, r.Complete)
	fmt.Fprintf(&text, "Command failures: %d\n", len(r.Failures))
	for _, failure := range r.Failures {
		fmt.Fprintf(&text, "\n%s #%s (%s) %s\n", failure.Build, failure.ID, failure.State, failure.Name)
		if len(failure.TriggerPath) > 0 {
			fmt.Fprintf(&text, "  Trigger path: %s\n", strings.Join(failure.TriggerPath, " -> "))
		}
		fmt.Fprintf(&text, "  Job: %s\n  Log: %s\n  Raw log: %s\n", failure.WebURL, failure.LogURL, failure.RawLogURL)
	}
	for _, issue := range r.Issues {
		fmt.Fprintf(&text, "\nIncomplete: %s", issue.Build)
		if issue.JobID != "" {
			fmt.Fprintf(&text, " #%s", issue.JobID)
		}
		fmt.Fprintf(&text, ": %s\n", issue.Message)
	}
	return text.String()
}

func buildSlug(opts view.ViewOptions) string {
	return fmt.Sprintf("%s/%s/%d", opts.Organization, opts.Pipeline, opts.BuildNumber)
}

func fetchRecursiveFailures(ctx context.Context, client *buildkite.Client, root view.ViewOptions) recursiveFailures {
	result := recursiveFailures{
		RootBuild: buildSlug(root),
		Complete:  true,
		Failures:  []recursiveFailure{},
		Issues:    []traversalIssue{},
	}
	visited := map[string]bool{}
	var visit func(view.ViewOptions, []string)
	visit = func(opts view.ViewOptions, path []string) {
		slug := buildSlug(opts)
		if visited[slug] {
			return
		}
		visited[slug] = true
		// Get returns all embedded jobs when JobStates is unset. By default it
		// excludes superseded attempts, unlike the paginated Jobs.ListByBuild.
		b, _, err := client.Builds.Get(ctx, opts.Organization, opts.Pipeline, strconv.Itoa(opts.BuildNumber), &buildkite.BuildGetOptions{
			BuildsListOptions: buildkite.BuildsListOptions{ExcludePipeline: true},
		})
		if err != nil {
			result.Issues = append(result.Issues, traversalIssue{Build: slug, Message: err.Error()})
			return
		}
		for _, job := range b.Jobs {
			if job.Retried {
				continue
			}
			if job.Type == "script" && !job.SoftFailed && (job.State == "failed" || job.State == "timed_out") {
				result.Failures = append(result.Failures, recursiveFailure{
					Build: slug, ID: job.ID, Name: job.Name, State: job.State,
					WebURL: job.WebURL, LogURL: job.LogURL, RawLogURL: job.RawLogsURL, TriggerPath: path,
				})
			}
			if job.Type != "trigger" {
				continue
			}
			if job.TriggeredBuild == nil {
				// Skipped/configuration-excluded triggers have no descendants to
				// visit. Other null references may be failed or not yet created.
				if job.State != "skipped" && job.State != "broken" {
					result.Issues = append(result.Issues, traversalIssue{Build: slug, JobID: job.ID, Message: "trigger has no child build reference (child missing or not yet created)"})
				}
				continue
			}
			child, err := triggeredBuildOptions(job.TriggeredBuild.URL)
			if err != nil {
				result.Issues = append(result.Issues, traversalIssue{Build: slug, JobID: job.ID, Message: err.Error()})
				continue
			}
			childPath := append(append([]string{}, path...), slug+"#"+job.ID)
			visit(child, childPath)
		}
	}
	visit(root, []string{})
	result.Complete = len(result.Issues) == 0
	return result
}

func triggeredBuildOptions(rawURL string) (view.ViewOptions, error) {
	// Extract the canonical REST route, but always request through the SDK's
	// configured endpoint rather than sending credentials to a returned host.
	u, err := url.Parse(rawURL)
	if err == nil {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 7 && parts[0] == "v2" && parts[1] == "organizations" && parts[3] == "pipelines" && parts[5] == "builds" {
			number, err := strconv.Atoi(parts[6])
			opts := view.ViewOptions{Organization: parts[2], Pipeline: parts[4], BuildNumber: number}
			if err == nil && number > 0 && opts.Validate() == nil {
				return opts, nil
			}
		}
	}
	return view.ViewOptions{}, fmt.Errorf("trigger has an invalid child build API URL")
}
