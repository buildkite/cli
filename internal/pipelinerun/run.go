package pipelinerun

import (
	"context"
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

	// Path to buildkite-agent binary
	AgentBinary string

	// Build directory
	BuildPath string

	// Dry run mode - just plan, don't execute
	DryRun bool

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
		return dryRun(config.Output, graph)
	}

	// Find agent binary
	agentBinary := config.AgentBinary
	if agentBinary == "" {
		agentBinary, err = FindAgentBinary()
		if err != nil {
			return nil, fmt.Errorf("finding buildkite-agent: %w", err)
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
		return nil, fmt.Errorf("starting server: %w", err)
	}
	defer func() { _ = server.Stop() }()

	fmt.Fprintf(config.Output, "Mock server started on %s\n", server.URL())

	// Create and start agent
	buildPath := config.BuildPath
	if buildPath == "" {
		buildPath, err = os.MkdirTemp("", "bk-local-*")
		if err != nil {
			return nil, fmt.Errorf("creating build directory: %w", err)
		}
		defer os.RemoveAll(buildPath)
	}

	agent := NewAgent(&AgentConfig{
		BinaryPath: agentBinary,
		Spawn:      spawn,
		Endpoint:   server.URL(),
		Token:      "local-token",
		BuildPath:  buildPath,
		Debug:      config.Debug,
	})

	// Set up signal handling
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
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
	fmt.Fprintf(config.Output, "Starting agent with %d workers...\n", spawn)
	if err := agent.Start(ctx); err != nil {
		return nil, fmt.Errorf("starting agent: %w", err)
	}

	startTime := time.Now()

	// Monitor progress
	go monitorProgress(ctx, config.Output, scheduler, config.Debug)

	// Wait for completion
	select {
	case <-scheduler.Done():
		fmt.Fprintf(config.Output, "\nPipeline execution complete\n")
	case <-ctx.Done():
		fmt.Fprintf(config.Output, "\nPipeline execution cancelled\n")
	}

	// Stop agent
	_ = agent.Stop()

	// Gather results
	duration := time.Since(startTime)
	stats := scheduler.GetStats()

	var failedJobIDs []string
	for _, job := range graph.Jobs {
		if job.State == JobStateFailed || job.State == JobStateBroken {
			failedJobIDs = append(failedJobIDs, job.ID)
		}
	}

	result := &RunResult{
		Success:      !scheduler.HasFailures(),
		TotalJobs:    stats.TotalJobs,
		PassedJobs:   stats.PassedJobs,
		FailedJobs:   stats.FailedJobs + stats.BrokenJobs,
		SkippedJobs:  stats.SkippedJobs,
		Duration:     duration,
		FailedJobIDs: failedJobIDs,
	}

	printSummary(config.Output, result, graph)

	return result, nil
}

func dryRun(w io.Writer, graph *JobGraph) (*RunResult, error) {
	fmt.Fprintf(w, "\n=== Dry Run ===\n\n")

	for i, job := range graph.GetOrderedJobs() {
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
			fmt.Fprintf(w, "   Depends on: %d jobs\n", len(deps))
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
