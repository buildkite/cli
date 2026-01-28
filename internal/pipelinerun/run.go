package pipelinerun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// RunConfig holds configuration for a pipeline run
type RunConfig struct {
	// Pipeline file path
	PipelineFile string

	// Number of agent workers to spawn (0 = auto-detect)
	Spawn int

	// Additional environment variables
	Env map[string]string

	// Port for the mock server (0 = auto)
	Port int

	// Use buildkite-agent for execution (enables plugins)
	UseAgent bool

	// Path to buildkite-agent binary
	AgentBinary string

	// Build directory
	BuildPath string

	// Dry run mode - just plan, don't execute
	DryRun bool

	// Steps to run (by ID or key) - empty means all
	Steps []string

	// JSON output mode
	JSON bool

	// Debug mode
	Debug bool

	// Output writer
	Output io.Writer
}

// RunResult holds the result of a pipeline run
type RunResult struct {
	Success      bool
	TotalJobs    int
	PassedJobs   int
	FailedJobs   int
	SkippedJobs  int
	Duration     time.Duration
	FailedJobIDs []string
}

// Run executes a pipeline locally
func Run(ctx context.Context, config *RunConfig) (*RunResult, error) {
	if config.Output == nil {
		config.Output = os.Stdout
	}

	// Load the pipeline
	fmt.Fprintf(config.Output, "Loading pipeline from %s\n", config.PipelineFile)

	pipeline, err := LoadPipeline(config.PipelineFile)
	if err != nil {
		return nil, fmt.Errorf("loading pipeline: %w", err)
	}

	fmt.Fprintf(config.Output, "Found %d steps\n", len(pipeline.Steps))

	// Plan the pipeline
	planner := NewPlanner(pipeline)
	if config.Env != nil {
		planner.WithEnv(config.Env)
	}

	graph, err := planner.Plan()
	if err != nil {
		return nil, fmt.Errorf("planning pipeline: %w", err)
	}

	fmt.Fprintf(config.Output, "Planned %d jobs\n", len(graph.Jobs))

	// Filter to specific steps if requested
	if len(config.Steps) > 0 {
		graph, err = filterGraphToSteps(graph, config.Steps)
		if err != nil {
			return nil, fmt.Errorf("filtering steps: %w", err)
		}
		fmt.Fprintf(config.Output, "Filtered to %d jobs\n", len(graph.Jobs))
	}

	// Calculate concurrency
	maxConcurrency := CalculateMaxConcurrency(graph)
	spawn := config.Spawn
	if spawn <= 0 {
		spawn = maxConcurrency
	}
	if spawn > 10 {
		spawn = 10 // Cap at reasonable maximum
	}

	fmt.Fprintf(config.Output, "Max concurrency: %d, spawning %d agents\n", maxConcurrency, spawn)

	// Dry run mode
	if config.DryRun {
		return dryRun(config.Output, graph, config.JSON)
	}

	// Set up signal handling
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	startTime := time.Now()

	// Choose execution mode
	if config.UseAgent {
		err = runWithAgent(ctx, config, graph, spawn, sigChan, cancel)
	} else {
		err = runDirect(ctx, config, graph, spawn, sigChan, cancel)
	}

	if err != nil && err != context.Canceled {
		return nil, err
	}

	// Gather results
	duration := time.Since(startTime)

	var passed, failed, skipped, broken int
	for _, job := range graph.Jobs {
		switch job.State {
		case JobStatePassed:
			passed++
		case JobStateFailed:
			failed++
		case JobStateSkipped:
			skipped++
		case JobStateBroken:
			broken++
		}
	}

	var failedJobIDs []string
	for _, job := range graph.Jobs {
		if job.State == JobStateFailed || job.State == JobStateBroken {
			failedJobIDs = append(failedJobIDs, job.ID)
		}
	}

	result := &RunResult{
		Success:      failed == 0 && broken == 0,
		TotalJobs:    len(graph.Jobs),
		PassedJobs:   passed,
		FailedJobs:   failed + broken,
		SkippedJobs:  skipped,
		Duration:     duration,
		FailedJobIDs: failedJobIDs,
	}

	printSummary(config.Output, result, graph)

	return result, nil
}

// runDirect executes jobs directly via bash subprocesses
func runDirect(ctx context.Context, config *RunConfig, graph *JobGraph, spawn int, sigChan chan os.Signal, cancel context.CancelFunc) error {
	// Determine working directory
	workDir := config.BuildPath
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
	}

	go func() {
		select {
		case <-sigChan:
			fmt.Fprintf(config.Output, "\nReceived interrupt, shutting down...\n")
			cancel()
		case <-ctx.Done():
		}
	}()

	// Create and run executor
	executor := NewExecutor(graph, spawn, workDir)
	executor.SetEnv(config.Env)
	executor.SetOutput(config.Output)
	executor.SetDebug(config.Debug)

	fmt.Fprintf(config.Output, "\nRunning %d jobs with %d workers...\n\n", len(graph.Jobs), spawn)

	return executor.Run(ctx)
}

