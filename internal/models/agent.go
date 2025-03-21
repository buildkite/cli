package models

// Agent represents a Buildkite agent
type Agent struct {
	ID               string    `json:"id,omitempty" yaml:"id,omitempty"`
	GraphQLID        string    `json:"graphql_id,omitempty" yaml:"graphql_id,omitempty"`
	URL              string    `json:"url,omitempty" yaml:"url,omitempty"`
	WebURL           string    `json:"web_url,omitempty" yaml:"web_url,omitempty"`
	Name             string    `json:"name,omitempty" yaml:"name,omitempty"`
	ConnectionState  string    `json:"connection_state,omitempty" yaml:"connection_state,omitempty"`
	Hostname         string    `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	IPAddress        string    `json:"ip_address,omitempty" yaml:"ip_address,omitempty"`
	UserAgent        string    `json:"user_agent,omitempty" yaml:"user_agent,omitempty"`
	Version          string    `json:"version,omitempty" yaml:"version,omitempty"`
	Creator          *User     `json:"creator,omitempty" yaml:"creator,omitempty"`
	CreatedAt        string    `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	Job              *Job      `json:"job,omitempty" yaml:"job,omitempty"`
	LastJobFinishedAt string    `json:"last_job_finished_at,omitempty" yaml:"last_job_finished_at,omitempty"`
	Priority         string    `json:"priority,omitempty" yaml:"priority,omitempty"`
	MetaData         []string  `json:"meta_data,omitempty" yaml:"meta_data,omitempty"`
	
	// Fields for internal use
	ClusterID        string    `json:"-" yaml:"-"`
	Organization     *Organization `json:"-" yaml:"-"`
}