package pipelinerun

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Executor runs jobs directly without buildkite-agent
type Executor struct {
	graph      *JobGraph
	maxWorkers int
	workDir    string
	env        map[string]string
	output     io.Writer
	debug      bool

	mu      sync.Mutex
	running int
}

// NewExecutor creates a new direct executor
func NewExecutor(graph *JobGraph, maxWorkers int, workDir string) *Executor {
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	return &Executor{
		graph:      graph,
		maxWorkers: maxWorkers,
		workDir:    workDir,
		env:        make(map[string]string),
		output:     os.Stdout,
	}
}

// SetEnv sets environment variables for job execution
func (e *Executor) SetEnv(env map[string]string) {
	for k, v := range env {
		e.env[k] = v
	}
}

// SetOutput sets the output writer
func (e *Executor) SetOutput(w io.Writer) {
	e.output = w
}

// SetDebug enables debug output
func (e *Executor) SetDebug(debug bool) {
	e.debug = debug
}

// Run executes all jobs in the graph respecting dependencies
func (e *Executor) Run(ctx context.Context) error {
	// Simple sequential execution respecting dependencies
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if all jobs are done
		if e.graph.AllJobsTerminal() {
			break
		}

		// Find ready jobs
		readyJobs := e.graph.GetReadyJobs()
		if len(readyJobs) == 0 {
			// No ready jobs but not all terminal - might be blocked
			hasBlocked := false
			for _, job := range e.graph.Jobs {
				if job.State == JobStatePending {
					hasBlocked = true
					break
				}
			}
			if hasBlocked {
				// Check if all pending jobs have failed dependencies
				allBlocked := true
				for _, job := range e.graph.Jobs {
					if job.State == JobStatePending {
						deps := e.graph.GetDependencies(job.ID)
						for _, depID := range deps {
							if dep, ok := e.graph.GetJob(depID); ok {
								if !dep.State.IsTerminal() {
									allBlocked = false
									break
								}
							}
						}
					}
				}
				if allBlocked {
					// Mark remaining pending jobs as broken
					for _, job := range e.graph.Jobs {
						if job.State == JobStatePending {
							e.graph.UpdateJobState(job.ID, JobStateBroken)
						}
					}
				}
			}
			continue
		}

		// Execute ready jobs (up to maxWorkers concurrently)
		var wg sync.WaitGroup
		semaphore := make(chan struct{}, e.maxWorkers)

		for _, job := range readyJobs {
			// Skip non-command jobs
			if job.Type == StepTypeWait {
				e.graph.UpdateJobState(job.ID, JobStatePassed)
				continue
			}
			if job.Type == StepTypeBlock || job.Type == StepTypeInput {
				// Auto-unblock for local runs
				e.graph.UpdateJobState(job.ID, JobStatePassed)
				continue
			}
			if job.Type == StepTypeTrigger {
				// Can't trigger remote pipelines locally
				fmt.Fprintf(e.output, "⏭ Skipping trigger: %s\n", job.Name)
				e.graph.UpdateJobState(job.ID, JobStateSkipped)
				continue
			}

			semaphore <- struct{}{}
			wg.Add(1)

			go func(j *Job) {
				defer wg.Done()
				defer func() { <-semaphore }()

				e.executeJob(ctx, j)
			}(job)
		}

		wg.Wait()
	}

	return nil
}

func (e *Executor) executeJob(ctx context.Context, job *Job) {
	e.graph.UpdateJobState(job.ID, JobStateRunning)

	fmt.Fprintf(e.output, "▶ Running: %s\n", job.Name)

	// Build command
	var cmdStr string
	if job.Command != "" {
		cmdStr = job.Command
	} else if len(job.Commands) > 0 {
		cmdStr = strings.Join(job.Commands, " && ")
	} else {
		fmt.Fprintf(e.output, "✓ %s (no command)\n", job.Name)
		e.graph.UpdateJobState(job.ID, JobStatePassed)
		return
	}

	// Create command
	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	cmd.Dir = e.workDir

	// Build environment
	cmd.Env = os.Environ()
	for k, v := range e.env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range job.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add Buildkite environment variables
	cmd.Env = append(cmd.Env,
		"BUILDKITE=true",
		"BUILDKITE_LOCAL_RUN=true",
		fmt.Sprintf("BUILDKITE_JOB_ID=%s", job.ID),
		fmt.Sprintf("BUILDKITE_LABEL=%s", job.Name),
		fmt.Sprintf("BUILDKITE_STEP_KEY=%s", job.Key),
	)

	if job.ParallelJobCount > 0 {
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("BUILDKITE_PARALLEL_JOB=%d", job.ParallelJob),
			fmt.Sprintf("BUILDKITE_PARALLEL_JOB_COUNT=%d", job.ParallelJobCount),
		)
	}

	// Connect output
	if e.debug {
		cmd.Stdout = e.output
		cmd.Stderr = e.output
	} else {
		// Capture output but don't show unless error
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	}

	// Run command
	err := cmd.Run()

	if err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		job.ExitCode = exitCode

		// Check soft fail
		if job.SoftFail {
			fmt.Fprintf(e.output, "⚠ %s (soft fail, exit %d)\n", job.Name, exitCode)
			e.graph.UpdateJobState(job.ID, JobStatePassed)
			return
		}

		for _, code := range job.SoftFailCode {
			if code == exitCode {
				fmt.Fprintf(e.output, "⚠ %s (soft fail, exit %d)\n", job.Name, exitCode)
				e.graph.UpdateJobState(job.ID, JobStatePassed)
				return
			}
		}

		fmt.Fprintf(e.output, "✗ %s (exit %d)\n", job.Name, exitCode)
		e.graph.UpdateJobState(job.ID, JobStateFailed)

		// Mark dependents as broken
		e.markDependentsBroken(job.ID)
	} else {
		fmt.Fprintf(e.output, "✓ %s\n", job.Name)
		e.graph.UpdateJobState(job.ID, JobStatePassed)
	}
}

func (e *Executor) markDependentsBroken(jobID string) {
	visited := make(map[string]bool)
	e.markDependentsBrokenRecursive(jobID, visited)
}

func (e *Executor) markDependentsBrokenRecursive(jobID string, visited map[string]bool) {
	if visited[jobID] {
		return
	}
	visited[jobID] = true

	dependents := e.graph.GetDependents(jobID)
	for _, depID := range dependents {
		job, ok := e.graph.GetJob(depID)
		if !ok {
			continue
		}
		if !job.AllowDepFail && job.State == JobStatePending {
			e.graph.UpdateJobState(depID, JobStateBroken)
			e.markDependentsBrokenRecursive(depID, visited)
		}
	}
}
