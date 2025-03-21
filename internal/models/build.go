package models

import "time"

// Build represents a Buildkite build
type Build struct {
	// API fields
	ID           string     `json:"id,omitempty" yaml:"id,omitempty"`
	GraphQLID    string     `json:"graphql_id,omitempty" yaml:"graphql_id,omitempty"`
	URL          string     `json:"url,omitempty" yaml:"url,omitempty"`
	WebURL       string     `json:"web_url,omitempty" yaml:"web_url,omitempty"`
	Number       int        `json:"number,omitempty" yaml:"number,omitempty"`
	State        string     `json:"state,omitempty" yaml:"state,omitempty"`
	CancelReason string     `json:"cancel_reason,omitempty" yaml:"cancel_reason,omitempty"`
	Blocked      bool       `json:"blocked,omitempty" yaml:"blocked,omitempty"`
	Message      string     `json:"message,omitempty" yaml:"message,omitempty"`
	Commit       string     `json:"commit,omitempty" yaml:"commit,omitempty"`
	Branch       string     `json:"branch,omitempty" yaml:"branch,omitempty"`
	Env          map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Source       string     `json:"source,omitempty" yaml:"source,omitempty"`
	Creator      *User      `json:"creator,omitempty" yaml:"creator,omitempty"`
	Jobs         []*Job     `json:"jobs,omitempty" yaml:"jobs,omitempty"`
	CreatedAt    string     `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	ScheduledAt  string     `json:"scheduled_at,omitempty" yaml:"scheduled_at,omitempty"`
	StartedAt    string     `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	FinishedAt   string     `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
	MetaData     map[string]string `json:"meta_data,omitempty" yaml:"meta_data,omitempty"`
	PullRequest  map[string]interface{} `json:"pull_request,omitempty" yaml:"pull_request,omitempty"`
	RebuiltFrom  *RebuiltFrom `json:"rebuilt_from,omitempty" yaml:"rebuilt_from,omitempty"`
	Pipeline     *Pipeline `json:"pipeline,omitempty" yaml:"pipeline,omitempty"`
	
	// Fields for CLI resolution (not part of the API)
	Organization string `json:"-" yaml:"-"`
	BuildNumber  int    `json:"-" yaml:"-"`
}

// RebuiltFrom represents information about a build this build was rebuilt from
type RebuiltFrom struct {
	ID     string `json:"id,omitempty" yaml:"id,omitempty"`
	Number int    `json:"number,omitempty" yaml:"number,omitempty"`
	URL    string `json:"url,omitempty" yaml:"url,omitempty"`
}