// runWithAgent executes jobs using buildkite-agent (supports plugins)
func runWithAgent(ctx context.Context, config *RunConfig, graph *JobGraph, spawn int, sigChan chan os.Signal, cancel context.CancelFunc) error {
	// Find agent binary
	agentBinary := config.AgentBinary
	if agentBinary == "" {
		var err error
		agentBinary, err = FindAgentBinary()
		if err != nil {
			return fmt.Errorf("finding buildkite-agent: %w", err)
		}
	}
	fmt.Fprintf(config.Output, "Using agent: %s\n", agentBinary)

	// Create scheduler
	scheduler := NewScheduler(graph, spawn)

	// Create mock server
	server := NewServer(scheduler, graph, config.Port)
	server.SetDebug(config.Debug)
	server.SetPipelineUploadHandler(func(jobID string, p *Pipeline) error {
		return scheduler.HandlePipelineUpload(jobID, p)
	})

	if err := server.Start(); err != nil {
		return fmt.Errorf("starting server: %w", err)
	}
	defer func() { _ = server.Stop() }()

	fmt.Fprintf(config.Output, "Mock server started on %s\n", server.URL())

	// Create agent
	agent := NewAgent(&AgentConfig{
		BinaryPath: agentBinary,
		Spawn:      spawn,
		Endpoint:   server.URL(),
		Token:      "local-token",
		BuildPath:  config.BuildPath,
		Debug:      config.Debug,
	})

	go func() {
		select {
		case <-sigChan:
			fmt.Fprintf(config.Output, "\nReceived interrupt, shutting down...\n")
			cancel()
			scheduler.Stop()
			_ = agent.Stop()
		case <-ctx.Done():
		}
	}()

	// Start the scheduler
	scheduler.Start()

	// Start the agent
	fmt.Fprintf(config.Output, "\nStarting agent with %d workers...\n\n", spawn)
	if err := agent.Start(ctx); err != nil {
		return fmt.Errorf("starting agent: %w", err)
	}

	// Wait for completion
	select {
	case <-scheduler.Done():
		fmt.Fprintf(config.Output, "\nPipeline execution complete\n")
	case <-ctx.Done():
		fmt.Fprintf(config.Output, "\nPipeline execution cancelled\n")
	}

	// Stop agent
	_ = agent.Stop()

	return nil
}

// DryRunJob represents a job in dry run output
type DryRunJob struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Type             string            `json:"type"`
	Key              string            `json:"key,omitempty"`
	Command          string            `json:"command,omitempty"`
	Commands         []string          `json:"commands,omitempty"`
	ParallelJob      int               `json:"parallel_job,omitempty"`
	ParallelJobCount int               `json:"parallel_job_count,omitempty"`
	Matrix           map[string]string `json:"matrix,omitempty"`
	DependsOn        []string          `json:"depends_on,omitempty"`
}

// DryRunOutput represents the full dry run output
type DryRunOutput struct {
	Jobs           []DryRunJob `json:"jobs"`
	TotalJobs      int         `json:"total_jobs"`
	MaxConcurrency int         `json:"max_concurrency"`
}

func dryRun(w io.Writer, graph *JobGraph, jsonOutput bool) (*RunResult, error) {
	jobs := graph.GetOrderedJobs()
	maxConcurrency := CalculateMaxConcurrency(graph)

	// Build job ID to name map for dependency resolution
	jobNames := make(map[string]string)
	for _, job := range jobs {
		name := job.Name
		if name == "" {
			name = string(job.Type)
		}
		jobNames[job.ID] = name
	}

	if jsonOutput {
		output := DryRunOutput{
			Jobs:           make([]DryRunJob, 0, len(jobs)),
			TotalJobs:      len(jobs),
			MaxConcurrency: maxConcurrency,
		}

		for _, job := range jobs {
			deps := graph.GetDependencies(job.ID)
			depNames := make([]string, 0, len(deps))
			for _, depID := range deps {
				if name, ok := jobNames[depID]; ok {
					depNames = append(depNames, name)
				}
			}

			dryJob := DryRunJob{
				ID:               job.ID,
				Name:             job.Name,
				Type:             string(job.Type),
				Key:              job.Key,
				Command:          job.Command,
				Commands:         job.Commands,
				ParallelJob:      job.ParallelJob,
				ParallelJobCount: job.ParallelJobCount,
				Matrix:           job.MatrixValues,
				DependsOn:        depNames,
			}
			output.Jobs = append(output.Jobs, dryJob)
		}

		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(output); err != nil {
			return nil, fmt.Errorf("encoding JSON: %w", err)
		}
	} else {
		fmt.Fprintf(w, "\n=== Dry Run ===\n\n")

		for i, job := range jobs {
			fmt.Fprintf(w, "%d. [%s] %s\n", i+1, job.Type, job.Name)

			if job.Command != "" {
				fmt.Fprintf(w, "   Command: %s\n", job.Command)
			}
			if len(job.Commands) > 0 {
				fmt.Fprintf(w, "   Commands:\n")
				for _, cmd := range job.Commands {
					fmt.Fprintf(w, "     - %s\n", cmd)
				}
			}

			if job.ParallelJobCount > 0 {
				fmt.Fprintf(w, "   Parallel: %d of %d\n", job.ParallelJob+1, job.ParallelJobCount)
			}

			if len(job.MatrixValues) > 0 {
				fmt.Fprintf(w, "   Matrix: %v\n", job.MatrixValues)
			}

			deps := graph.GetDependencies(job.ID)
			if len(deps) > 0 {
				fmt.Fprintf(w, "   Depends on:\n")
				for _, depID := range deps {
					if name, ok := jobNames[depID]; ok {
						fmt.Fprintf(w, "     - %s\n", name)
					}
				}
			}
		}
	}

	return &RunResult{
		Success:   true,
		TotalJobs: len(graph.Jobs),
	}, nil
}

