package pipelinerun

import (
	"sync"
	"time"
)

// StepType represents the type of pipeline step
type StepType string

const (
	StepTypeCommand StepType = "command"
	StepTypeWait    StepType = "wait"
	StepTypeBlock   StepType = "block"
	StepTypeInput   StepType = "input"
	StepTypeTrigger StepType = "trigger"
	StepTypeGroup   StepType = "group"
)

// JobState represents the current state of a job
type JobState string

const (
	JobStatePending       JobState = "pending"
	JobStateWaiting       JobState = "waiting"
	JobStateRunning       JobState = "running"
	JobStatePassed        JobState = "passed"
	JobStateFailed        JobState = "failed"
	JobStateCanceled      JobState = "canceled"
	JobStateTimedOut      JobState = "timed_out"
	JobStateSkipped       JobState = "skipped"
	JobStateBlocked       JobState = "blocked"
	JobStateUnblocked     JobState = "unblocked"
	JobStateBroken        JobState = "broken"
	JobStateWaitingFailed JobState = "waiting_failed"
)

// IsTerminal returns true if the job state is a final state
func (s JobState) IsTerminal() bool {
	switch s {
	case JobStatePassed, JobStateFailed, JobStateCanceled, JobStateTimedOut, JobStateSkipped, JobStateBroken:
		return true
	}
	return false
}

// IsSuccess returns true if the job completed successfully
func (s JobState) IsSuccess() bool {
	return s == JobStatePassed
}

// Job represents a single executable unit in the pipeline
type Job struct {
	ID   string
	Name string
	Type StepType

	// Command execution
	Command  string
	Commands []string
	Env      map[string]string
	Plugins  []Plugin

	// Parallelism support
	ParallelJob      int // 0-indexed parallel job number
	ParallelJobCount int // Total number of parallel jobs (0 means not parallel)

	// Matrix expansion
	MatrixValues map[string]string // Values for this matrix combination

	// Dependencies
	DependsOn    []string // Job IDs this job depends on
	AllowDepFail bool     // Continue even if dependencies fail

	// Concurrency control
	ConcurrencyGroup string
	ConcurrencyLimit int

	// State
	State     JobState
	ExitCode  int
	StartedAt time.Time
	EndedAt   time.Time

	// Block/Input step fields
	BlockedState string
	Prompt       string
	Fields       []InputField

	// Trigger step fields
	TriggerPipeline string
	TriggerBuild    map[string]any

	// Agent requirements
	AgentQueryRules []string

	// Artifacts
	ArtifactPaths []string

	// Soft fail configuration
	SoftFail     bool
	SoftFailCode []int

	// Timeouts
	TimeoutInMinutes int

	// Retry configuration
	Retry *RetryConfig

	// Step key for depends_on references
	Key string

	// Original step index for ordering
	StepIndex int
}

// InputField represents a field in a block/input step
type InputField struct {
	Key      string
	Text     string
	Hint     string
	Required bool
	Default  string
	Select   string
	Options  []SelectOption
	Multiple bool
}

// SelectOption represents an option in a select field
type SelectOption struct {
	Label string
	Value string
}

// Plugin represents a Buildkite plugin configuration
type Plugin struct {
	Name   string
	Config map[string]any
}

// RetryConfig represents retry configuration for a job
type RetryConfig struct {
	Automatic []AutomaticRetry
	Manual    *ManualRetry
}

// AutomaticRetry represents automatic retry rules
type AutomaticRetry struct {
	ExitStatus   string
	Limit        int
	Signal       string
	SignalReason string
}

// ManualRetry represents manual retry configuration
type ManualRetry struct {
	Allowed        bool
	Reason         string
	PermitOnPassed bool
}

// JobGraph represents the directed acyclic graph of jobs
type JobGraph struct {
	mu   sync.RWMutex
	Jobs map[string]*Job

	// Adjacency lists for dependencies
	// dependents[A] = [B, C] means B and C depend on A
	dependents map[string][]string
	// dependencies[B] = [A] means B depends on A
	dependencies map[string][]string

	// Job ordering for execution
	orderedJobIDs []string
}

// NewJobGraph creates a new empty job graph
func NewJobGraph() *JobGraph {
	return &JobGraph{
		Jobs:         make(map[string]*Job),
		dependents:   make(map[string][]string),
		dependencies: make(map[string][]string),
	}
}

// AddJob adds a job to the graph
func (g *JobGraph) AddJob(job *Job) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.Jobs[job.ID] = job
	g.orderedJobIDs = append(g.orderedJobIDs, job.ID)
}

