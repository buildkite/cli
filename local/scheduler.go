package local

import (
	"errors"
	"fmt"
	"sync"
)

type build struct {
	ID     string
	Number int
	URL    string
}

type job struct {
	ID               string
	Build            build
	State            string
	ProjectSlug      string
	PipelineSlug     string
	OrganizationSlug string
	ArtifactPaths    string
	CreatorName      string
	CreatorEmail     string
	Command          string
	Label            string
	Timeout          int
	Repository       string
	Commit           string
	Branch           string
	Tag              string
	Message          string
	RetryCount       int
	PluginJSON       string
}

var errUnknownJob = errors.New("Unknown Job")

type scheduler struct {
	sync.Mutex
	jobs map[string]*job
}

func newScheduler() *scheduler {
	return &scheduler{
		jobs: map[string]*job{},
	}
}

func (s *scheduler) Schedule(job job) {
	s.Lock()
	defer s.Unlock()
	s.jobs[job.ID] = &job
}

func (s *scheduler) ChangeJobState(jobID string, from, to string) (job, error) {
	s.Lock()
	defer s.Unlock()

	j, ok := s.jobs[jobID]
	if !ok {
		return job{}, errUnknownJob
	}

	if j.State != from {
		return job{}, fmt.Errorf("Job state is %q, expected %q", j.State, from)
	}
	j.State = to
	return *j, nil
}

func (s *scheduler) GetJob(jobID string) (job, error) {
	s.Lock()
	defer s.Unlock()

	j, ok := s.jobs[jobID]
	if !ok {
		return job{}, errUnknownJob
	}

	return *j, nil
}

func (s *scheduler) NextJob() (job, bool) {
	for _, j := range s.jobs {
		if j.State == "" {
			j.State = "scheduled"
			return *j, true
		}
	}
	return job{}, false
}
