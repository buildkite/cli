package pipelinerun

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SchedulerEvent represents an event in the scheduler
type SchedulerEvent interface {
	eventType() string
}

// JobStartedEvent is emitted when a job starts running
type JobStartedEvent struct {
	JobID string
}

func (JobStartedEvent) eventType() string { return "job_started" }

// JobFinishedEvent is emitted when a job completes
type JobFinishedEvent struct {
	JobID    string
	State    JobState
	ExitCode int
}

func (JobFinishedEvent) eventType() string { return "job_finished" }

// PipelineUploadEvent is emitted when a job uploads a dynamic pipeline
type PipelineUploadEvent struct {
	JobID    string
	Pipeline *Pipeline
}

func (PipelineUploadEvent) eventType() string { return "pipeline_upload" }

// Scheduler manages job execution and state
type Scheduler struct {
	graph      *JobGraph
	maxWorkers int

	// Channels
	jobQueue chan *Job
	events   chan SchedulerEvent
	done     chan struct{}

	// Concurrency tracking
	mu                sync.Mutex
	runningJobs       map[string]bool
	concurrencyGroups map[string]int // group -> current count

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// NewScheduler creates a new scheduler for the given job graph
func NewScheduler(graph *JobGraph, maxWorkers int) *Scheduler {
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		graph:             graph,
		maxWorkers:        maxWorkers,
		jobQueue:          make(chan *Job, maxWorkers*2),
		events:            make(chan SchedulerEvent, 100),
		done:              make(chan struct{}),
		runningJobs:       make(map[string]bool),
		concurrencyGroups: make(map[string]int),
		ctx:               ctx,
		cancel:            cancel,
	}
}

// JobQueue returns the channel of jobs ready for execution
func (s *Scheduler) JobQueue() <-chan *Job {
	return s.jobQueue
}

// Events returns the channel of scheduler events
func (s *Scheduler) Events() <-chan SchedulerEvent {
	return s.events
}

// Done returns a channel that's closed when the scheduler is complete
func (s *Scheduler) Done() <-chan struct{} {
	return s.done
}

// Start begins the scheduler's event loop
func (s *Scheduler) Start() {
	go s.scheduleLoop()
}

// Stop cancels the scheduler
func (s *Scheduler) Stop() {
	s.cancel()
}

// HandleJobStarted processes a job started event
func (s *Scheduler) HandleJobStarted(jobID string) {
	s.mu.Lock()
	s.runningJobs[jobID] = true
	s.mu.Unlock()

	s.graph.UpdateJobState(jobID, JobStateRunning)

	select {
	case s.events <- JobStartedEvent{JobID: jobID}:
	default:
	}
}

// HandleJobFinished processes a job completion
func (s *Scheduler) HandleJobFinished(jobID string, state JobState, exitCode int) {
	s.mu.Lock()
	delete(s.runningJobs, jobID)

	// Update concurrency group
	if job, ok := s.graph.GetJob(jobID); ok && job.ConcurrencyGroup != "" {
		s.concurrencyGroups[job.ConcurrencyGroup]--
		if s.concurrencyGroups[job.ConcurrencyGroup] < 0 {
			s.concurrencyGroups[job.ConcurrencyGroup] = 0
		}
	}
	s.mu.Unlock()

	// Update job state
	if job, ok := s.graph.GetJob(jobID); ok {
		job.ExitCode = exitCode
	}
	s.graph.UpdateJobState(jobID, state)

	select {
	case s.events <- JobFinishedEvent{JobID: jobID, State: state, ExitCode: exitCode}:
	default:
	}
}

// HandlePipelineUpload processes a dynamic pipeline upload
func (s *Scheduler) HandlePipelineUpload(jobID string, pipeline *Pipeline) error {
	// Plan the new pipeline
	planner := NewPlanner(pipeline)
	graph, err := planner.Plan()
	if err != nil {
		return fmt.Errorf("planning uploaded pipeline: %w", err)
	}

	// Insert new jobs after the uploading job
	newJobs := graph.GetOrderedJobs()
	if err := s.graph.InsertJobsAfter(jobID, newJobs); err != nil {
		return fmt.Errorf("inserting jobs: %w", err)
	}

	select {
	case s.events <- PipelineUploadEvent{JobID: jobID, Pipeline: pipeline}:
	default:
	}

	return nil
}

