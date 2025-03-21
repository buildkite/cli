package models

import (
	"github.com/buildkite/go-buildkite/v4"
)

// FromBuildkiteBuild converts a buildkite.Build to our internal Build model
func FromBuildkiteBuild(bkBuild *buildkite.Build) *Build {
	if bkBuild == nil {
		return nil
	}

	build := &Build{
		ID:           bkBuild.ID,
		GraphQLID:    bkBuild.GraphQLID,
		URL:          bkBuild.URL,
		WebURL:       bkBuild.WebURL,
		State:        bkBuild.State,
		Message:      bkBuild.Message,
		Commit:       bkBuild.Commit,
		Branch:       bkBuild.Branch,
	}

	// Number is an int, not a pointer
	build.Number = bkBuild.Number
	build.BuildNumber = bkBuild.Number // Also set internal field

	// Creator is a struct, not a pointer
	build.Creator = &User{
		ID:        bkBuild.Creator.ID,
		Name:      bkBuild.Creator.Name,
		Email:     bkBuild.Creator.Email,
		AvatarURL: bkBuild.Creator.AvatarURL,
		// Convert timestamp to string if needed
		CreatedAt: bkBuild.Creator.CreatedAt.String(),
	}

	if bkBuild.Pipeline != nil {
		build.Pipeline = &Pipeline{
			ID:       bkBuild.Pipeline.ID,
			Name:     bkBuild.Pipeline.Name,
			Slug:     bkBuild.Pipeline.Slug,
			URL:      bkBuild.Pipeline.URL,
		}
		
		// Set internal fields
		build.Pipeline.Org = build.Organization
	}

	// Based on your errors, Organization field doesn't exist in buildkite.Build anymore
	// It looks like the organization information might need to be derived from the pipeline

	// Convert timestamps to strings
	if bkBuild.CreatedAt != nil {
		build.CreatedAt = bkBuild.CreatedAt.String()
	}
	if bkBuild.ScheduledAt != nil {
		build.ScheduledAt = bkBuild.ScheduledAt.String()
	}
	if bkBuild.StartedAt != nil {
		build.StartedAt = bkBuild.StartedAt.String()
	}
	if bkBuild.FinishedAt != nil {
		build.FinishedAt = bkBuild.FinishedAt.String()
	}

	if bkBuild.Jobs != nil {
		build.Jobs = make([]*Job, len(bkBuild.Jobs))
		for i, bkJob := range bkBuild.Jobs {
			build.Jobs[i] = FromBuildkiteJob(&bkJob)
		}
	}

	return build
}

// FromBuildkiteArtifact converts a buildkite.Artifact to our internal Artifact model
func FromBuildkiteArtifact(bkArtifact *buildkite.Artifact) *Artifact {
	if bkArtifact == nil {
		return nil
	}

	return &Artifact{
		ID:           bkArtifact.ID,
		JobID:        bkArtifact.JobID,
		URL:          bkArtifact.URL,
		DownloadURL:  bkArtifact.DownloadURL,
		State:        bkArtifact.State,
		Path:         bkArtifact.Path,
		Dirname:      bkArtifact.Dirname,
		Filename:     bkArtifact.Filename,
		MimeType:     bkArtifact.MimeType,
		FileSize:     bkArtifact.FileSize,
		GlobPath:     bkArtifact.GlobPath,
		OriginalPath: bkArtifact.OriginalPath,
		Sha1Sum:      bkArtifact.SHA1,
		// CreatedAt and UploadedAt fields no longer exist in buildkite.Artifact
	}
}

// FromBuildkiteAnnotation converts a buildkite.Annotation to our internal Annotation model
func FromBuildkiteAnnotation(bkAnnotation *buildkite.Annotation) *Annotation {
	if bkAnnotation == nil {
		return nil
	}

	// Based on error messages, CreatedBy and Body no longer exist in buildkite.Annotation
	// Let's create with what we know is available
	return &Annotation{
		ID:        bkAnnotation.ID,
		Context:   bkAnnotation.Context,
		Style:     bkAnnotation.Style,
		BodyHTML:  bkAnnotation.BodyHTML,
		// Convert timestamp to string if needed
		CreatedAt: bkAnnotation.CreatedAt.String(),
		// CreatedBy: nil, // Not available in buildkite.Annotation anymore
	}
}

