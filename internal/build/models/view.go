package models

import (
	"time"

	"github.com/buildkite/cli/v3/internal/build/view"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// CreatorView represents build creator information
type CreatorView struct {
	ID        string    `json:"id" yaml:"id"`
	GraphQLID string    `json:"graphql_id" yaml:"graphql_id"`
	Name      string    `json:"name" yaml:"name"`
	Email     string    `json:"email" yaml:"email"`
	AvatarURL string    `json:"avatar_url" yaml:"avatar_url"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
}

// AgentView represents agent information
type AgentView struct {
	ID              string       `json:"id" yaml:"id"`
	GraphQLID       string       `json:"graphql_id" yaml:"graphql_id"`
	URL             string       `json:"url" yaml:"url"`
	WebURL          string       `json:"web_url" yaml:"web_url"`
	Name            string       `json:"name" yaml:"name"`
	ConnectionState string       `json:"connection_state" yaml:"connection_state"`
	Hostname        string       `json:"hostname" yaml:"hostname"`
	IPAddress       string       `json:"ip_address" yaml:"ip_address"`
	UserAgent       string       `json:"user_agent" yaml:"user_agent"`
	Creator         *CreatorView `json:"creator,omitempty" yaml:"creator,omitempty"`
	CreatedAt       time.Time    `json:"created_at" yaml:"created_at"`
}

// StepView represents step information
type StepView struct {
	ID        string                 `json:"id" yaml:"id"`
	Signature map[string]interface{} `json:"signature,omitempty" yaml:"signature,omitempty"`
}

// RetrySourceView represents retry source information
type RetrySourceView struct {
	JobID     string `json:"job_id" yaml:"job_id"`
	RetryType string `json:"retry_type" yaml:"retry_type"`
}

// PipelineView represents pipeline information
type PipelineView struct {
	ID                              string                 `json:"id" yaml:"id"`
	GraphQLID                       string                 `json:"graphql_id" yaml:"graphql_id"`
	URL                             string                 `json:"url" yaml:"url"`
	Name                            string                 `json:"name" yaml:"name"`
	Slug                            string                 `json:"slug" yaml:"slug"`
	Repository                      string                 `json:"repository" yaml:"repository"`
	Provider                        map[string]interface{} `json:"provider,omitempty" yaml:"provider,omitempty"`
	SkipQueuedBranchBuilds          bool                   `json:"skip_queued_branch_builds" yaml:"skip_queued_branch_builds"`
	SkipQueuedBranchBuildsFilter    *string                `json:"skip_queued_branch_builds_filter" yaml:"skip_queued_branch_builds_filter"`
	CancelRunningBranchBuilds       bool                   `json:"cancel_running_branch_builds" yaml:"cancel_running_branch_builds"`
	CancelRunningBranchBuildsFilter *string                `json:"cancel_running_branch_builds_filter" yaml:"cancel_running_branch_builds_filter"`
	BuildsURL                       string                 `json:"builds_url" yaml:"builds_url"`
	BadgeURL                        string                 `json:"badge_url" yaml:"badge_url"`
	CreatedAt                       time.Time              `json:"created_at" yaml:"created_at"`
	ScheduledBuildsCount            int                    `json:"scheduled_builds_count" yaml:"scheduled_builds_count"`
	RunningBuildsCount              int                    `json:"running_builds_count" yaml:"running_builds_count"`
	ScheduledJobsCount              int                    `json:"scheduled_jobs_count" yaml:"scheduled_jobs_count"`
	RunningJobsCount                int                    `json:"running_jobs_count" yaml:"running_jobs_count"`
	WaitingJobsCount                int                    `json:"waiting_jobs_count" yaml:"waiting_jobs_count"`
}

// RebuiltFromView represents rebuilt from information
type RebuiltFromView struct {
	ID     string `json:"id" yaml:"id"`
	Number int    `json:"number" yaml:"number"`
	URL    string `json:"url" yaml:"url"`
}

// AuthorView represents build author information
type AuthorView struct {
	Username string `json:"username" yaml:"username"`
	Name     string `json:"name" yaml:"name"`
	Email    string `json:"email" yaml:"email"`
}

// JobView represents job information
type JobView struct {
	ID                 string           `json:"id" yaml:"id"`
	GraphQLID          string           `json:"graphql_id" yaml:"graphql_id"`
	Type               string           `json:"type" yaml:"type"`
	Name               string           `json:"name,omitempty" yaml:"name,omitempty"`
	Label              string           `json:"label,omitempty" yaml:"label,omitempty"`
	StepKey            string           `json:"step_key,omitempty" yaml:"step_key,omitempty"`
	Step               *StepView        `json:"step,omitempty" yaml:"step,omitempty"`
	Command            string           `json:"command,omitempty" yaml:"command,omitempty"`
	State              string           `json:"state" yaml:"state"`
	WebURL             string           `json:"web_url" yaml:"web_url"`
	LogURL             string           `json:"log_url,omitempty" yaml:"log_url,omitempty"`
	RawLogURL          string           `json:"raw_log_url,omitempty" yaml:"raw_log_url,omitempty"`
	SoftFailed         bool             `json:"soft_failed" yaml:"soft_failed"`
	ArtifactPaths      string           `json:"artifact_paths,omitempty" yaml:"artifact_paths,omitempty"`
	Agent              *AgentView       `json:"agent,omitempty" yaml:"agent,omitempty"`
	CreatedAt          time.Time        `json:"created_at" yaml:"created_at"`
	ScheduledAt        *time.Time       `json:"scheduled_at,omitempty" yaml:"scheduled_at,omitempty"`
	RunnableAt         *time.Time       `json:"runnable_at,omitempty" yaml:"runnable_at,omitempty"`
	StartedAt          *time.Time       `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	FinishedAt         *time.Time       `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
	ExitStatus         *int             `json:"exit_status,omitempty" yaml:"exit_status,omitempty"`
	Retried            bool             `json:"retried" yaml:"retried"`
	RetriedInJobID     *string          `json:"retried_in_job_id" yaml:"retried_in_job_id"`
	RetriesCount       int              `json:"retries_count" yaml:"retries_count"`
	RetrySource        *RetrySourceView `json:"retry_source,omitempty" yaml:"retry_source,omitempty"`
	RetryType          *string          `json:"retry_type" yaml:"retry_type"`
	ParallelGroupIndex *int             `json:"parallel_group_index" yaml:"parallel_group_index"`
	ParallelGroupTotal *int             `json:"parallel_group_total" yaml:"parallel_group_total"`
	Matrix             interface{}      `json:"matrix" yaml:"matrix"`
	ClusterID          *string          `json:"cluster_id" yaml:"cluster_id"`
	ClusterURL         *string          `json:"cluster_url" yaml:"cluster_url"`
	ClusterQueueID     *string          `json:"cluster_queue_id" yaml:"cluster_queue_id"`
	ClusterQueueURL    *string          `json:"cluster_queue_url" yaml:"cluster_queue_url"`
	AgentQueryRules    []string         `json:"agent_query_rules,omitempty" yaml:"agent_query_rules,omitempty"`
}

// ArtifactView represents artifact information
type ArtifactView struct {
	ID           string `json:"id" yaml:"id"`
	JobID        string `json:"job_id" yaml:"job_id"`
	URL          string `json:"url" yaml:"url"`
	DownloadURL  string `json:"download_url" yaml:"download_url"`
	State        string `json:"state" yaml:"state"`
	Path         string `json:"path" yaml:"path"`
	Dirname      string `json:"dirname" yaml:"dirname"`
	Filename     string `json:"filename" yaml:"filename"`
	MimeType     string `json:"mime_type" yaml:"mime_type"`
	FileSize     int64  `json:"file_size" yaml:"file_size"`
	GlobPath     string `json:"glob_path" yaml:"glob_path"`
	OriginalPath string `json:"original_path" yaml:"original_path"`
	SHA1         string `json:"sha1sum" yaml:"sha1sum"`
}

// AnnotationView represents annotation information
type AnnotationView struct {
	ID       string `json:"id" yaml:"id"`
	Context  string `json:"context" yaml:"context"`
	Style    string `json:"style" yaml:"style"`
	BodyHTML string `json:"body_html" yaml:"body_html"`
}

// BuildView provides a formatted view of build data
type BuildView struct {
	ID          string                 `json:"id" yaml:"id"`
	GraphQLID   string                 `json:"graphql_id" yaml:"graphql_id"`
	URL         string                 `json:"url" yaml:"url"`
	WebURL      string                 `json:"web_url" yaml:"web_url"`
	Number      int                    `json:"number" yaml:"number"`
	State       string                 `json:"state" yaml:"state"`
	Blocked     bool                   `json:"blocked" yaml:"blocked"`
	Message     string                 `json:"message" yaml:"message"`
	Commit      string                 `json:"commit" yaml:"commit"`
	Branch      string                 `json:"branch" yaml:"branch"`
	Env         map[string]interface{} `json:"env,omitempty" yaml:"env,omitempty"`
	Source      string                 `json:"source" yaml:"source"`
	Creator     *CreatorView           `json:"creator,omitempty" yaml:"creator,omitempty"`
	Author      *AuthorView            `json:"author,omitempty" yaml:"author,omitempty"`
	Jobs        []JobView              `json:"jobs,omitempty" yaml:"jobs,omitempty"`
	CreatedAt   time.Time              `json:"created_at" yaml:"created_at"`
	ScheduledAt *time.Time             `json:"scheduled_at,omitempty" yaml:"scheduled_at,omitempty"`
	StartedAt   *time.Time             `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	FinishedAt  *time.Time             `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
	MetaData    map[string]interface{} `json:"meta_data,omitempty" yaml:"meta_data,omitempty"`
	PullRequest map[string]interface{} `json:"pull_request,omitempty" yaml:"pull_request,omitempty"`
	RebuiltFrom *RebuiltFromView       `json:"rebuilt_from,omitempty" yaml:"rebuilt_from,omitempty"`
	Pipeline    *PipelineView          `json:"pipeline,omitempty" yaml:"pipeline,omitempty"`
	Artifacts   []ArtifactView         `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
	Annotations []AnnotationView       `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// TextOutput implements the output.Formatter interface
func (b BuildView) TextOutput() string {
	// Convert back to buildkite types for rendering
	build := &buildkite.Build{
		ID:        b.ID,
		GraphQLID: b.GraphQLID,
		Number:    b.Number,
		State:     b.State,
		Blocked:   b.Blocked,
		Message:   b.Message,
		Commit:    b.Commit,
		Branch:    b.Branch,
		Source:    b.Source,
		WebURL:    b.WebURL,
		URL:       b.URL,
		CreatedAt: &buildkite.Timestamp{Time: b.CreatedAt},
	}

	if b.ScheduledAt != nil {
		build.ScheduledAt = &buildkite.Timestamp{Time: *b.ScheduledAt}
	}
	if b.StartedAt != nil {
		build.StartedAt = &buildkite.Timestamp{Time: *b.StartedAt}
	}
	if b.FinishedAt != nil {
		build.FinishedAt = &buildkite.Timestamp{Time: *b.FinishedAt}
	}

	if b.Creator != nil {
		build.Creator = buildkite.Creator{
			ID:        b.Creator.ID,
			Name:      b.Creator.Name,
			Email:     b.Creator.Email,
			AvatarURL: b.Creator.AvatarURL,
			CreatedAt: &buildkite.Timestamp{Time: b.Creator.CreatedAt},
		}
	}

	if b.Author != nil {
		build.Author = buildkite.Author{
			Username: b.Author.Username,
			Name:     b.Author.Name,
			Email:    b.Author.Email,
		}
	}

	// Convert jobs back
	for _, jobView := range b.Jobs {
		job := buildkite.Job{
			ID:              jobView.ID,
			GraphQLID:       jobView.GraphQLID,
			Type:            jobView.Type,
			Name:            jobView.Name,
			Label:           jobView.Label,
			Command:         jobView.Command,
			State:           jobView.State,
			WebURL:          jobView.WebURL,
			CreatedAt:       &buildkite.Timestamp{Time: jobView.CreatedAt},
			AgentQueryRules: jobView.AgentQueryRules,
		}

		if jobView.ScheduledAt != nil {
			job.ScheduledAt = &buildkite.Timestamp{Time: *jobView.ScheduledAt}
		}
		if jobView.StartedAt != nil {
			job.StartedAt = &buildkite.Timestamp{Time: *jobView.StartedAt}
		}
		if jobView.FinishedAt != nil {
			job.FinishedAt = &buildkite.Timestamp{Time: *jobView.FinishedAt}
		}
		if jobView.ExitStatus != nil {
			job.ExitStatus = jobView.ExitStatus
		}

		build.Jobs = append(build.Jobs, job)
	}

	// Convert artifacts back
	var artifacts []buildkite.Artifact
	for _, artifactView := range b.Artifacts {
		artifact := buildkite.Artifact{
			ID:           artifactView.ID,
			JobID:        artifactView.JobID,
			URL:          artifactView.URL,
			DownloadURL:  artifactView.DownloadURL,
			State:        artifactView.State,
			Path:         artifactView.Path,
			Dirname:      artifactView.Dirname,
			Filename:     artifactView.Filename,
			MimeType:     artifactView.MimeType,
			FileSize:     artifactView.FileSize,
			GlobPath:     artifactView.GlobPath,
			OriginalPath: artifactView.OriginalPath,
			SHA1:         artifactView.SHA1,
		}
		artifacts = append(artifacts, artifact)
	}

	// Convert annotations back
	var annotations []buildkite.Annotation
	for _, annotationView := range b.Annotations {
		annotation := buildkite.Annotation{
			ID:       annotationView.ID,
			Context:  annotationView.Context,
			Style:    annotationView.Style,
			BodyHTML: annotationView.BodyHTML,
		}
		annotations = append(annotations, annotation)
	}

	// Use the existing view logic
	buildView := view.NewBuildView(build, artifacts, annotations)
	return buildView.Render()
}

// NewBuildView creates a BuildView from buildkite types
func NewBuildView(build *buildkite.Build, artifacts []buildkite.Artifact, annotations []buildkite.Annotation) *BuildView {
	buildView := &BuildView{
		ID:        build.ID,
		GraphQLID: build.GraphQLID,
		URL:       build.URL,
		WebURL:    build.WebURL,
		Number:    build.Number,
		State:     build.State,
		Blocked:   build.Blocked,
		Message:   build.Message,
		Commit:    build.Commit,
		Branch:    build.Branch,
		Source:    build.Source,
		CreatedAt: build.CreatedAt.Time,
	}

	// Handle timestamps
	if build.ScheduledAt != nil {
		t := build.ScheduledAt.Time
		buildView.ScheduledAt = &t
	}
	if build.StartedAt != nil {
		t := build.StartedAt.Time
		buildView.StartedAt = &t
	}
	if build.FinishedAt != nil {
		t := build.FinishedAt.Time
		buildView.FinishedAt = &t
	}

	// Add environment variables
	if build.Env != nil {
		buildView.Env = build.Env
	}

	// Add metadata - convert from map[string]string to map[string]interface{}
	if build.MetaData != nil {
		metaData := make(map[string]interface{})
		for k, v := range build.MetaData {
			metaData[k] = v
		}
		buildView.MetaData = metaData
	}

	// Add pull request info
	if build.PullRequest != nil {
		buildView.PullRequest = map[string]interface{}{
			"id":         build.PullRequest.ID,
			"base":       build.PullRequest.Base,
			"repository": build.PullRequest.Repository,
		}
	}

	// Add rebuilt from info
	if build.RebuiltFrom != nil {
		buildView.RebuiltFrom = &RebuiltFromView{
			ID:     build.RebuiltFrom.ID,
			Number: build.RebuiltFrom.Number,
			URL:    build.RebuiltFrom.URL,
		}
	}

	// Add pipeline info
	if build.Pipeline != nil {
		pipelineView := &PipelineView{
			ID:                        build.Pipeline.ID,
			GraphQLID:                 build.Pipeline.GraphQLID,
			URL:                       build.Pipeline.URL,
			Name:                      build.Pipeline.Name,
			Slug:                      build.Pipeline.Slug,
			Repository:                build.Pipeline.Repository,
			SkipQueuedBranchBuilds:    build.Pipeline.SkipQueuedBranchBuilds,
			CancelRunningBranchBuilds: build.Pipeline.CancelRunningBranchBuilds,
			BuildsURL:                 build.Pipeline.BuildsURL,
			BadgeURL:                  build.Pipeline.BadgeURL,
			ScheduledBuildsCount:      build.Pipeline.ScheduledBuildsCount,
			RunningBuildsCount:        build.Pipeline.RunningBuildsCount,
			ScheduledJobsCount:        build.Pipeline.ScheduledJobsCount,
			RunningJobsCount:          build.Pipeline.RunningJobsCount,
			WaitingJobsCount:          build.Pipeline.WaitingJobsCount,
		}

		if build.Pipeline.CreatedAt != nil {
			pipelineView.CreatedAt = build.Pipeline.CreatedAt.Time
		}

		if build.Pipeline.SkipQueuedBranchBuildsFilter != "" {
			pipelineView.SkipQueuedBranchBuildsFilter = &build.Pipeline.SkipQueuedBranchBuildsFilter
		}

		if build.Pipeline.CancelRunningBranchBuildsFilter != "" {
			pipelineView.CancelRunningBranchBuildsFilter = &build.Pipeline.CancelRunningBranchBuildsFilter
		}

		// Provider is not a pointer in the buildkite library
		pipelineView.Provider = map[string]interface{}{
			"id":          build.Pipeline.Provider.ID,
			"webhook_url": build.Pipeline.Provider.WebhookURL,
		}

		buildView.Pipeline = pipelineView
	}

	if build.Creator.ID != "" {
		buildView.Creator = &CreatorView{
			ID:        build.Creator.ID,
			Name:      build.Creator.Name,
			Email:     build.Creator.Email,
			AvatarURL: build.Creator.AvatarURL,
			CreatedAt: build.Creator.CreatedAt.Time,
		}
	}

	if build.Author.Username != "" || build.Author.Name != "" || build.Author.Email != "" {
		buildView.Author = &AuthorView{
			Username: build.Author.Username,
			Name:     build.Author.Name,
			Email:    build.Author.Email,
		}
	}

	// Convert jobs
	for _, job := range build.Jobs {
		jobView := JobView{
			ID:              job.ID,
			GraphQLID:       job.GraphQLID,
			Type:            job.Type,
			Name:            job.Name,
			Label:           job.Label,
			StepKey:         job.StepKey,
			Command:         job.Command,
			State:           job.State,
			WebURL:          job.WebURL,
			LogURL:          job.LogsURL,
			RawLogURL:       job.RawLogsURL,
			SoftFailed:      job.SoftFailed,
			ArtifactPaths:   job.ArtifactPaths,
			CreatedAt:       job.CreatedAt.Time,
			Retried:         job.Retried,
			RetriesCount:    job.RetriesCount,
			Matrix:          nil, // Matrix field not available in buildkite library
			AgentQueryRules: job.AgentQueryRules,
		}

		// Step information not available in buildkite library
		jobView.Step = nil

		// Handle agent information (Agent is a value type, not pointer)
		if job.Agent.ID != "" {
			agentView := &AgentView{
				ID:              job.Agent.ID,
				GraphQLID:       job.Agent.GraphQLID,
				URL:             job.Agent.URL,
				WebURL:          job.Agent.WebURL,
				Name:            job.Agent.Name,
				ConnectionState: job.Agent.ConnectedState,
				Hostname:        job.Agent.Hostname,
				IPAddress:       job.Agent.IPAddress,
				UserAgent:       job.Agent.UserAgent,
			}

			if job.Agent.CreatedAt != nil {
				agentView.CreatedAt = job.Agent.CreatedAt.Time
			}

			if job.Agent.Creator != nil {
				agentView.Creator = &CreatorView{
					ID:        job.Agent.Creator.ID,
					Name:      job.Agent.Creator.Name,
					Email:     job.Agent.Creator.Email,
					AvatarURL: "", // AvatarURL not available in User type
				}
				if job.Agent.Creator.CreatedAt != nil {
					agentView.Creator.CreatedAt = job.Agent.Creator.CreatedAt.Time
				}
			}

			jobView.Agent = agentView
		}

		// Handle retry source
		if job.RetrySource != nil {
			jobView.RetrySource = &RetrySourceView{
				JobID:     job.RetrySource.JobID,
				RetryType: job.RetrySource.RetryType,
			}
		}

		// Handle time fields
		if job.ScheduledAt != nil {
			t := job.ScheduledAt.Time
			jobView.ScheduledAt = &t
		}
		if job.RunnableAt != nil {
			t := job.RunnableAt.Time
			jobView.RunnableAt = &t
		}
		if job.StartedAt != nil {
			t := job.StartedAt.Time
			jobView.StartedAt = &t
		}
		if job.FinishedAt != nil {
			t := job.FinishedAt.Time
			jobView.FinishedAt = &t
		}

		// Handle optional fields
		if job.ExitStatus != nil {
			jobView.ExitStatus = job.ExitStatus
		}
		if job.RetriedInJobID != "" {
			jobView.RetriedInJobID = &job.RetriedInJobID
		}
		if job.RetryType != "" {
			jobView.RetryType = &job.RetryType
		}
		if job.ParallelGroupIndex != nil {
			jobView.ParallelGroupIndex = job.ParallelGroupIndex
		}
		if job.ParallelGroupTotal != nil {
			jobView.ParallelGroupTotal = job.ParallelGroupTotal
		}
		if job.ClusterID != "" {
			jobView.ClusterID = &job.ClusterID
		}
		// ClusterURL not available in buildkite library
		if job.ClusterQueueID != "" {
			jobView.ClusterQueueID = &job.ClusterQueueID
		}
		// ClusterQueueURL not available in buildkite library

		buildView.Jobs = append(buildView.Jobs, jobView)
	}

	// Convert artifacts
	for _, artifact := range artifacts {
		artifactView := ArtifactView{
			ID:           artifact.ID,
			JobID:        artifact.JobID,
			URL:          artifact.URL,
			DownloadURL:  artifact.DownloadURL,
			State:        artifact.State,
			Path:         artifact.Path,
			Dirname:      artifact.Dirname,
			Filename:     artifact.Filename,
			MimeType:     artifact.MimeType,
			FileSize:     artifact.FileSize,
			GlobPath:     artifact.GlobPath,
			OriginalPath: artifact.OriginalPath,
			SHA1:         artifact.SHA1,
		}
		buildView.Artifacts = append(buildView.Artifacts, artifactView)
	}

	// Convert annotations
	for _, annotation := range annotations {
		annotationView := AnnotationView{
			ID:       annotation.ID,
			Context:  annotation.Context,
			Style:    annotation.Style,
			BodyHTML: annotation.BodyHTML,
		}
		buildView.Annotations = append(buildView.Annotations, annotationView)
	}

	return buildView
}