// scheduleLoop is the main scheduling loop
func (s *Scheduler) scheduleLoop() {
	defer close(s.done)
	defer close(s.jobQueue)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if s.graph.AllJobsTerminal() {
				return
			}
			s.scheduleReadyJobs()
		}
	}
}

// scheduleReadyJobs finds and queues jobs that are ready to run
func (s *Scheduler) scheduleReadyJobs() {
	s.mu.Lock()
	defer s.mu.Unlock()

	runningCount := len(s.runningJobs)
	if runningCount >= s.maxWorkers {
		return
	}

	readyJobs := s.graph.GetReadyJobs()

	for _, job := range readyJobs {
		if runningCount >= s.maxWorkers {
			break
		}

		// Skip already running jobs
		if s.runningJobs[job.ID] {
			continue
		}

		// Check concurrency group limits
		if job.ConcurrencyGroup != "" && job.ConcurrencyLimit > 0 {
			current := s.concurrencyGroups[job.ConcurrencyGroup]
			if current >= job.ConcurrencyLimit {
				continue
			}
			s.concurrencyGroups[job.ConcurrencyGroup]++
		}

		// Handle wait steps immediately (they don't need agent execution)
		if job.Type == StepTypeWait {
			s.graph.UpdateJobState(job.ID, JobStatePassed)
			continue
		}

		// Handle block/input steps (auto-pass for local runs, or could prompt)
		if job.Type == StepTypeBlock || job.Type == StepTypeInput {
			// For local runs, we auto-unblock unless configured otherwise
			s.graph.UpdateJobState(job.ID, JobStatePassed)
			continue
		}

		// Handle trigger steps (skip for local runs)
		if job.Type == StepTypeTrigger {
			// Can't trigger remote pipelines locally
			s.graph.UpdateJobState(job.ID, JobStateSkipped)
			continue
		}

		// Mark as waiting and queue for execution
		s.graph.UpdateJobState(job.ID, JobStateWaiting)
		s.runningJobs[job.ID] = true
		runningCount++

		select {
		case s.jobQueue <- job:
		default:
			// Queue full, will retry next tick
			delete(s.runningJobs, job.ID)
			s.graph.UpdateJobState(job.ID, JobStatePending)
			runningCount--
		}
	}
}

// MarkDependentsFailed marks all jobs depending on a failed job as broken
func (s *Scheduler) MarkDependentsFailed(failedJobID string) {
	visited := make(map[string]bool)
	s.markDependentsFailedRecursive(failedJobID, visited)
}

func (s *Scheduler) markDependentsFailedRecursive(jobID string, visited map[string]bool) {
	if visited[jobID] {
		return
	}
	visited[jobID] = true

	dependents := s.graph.GetDependents(jobID)
	for _, depID := range dependents {
		job, ok := s.graph.GetJob(depID)
		if !ok {
			continue
		}

		// Only mark as broken if not allowing dependency failures
		if !job.AllowDepFail {
			s.graph.UpdateJobState(depID, JobStateBroken)
			s.markDependentsFailedRecursive(depID, visited)
		}
	}
}

// GetStats returns current scheduler statistics
func (s *Scheduler) GetStats() SchedulerStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := SchedulerStats{
		TotalJobs:   len(s.graph.Jobs),
		RunningJobs: len(s.runningJobs),
	}

	for _, job := range s.graph.Jobs {
		switch job.State {
		case JobStatePending, JobStateWaiting:
			stats.PendingJobs++
		case JobStatePassed:
			stats.PassedJobs++
		case JobStateFailed:
			stats.FailedJobs++
		case JobStateBroken:
			stats.BrokenJobs++
		case JobStateSkipped:
			stats.SkippedJobs++
		}
	}

	return stats
}

// SchedulerStats contains scheduler statistics
type SchedulerStats struct {
	TotalJobs   int
	PendingJobs int
	RunningJobs int
	PassedJobs  int
	FailedJobs  int
	BrokenJobs  int
	SkippedJobs int
}

// IsComplete returns true if the scheduler has finished processing all jobs
func (s *Scheduler) IsComplete() bool {
	return s.graph.AllJobsTerminal()
}

// HasFailures returns true if any jobs failed
func (s *Scheduler) HasFailures() bool {
	for _, job := range s.graph.Jobs {
		if job.State == JobStateFailed || job.State == JobStateBroken {
			return true
		}
	}
	return false
}