func monitorProgress(ctx context.Context, w io.Writer, scheduler *Scheduler, debug bool) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastStats := SchedulerStats{}

	for {
		select {
		case <-ctx.Done():
			return
		case <-scheduler.Done():
			return
		case event := <-scheduler.Events():
			if debug {
				switch e := event.(type) {
				case JobStartedEvent:
					job, _ := scheduler.graph.GetJob(e.JobID)
					if job != nil {
						fmt.Fprintf(w, "▶ Started: %s\n", job.Name)
					}
				case JobFinishedEvent:
					job, _ := scheduler.graph.GetJob(e.JobID)
					if job != nil {
						icon := "✓"
						if e.State == JobStateFailed {
							icon = "✗"
						}
						fmt.Fprintf(w, "%s Finished: %s (%s)\n", icon, job.Name, e.State)
					}
				case PipelineUploadEvent:
					fmt.Fprintf(w, "⬆ Pipeline uploaded (%d steps)\n", len(e.Pipeline.Steps))
				}
			}
		case <-ticker.C:
			stats := scheduler.GetStats()
			if stats != lastStats {
				fmt.Fprintf(w, "Progress: %d/%d jobs (✓%d ✗%d ⏳%d)\n",
					stats.PassedJobs+stats.FailedJobs+stats.SkippedJobs,
					stats.TotalJobs,
					stats.PassedJobs,
					stats.FailedJobs,
					stats.RunningJobs,
				)
				lastStats = stats
			}
		}
	}
}

func printSummary(w io.Writer, result *RunResult, graph *JobGraph) {
	fmt.Fprintf(w, "\n=== Summary ===\n")
	fmt.Fprintf(w, "Duration: %s\n", result.Duration.Round(time.Second))
	fmt.Fprintf(w, "Total jobs: %d\n", result.TotalJobs)
	fmt.Fprintf(w, "Passed: %d\n", result.PassedJobs)
	fmt.Fprintf(w, "Failed: %d\n", result.FailedJobs)
	fmt.Fprintf(w, "Skipped: %d\n", result.SkippedJobs)

	if len(result.FailedJobIDs) > 0 {
		fmt.Fprintf(w, "\nFailed jobs:\n")
		for _, jobID := range result.FailedJobIDs {
			if job, ok := graph.GetJob(jobID); ok {
				fmt.Fprintf(w, "  - %s (exit code: %d)\n", job.Name, job.ExitCode)
			}
		}
	}

	if result.Success {
		fmt.Fprintf(w, "\n✓ Pipeline passed\n")
	} else {
		fmt.Fprintf(w, "\n✗ Pipeline failed\n")
	}
}

// filterGraphToSteps creates a new graph containing only the specified steps
// and their dependencies
func filterGraphToSteps(graph *JobGraph, stepFilters []string) (*JobGraph, error) {
	// Build a set of matching job IDs
	matchingJobs := make(map[string]bool)

	for _, filter := range stepFilters {
		found := false
		for _, job := range graph.Jobs {
			// Match by ID, key, or name
			if job.ID == filter || job.Key == filter || job.Name == filter {
				matchingJobs[job.ID] = true
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("step not found: %s", filter)
		}
	}

	// If no matches, return error
	if len(matchingJobs) == 0 {
		return nil, fmt.Errorf("no matching steps found")
	}

	// Create new graph with only matching jobs
	// Dependencies are removed since we're running specific steps
	newGraph := NewJobGraph()

	for id := range matchingJobs {
		if job, ok := graph.GetJob(id); ok {
			// Clone the job without dependencies
			newJob := *job
			newJob.DependsOn = nil
			newGraph.AddJob(&newJob)
		}
	}

	return newGraph, nil
}
