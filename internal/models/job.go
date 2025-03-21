package models

// Job represents a Buildkite job within a build
type Job struct {
	ID                 string     `json:"id,omitempty" yaml:"id,omitempty"`
	GraphQLID          string     `json:"graphql_id,omitempty" yaml:"graphql_id,omitempty"`
	Type               string     `json:"type,omitempty" yaml:"type,omitempty"`
	Name               string     `json:"name,omitempty" yaml:"name,omitempty"`
	StepKey            string     `json:"step_key,omitempty" yaml:"step_key,omitempty"`
	Step               *Step      `json:"step,omitempty" yaml:"step,omitempty"`
	AgentQueryRules    []string   `json:"agent_query_rules,omitempty" yaml:"agent_query_rules,omitempty"`
	State              string     `json:"state,omitempty" yaml:"state,omitempty"`
	BuildURL           string     `json:"build_url,omitempty" yaml:"build_url,omitempty"`
	WebURL             string     `json:"web_url,omitempty" yaml:"web_url,omitempty"`
	LogURL             string     `json:"log_url,omitempty" yaml:"log_url,omitempty"`
	RawLogURL          string     `json:"raw_log_url,omitempty" yaml:"raw_log_url,omitempty"`
	ArtifactsURL       string     `json:"artifacts_url,omitempty" yaml:"artifacts_url,omitempty"`
	Command            string     `json:"command,omitempty" yaml:"command,omitempty"`
	SoftFailed         bool       `json:"soft_failed,omitempty" yaml:"soft_failed,omitempty"`
	ExitStatus         int        `json:"exit_status,omitempty" yaml:"exit_status,omitempty"`
	ArtifactPaths      string     `json:"artifact_paths,omitempty" yaml:"artifact_paths,omitempty"`
	Agent              *Agent     `json:"agent,omitempty" yaml:"agent,omitempty"`
	CreatedAt          string     `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	ScheduledAt        string     `json:"scheduled_at,omitempty" yaml:"scheduled_at,omitempty"`
	RunnableAt         string     `json:"runnable_at,omitempty" yaml:"runnable_at,omitempty"`
	StartedAt          string     `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	FinishedAt         string     `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
	Retried            bool       `json:"retried,omitempty" yaml:"retried,omitempty"`
	RetriedInJobID     string     `json:"retried_in_job_id,omitempty" yaml:"retried_in_job_id,omitempty"`
	RetriesCount       int        `json:"retries_count,omitempty" yaml:"retries_count,omitempty"`
	RetrySource        *RetrySource `json:"retry_source,omitempty" yaml:"retry_source,omitempty"`
	RetryType          string     `json:"retry_type,omitempty" yaml:"retry_type,omitempty"`
	ParallelGroupIndex interface{} `json:"parallel_group_index,omitempty" yaml:"parallel_group_index,omitempty"`
	ParallelGroupTotal interface{} `json:"parallel_group_total,omitempty" yaml:"parallel_group_total,omitempty"`
	Matrix             interface{} `json:"matrix,omitempty" yaml:"matrix,omitempty"`
	ClusterID          string     `json:"cluster_id,omitempty" yaml:"cluster_id,omitempty"`
	ClusterURL         string     `json:"cluster_url,omitempty" yaml:"cluster_url,omitempty"`
	ClusterQueueID     string     `json:"cluster_queue_id,omitempty" yaml:"cluster_queue_id,omitempty"`
	ClusterQueueURL    string     `json:"cluster_queue_url,omitempty" yaml:"cluster_queue_url,omitempty"`
	
	// For internal CLI use
	CommandName        string     `json:"-" yaml:"-"`
}

// Step represents job step information
type Step struct {
	ID        string    `json:"id,omitempty" yaml:"id,omitempty"`
	Signature *Signature `json:"signature,omitempty" yaml:"signature,omitempty"`
}

// Signature contains step signature info
type Signature struct {
	Value        string   `json:"value,omitempty" yaml:"value,omitempty"`
	Algorithm    string   `json:"algorithm,omitempty" yaml:"algorithm,omitempty"`
	SignedFields []string `json:"signed_fields,omitempty" yaml:"signed_fields,omitempty"`
}

// RetrySource contains information about retry source
type RetrySource struct {
	JobID     string `json:"job_id,omitempty" yaml:"job_id,omitempty"` 
	RetryType string `json:"retry_type,omitempty" yaml:"retry_type,omitempty"`
}