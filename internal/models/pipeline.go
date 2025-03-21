package models

// Pipeline represents a Buildkite pipeline
type Pipeline struct {
	ID                            string        `json:"id,omitempty" yaml:"id,omitempty"`
	GraphQLID                     string        `json:"graphql_id,omitempty" yaml:"graphql_id,omitempty"`
	URL                           string        `json:"url,omitempty" yaml:"url,omitempty"`
	WebURL                        string        `json:"web_url,omitempty" yaml:"web_url,omitempty"`
	Name                          string        `json:"name,omitempty" yaml:"name,omitempty"`
	Description                   string        `json:"description,omitempty" yaml:"description,omitempty"`
	Slug                          string        `json:"slug,omitempty" yaml:"slug,omitempty"`
	Repository                    string        `json:"repository,omitempty" yaml:"repository,omitempty"`
	BranchConfiguration           string        `json:"branch_configuration,omitempty" yaml:"branch_configuration,omitempty"`
	DefaultBranch                 string        `json:"default_branch,omitempty" yaml:"default_branch,omitempty"`
	Provider                      *Provider     `json:"provider,omitempty" yaml:"provider,omitempty"`
	SkipQueuedBranchBuilds        bool          `json:"skip_queued_branch_builds,omitempty" yaml:"skip_queued_branch_builds,omitempty"`
	SkipQueuedBranchBuildsFilter  string        `json:"skip_queued_branch_builds_filter,omitempty" yaml:"skip_queued_branch_builds_filter,omitempty"`
	CancelRunningBranchBuilds     bool          `json:"cancel_running_branch_builds,omitempty" yaml:"cancel_running_branch_builds,omitempty"`
	CancelRunningBranchBuildsFilter string      `json:"cancel_running_branch_builds_filter,omitempty" yaml:"cancel_running_branch_builds_filter,omitempty"`
	BuildsURL                     string        `json:"builds_url,omitempty" yaml:"builds_url,omitempty"`
	BadgeURL                      string        `json:"badge_url,omitempty" yaml:"badge_url,omitempty"`
	CreatedBy                     *User         `json:"created_by,omitempty" yaml:"created_by,omitempty"`
	CreatedAt                     string        `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	ArchivedAt                    string        `json:"archived_at,omitempty" yaml:"archived_at,omitempty"`
	ScheduledBuildsCount          int           `json:"scheduled_builds_count,omitempty" yaml:"scheduled_builds_count,omitempty"`
	RunningBuildsCount            int           `json:"running_builds_count,omitempty" yaml:"running_builds_count,omitempty"`
	ScheduledJobsCount            int           `json:"scheduled_jobs_count,omitempty" yaml:"scheduled_jobs_count,omitempty"`
	RunningJobsCount              int           `json:"running_jobs_count,omitempty" yaml:"running_jobs_count,omitempty"`
	WaitingJobsCount              int           `json:"waiting_jobs_count,omitempty" yaml:"waiting_jobs_count,omitempty"`
	Visibility                    string        `json:"visibility,omitempty" yaml:"visibility,omitempty"`
	Steps                         []interface{} `json:"steps,omitempty" yaml:"steps,omitempty"`
	Env                           map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	
	// Fields for internal use (not part of the API)
	Org                           string        `json:"-" yaml:"-"`
}

// Provider represents a Buildkite pipeline provider
type Provider struct {
	ID          string                 `json:"id,omitempty" yaml:"id,omitempty"`
	WebhookURL  string                 `json:"webhook_url,omitempty" yaml:"webhook_url,omitempty"`
	Settings    map[string]interface{} `json:"settings,omitempty" yaml:"settings,omitempty"`
}