// AddDependency adds a dependency edge: dependent depends on dependency
func (g *JobGraph) AddDependency(dependentID, dependencyID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.dependents[dependencyID] = append(g.dependents[dependencyID], dependentID)
	g.dependencies[dependentID] = append(g.dependencies[dependentID], dependencyID)
}

// GetJob returns a job by ID
func (g *JobGraph) GetJob(id string) (*Job, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	job, ok := g.Jobs[id]
	return job, ok
}

// GetDependencies returns the IDs of jobs that the given job depends on
func (g *JobGraph) GetDependencies(jobID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.dependencies[jobID]
}

// GetDependents returns the IDs of jobs that depend on the given job
func (g *JobGraph) GetDependents(jobID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.dependents[jobID]
}

// GetOrderedJobs returns jobs in their original order
func (g *JobGraph) GetOrderedJobs() []*Job {
	g.mu.RLock()
	defer g.mu.RUnlock()

	jobs := make([]*Job, 0, len(g.orderedJobIDs))
	for _, id := range g.orderedJobIDs {
		if job, ok := g.Jobs[id]; ok {
			jobs = append(jobs, job)
		}
	}
	return jobs
}

// GetReadyJobs returns jobs that have no unfinished dependencies
func (g *JobGraph) GetReadyJobs() []*Job {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var ready []*Job
	for _, job := range g.Jobs {
		if job.State != JobStatePending {
			continue
		}
		if g.areDependenciesSatisfied(job) {
			ready = append(ready, job)
		}
	}
	return ready
}

// areDependenciesSatisfied checks if all dependencies of a job are complete
// Must be called with lock held
func (g *JobGraph) areDependenciesSatisfied(job *Job) bool {
	deps := g.dependencies[job.ID]
	for _, depID := range deps {
		dep, ok := g.Jobs[depID]
		if !ok {
			return false
		}
		if !dep.State.IsTerminal() {
			return false
		}
		if !dep.State.IsSuccess() && !job.AllowDepFail {
			return false
		}
	}
	return true
}

// UpdateJobState updates the state of a job
func (g *JobGraph) UpdateJobState(jobID string, state JobState) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if job, ok := g.Jobs[jobID]; ok {
		job.State = state
		if state == JobStateRunning && job.StartedAt.IsZero() {
			job.StartedAt = time.Now()
		}
		if state.IsTerminal() && job.EndedAt.IsZero() {
			job.EndedAt = time.Now()
		}
	}
}

// AllJobsTerminal returns true if all jobs have reached a terminal state
func (g *JobGraph) AllJobsTerminal() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, job := range g.Jobs {
		if !job.State.IsTerminal() {
			return false
		}
	}
	return true
}

// HasFailedJobs returns true if any job has failed
func (g *JobGraph) HasFailedJobs() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, job := range g.Jobs {
		if job.State == JobStateFailed || job.State == JobStateBroken || job.State == JobStateTimedOut {
			return false
		}
	}
	return true
}

// Pipeline represents a parsed pipeline configuration
type Pipeline struct {
	Env   map[string]string
	Steps []Step

	// Top-level agent requirements
	AgentQueryRules []string
}

// Step represents a step in the pipeline before expansion into jobs
type Step struct {
	Type StepType

	// Common fields
	Key                    string
	Label                  string
	Name                   string // Alias for Label
	If                     string
	DependsOn              []string
	AllowDependencyFailure bool

	// Command step fields
	Command          string
	Commands         []string
	Env              map[string]string
	Plugins          []Plugin
	Parallelism      int
	Matrix           *MatrixConfig
	Concurrency      int
	ConcurrencyGroup string
	ArtifactPaths    []string
	AgentQueryRules  []string
	TimeoutInMinutes int
	SoftFail         any // Can be bool or []SoftFailRule
	Retry            *RetryConfig

	// Wait step fields
	ContinueOnFailure bool

	// Block/Input step fields
	BlockedState string
	Prompt       string
	Fields       []InputField
	Branches     string
	AllowedTeams []string

	// Trigger step fields
	Trigger string
	Build   map[string]any
	Async   bool

	// Group step fields
	Group string
	Steps []Step
}

// MatrixConfig represents matrix build configuration
type MatrixConfig struct {
	Setup       map[string][]string
	Adjustments []MatrixAdjustment
}

// MatrixAdjustment represents a matrix adjustment/modification
type MatrixAdjustment struct {
	With     map[string]string
	Skip     bool
	SoftFail bool
}

// SoftFailRule represents a soft_fail exit code rule
type SoftFailRule struct {
	ExitStatus int
}