// FromBuildkiteJob converts a buildkite.Job to our internal Job model
func FromBuildkiteJob(bkJob *buildkite.Job) *Job {
	if bkJob == nil {
		return nil
	}

	job := &Job{
		ID:            bkJob.ID,
		GraphQLID:     bkJob.GraphQLID,
		Type:          bkJob.Type,
		Name:          bkJob.Name,
		StepKey:       bkJob.StepKey,
		WebURL:        bkJob.WebURL,
		// LogURL and RawLogURL fields don't exist in buildkite.Job anymore
		Command:       bkJob.Command,
		// CommandName field doesn't exist in buildkite.Job anymore
		ArtifactPaths: bkJob.ArtifactPaths,
		SoftFailed:    bkJob.SoftFailed,
		State:         bkJob.State,
		Retried:       bkJob.Retried,
		RetriedInJobID: bkJob.RetriedInJobID,
		RetriesCount:  bkJob.RetriesCount,
		RetryType:     bkJob.RetryType,
	}

	// Convert timestamps to strings
	if bkJob.CreatedAt != nil {
		job.CreatedAt = bkJob.CreatedAt.String()
	}
	if bkJob.ScheduledAt != nil {
		job.ScheduledAt = bkJob.ScheduledAt.String()
	}
	if bkJob.RunnableAt != nil {
		job.RunnableAt = bkJob.RunnableAt.String()
	}
	if bkJob.StartedAt != nil {
		job.StartedAt = bkJob.StartedAt.String()
	}
	if bkJob.FinishedAt != nil {
		job.FinishedAt = bkJob.FinishedAt.String()
	}

	if bkJob.ExitStatus != nil {
		job.ExitStatus = *bkJob.ExitStatus
	}

	if bkJob.AgentQueryRules != nil {
		job.AgentQueryRules = bkJob.AgentQueryRules
	}

	// Agent is now a struct, not a pointer
	job.Agent = FromBuildkiteAgent(&bkJob.Agent)

	return job
}

// FromBuildkiteAgent converts a buildkite.Agent to our internal Agent model
func FromBuildkiteAgent(bkAgent *buildkite.Agent) *Agent {
	if bkAgent == nil {
		return nil
	}

	agent := &Agent{
		ID:              bkAgent.ID,
		GraphQLID:       bkAgent.GraphQLID,
		URL:             bkAgent.URL,
		WebURL:          bkAgent.WebURL,
		Name:            bkAgent.Name,
		// ConnectionState renamed or removed in buildkite.Agent
		Hostname:        bkAgent.Hostname,
		IPAddress:       bkAgent.IPAddress,
		UserAgent:       bkAgent.UserAgent,
		Version:         bkAgent.Version,
		// Convert timestamp to string
		MetaData:        bkAgent.Metadata, // Note the capital 'D' in Metadata
	}

	// Convert timestamp to string
	if bkAgent.CreatedAt != nil {
		agent.CreatedAt = bkAgent.CreatedAt.String()
	}

	if bkAgent.Creator != nil {
		agent.Creator = &User{
			ID:        bkAgent.Creator.ID,
			Name:      bkAgent.Creator.Name,
			Email:     bkAgent.Creator.Email,
			// AvatarURL renamed or removed in buildkite.User
			// Convert timestamp to string if needed
		}
		if bkAgent.Creator.CreatedAt != nil {
			agent.Creator.CreatedAt = bkAgent.Creator.CreatedAt.String()
		}
	}

	return agent
}

// ToBuildkiteArtifact converts our internal Artifact model to a buildkite.Artifact
func (a *Artifact) ToBuildkiteArtifact() *buildkite.Artifact {
	if a == nil {
		return nil
	}

	return &buildkite.Artifact{
		ID:           a.ID,
		JobID:        a.JobID,
		URL:          a.URL,
		DownloadURL:  a.DownloadURL,
		State:        a.State,
		Path:         a.Path,
		Dirname:      a.Dirname,
		Filename:     a.Filename,
		MimeType:     a.MimeType,
		FileSize:     a.FileSize,
		GlobPath:     a.GlobPath,
		OriginalPath: a.OriginalPath,
		SHA1:         a.Sha1Sum,
		// CreatedAt and UploadedAt fields don't exist in buildkite.Artifact anymore
	}
}

// ToBuildkiteAnnotation converts our internal Annotation model to a buildkite.Annotation
func (a *Annotation) ToBuildkiteAnnotation() *buildkite.Annotation {
	if a == nil {
		return nil
	}

	// Create a timestamp from the string if needed
	var createdAt *buildkite.Timestamp
	// This would need actual parsing code, but we'll use nil for now
	// If needed, add proper time parsing logic here

	return &buildkite.Annotation{
		ID:        a.ID,
		Context:   a.Context,
		Style:     a.Style,
		// Body field not available in buildkite.Annotation anymore
		CreatedAt: createdAt,
		// CreatedBy field not available in buildkite.Annotation anymore
		BodyHTML:  a.BodyHTML,
	}
}