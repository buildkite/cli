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

	if bkBuild.Number != nil {
		build.Number = *bkBuild.Number
		build.BuildNumber = *bkBuild.Number // Also set internal field
	}

	if bkBuild.Creator != nil {
		build.Creator = &User{
			ID:        bkBuild.Creator.ID,
			Name:      bkBuild.Creator.Name,
			Email:     bkBuild.Creator.Email,
			AvatarURL: bkBuild.Creator.AvatarURL,
			CreatedAt: bkBuild.Creator.CreatedAt,
		}
	}

	if bkBuild.Pipeline != nil {
		// Extract organization slug from pipeline
		if bkBuild.Pipeline.Slug != nil {
			build.Pipeline = &Pipeline{
				ID:       bkBuild.Pipeline.ID,
				Name:     bkBuild.Pipeline.Name,
				Slug:     *bkBuild.Pipeline.Slug,
				URL:      bkBuild.Pipeline.URL,
			}
			
			// Set internal fields
			build.Pipeline.Org = build.Organization
		}
	}

	if bkBuild.Organization != nil && bkBuild.Organization.Slug != nil {
		build.Organization = *bkBuild.Organization.Slug
	}

	build.CreatedAt = bkBuild.CreatedAt
	build.ScheduledAt = bkBuild.ScheduledAt
	build.StartedAt = bkBuild.StartedAt
	build.FinishedAt = bkBuild.FinishedAt

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
		ID:          bkArtifact.ID,
		JobID:       bkArtifact.JobID,
		URL:         bkArtifact.URL,
		DownloadURL: bkArtifact.DownloadURL,
		State:       bkArtifact.State,
		Path:        bkArtifact.Path,
		Dirname:     bkArtifact.Dirname,
		Filename:    bkArtifact.Filename,
		MimeType:    bkArtifact.MimeType,
		FileSize:    bkArtifact.FileSize,
		Sha1Sum:     bkArtifact.Sha1Sum,
		CreatedAt:   bkArtifact.CreatedAt,
		UploadedAt:  bkArtifact.UploadedAt,
	}
}

// FromBuildkiteAnnotation converts a buildkite.Annotation to our internal Annotation model
func FromBuildkiteAnnotation(bkAnnotation *buildkite.Annotation) *Annotation {
	if bkAnnotation == nil {
		return nil
	}

	var user *User
	if bkAnnotation.CreatedBy != nil {
		user = &User{
			ID:        bkAnnotation.CreatedBy.ID,
			Name:      bkAnnotation.CreatedBy.Name,
			Email:     bkAnnotation.CreatedBy.Email,
			AvatarURL: bkAnnotation.CreatedBy.AvatarURL,
			CreatedAt: bkAnnotation.CreatedBy.CreatedAt,
		}
	}

	return &Annotation{
		ID:        bkAnnotation.ID,
		Context:   bkAnnotation.Context,
		Style:     bkAnnotation.Style,
		Body:      bkAnnotation.Body,
		BodyHTML:  bkAnnotation.BodyHTML,
		CreatedAt: bkAnnotation.CreatedAt,
		CreatedBy: user,
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
		LogURL:        bkJob.LogURL,
		RawLogURL:     bkJob.RawLogURL,
		Command:       bkJob.Command,
		CommandName:   bkJob.CommandName,
		ArtifactPaths: bkJob.ArtifactPaths,
		SoftFailed:    bkJob.SoftFailed,
		State:         bkJob.State,
		CreatedAt:     bkJob.CreatedAt,
		ScheduledAt:   bkJob.ScheduledAt,
		RunnableAt:    bkJob.RunnableAt,
		StartedAt:     bkJob.StartedAt,
		FinishedAt:    bkJob.FinishedAt,
		Retried:       bkJob.Retried,
		RetriedInJobID: bkJob.RetriedInJobID,
		RetriesCount:  bkJob.RetriesCount,
		RetryType:     bkJob.RetryType,
	}

	if bkJob.ExitStatus != nil {
		job.ExitStatus = *bkJob.ExitStatus
	}

	if bkJob.AgentQueryRules != nil {
		job.AgentQueryRules = bkJob.AgentQueryRules
	}

	if bkJob.Agent != nil {
		job.Agent = FromBuildkiteAgent(bkJob.Agent)
	}

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
		ConnectionState: bkAgent.ConnectionState,
		Hostname:        bkAgent.Hostname,
		IPAddress:       bkAgent.IPAddress,
		UserAgent:       bkAgent.UserAgent,
		Version:         bkAgent.Version,
		CreatedAt:       bkAgent.CreatedAt,
		MetaData:        bkAgent.MetaData,
	}

	if bkAgent.Creator != nil {
		agent.Creator = &User{
			ID:        bkAgent.Creator.ID,
			Name:      bkAgent.Creator.Name,
			Email:     bkAgent.Creator.Email,
			AvatarURL: bkAgent.Creator.AvatarURL,
			CreatedAt: bkAgent.Creator.CreatedAt,
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
		ID:          a.ID,
		JobID:       a.JobID,
		URL:         a.URL,
		DownloadURL: a.DownloadURL,
		State:       a.State,
		Path:        a.Path,
		Dirname:     a.Dirname,
		Filename:    a.Filename,
		MimeType:    a.MimeType,
		FileSize:    a.FileSize,
		Sha1Sum:     a.Sha1Sum,
		CreatedAt:   a.CreatedAt,
		UploadedAt:  a.UploadedAt,
	}
}

// ToBuildkiteAnnotation converts our internal Annotation model to a buildkite.Annotation
func (a *Annotation) ToBuildkiteAnnotation() *buildkite.Annotation {
	if a == nil {
		return nil
	}

	var bkUser *buildkite.User
	if a.CreatedBy != nil {
		bkUser = &buildkite.User{
			ID:        a.CreatedBy.ID,
			Name:      a.CreatedBy.Name,
			Email:     a.CreatedBy.Email,
			AvatarURL: a.CreatedBy.AvatarURL,
			CreatedAt: a.CreatedBy.CreatedAt,
		}
	}

	return &buildkite.Annotation{
		ID:        a.ID,
		Context:   a.Context,
		Style:     a.Style,
		Body:      a.Body,
		CreatedAt: a.CreatedAt,
		CreatedBy: bkUser,
		BodyHTML:  a.BodyHTML,
	}
}