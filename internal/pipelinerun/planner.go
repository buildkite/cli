package pipelinerun

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Planner converts a Pipeline into a JobGraph
type Planner struct {
	pipeline *Pipeline
	env      map[string]string
}

// NewPlanner creates a new planner for the given pipeline
func NewPlanner(p *Pipeline) *Planner {
	return &Planner{
		pipeline: p,
		env:      make(map[string]string),
	}
}

// WithEnv adds environment variables to the planner
func (p *Planner) WithEnv(env map[string]string) *Planner {
	for k, v := range env {
		p.env[k] = v
	}
	return p
}

// Plan converts the pipeline into a job graph
func (p *Planner) Plan() (*JobGraph, error) {
	graph := NewJobGraph()

	// Merge pipeline-level env with planner env
	mergedEnv := make(map[string]string)
	for k, v := range p.pipeline.Env {
		mergedEnv[k] = v
	}
	for k, v := range p.env {
		mergedEnv[k] = v
	}

	// Track the last "barrier" jobs (wait steps create barriers)
	var lastBarrierJobs []string
	var currentGroupJobs []string

	// Process steps in order
	for i, step := range p.pipeline.Steps {
		jobs, err := p.expandStep(&step, i, mergedEnv, p.pipeline.AgentQueryRules)
		if err != nil {
			return nil, fmt.Errorf("expanding step %d: %w", i, err)
		}

		if step.Type == StepTypeWait {
			// Wait steps create a barrier
			// All subsequent jobs depend on all jobs before the wait
			if len(jobs) == 1 {
				waitJob := jobs[0]
				graph.AddJob(waitJob)

				// Wait job depends on all current group jobs
				for _, jobID := range currentGroupJobs {
					graph.AddDependency(waitJob.ID, jobID)
				}

				// Update barrier
				lastBarrierJobs = []string{waitJob.ID}
				currentGroupJobs = nil
			}
		} else {
			// Add jobs and wire dependencies
			for _, job := range jobs {
				graph.AddJob(job)

				// Explicit depends_on takes precedence
				if len(step.DependsOn) > 0 {
					for _, depKey := range step.DependsOn {
						// Find job(s) with matching key
						for _, j := range graph.Jobs {
							if j.Key == depKey {
								graph.AddDependency(job.ID, j.ID)
							}
						}
					}
				} else {
					// Implicit dependency on last barrier
					for _, barrierID := range lastBarrierJobs {
						graph.AddDependency(job.ID, barrierID)
					}
				}

				currentGroupJobs = append(currentGroupJobs, job.ID)
			}
		}
	}

	return graph, nil
}

// expandStep expands a step into one or more jobs
func (p *Planner) expandStep(step *Step, index int, env map[string]string, defaultAgentRules []string) ([]*Job, error) {
	switch step.Type {
	case StepTypeWait:
		return p.expandWaitStep(step, index)
	case StepTypeBlock, StepTypeInput:
		return p.expandBlockStep(step, index, env)
	case StepTypeTrigger:
		return p.expandTriggerStep(step, index, env)
	case StepTypeGroup:
		return p.expandGroupStep(step, index, env, defaultAgentRules)
	case StepTypeCommand:
		return p.expandCommandStep(step, index, env, defaultAgentRules)
	default:
		return nil, fmt.Errorf("unknown step type: %s", step.Type)
	}
}

func (p *Planner) expandWaitStep(step *Step, index int) ([]*Job, error) {
	job := &Job{
		ID:           generateJobID(),
		Type:         StepTypeWait,
		State:        JobStatePending,
		StepIndex:    index,
		AllowDepFail: step.ContinueOnFailure,
	}
	return []*Job{job}, nil
}

func (p *Planner) expandBlockStep(step *Step, index int, env map[string]string) ([]*Job, error) {
	label := step.Label
	if label == "" {
		label = step.Name
	}
	if label == "" {
		label = "Block"
	}

	job := &Job{
		ID:           generateJobID(),
		Name:         ExpandEnvVars(label, env),
		Type:         step.Type,
		Key:          step.Key,
		State:        JobStatePending,
		StepIndex:    index,
		BlockedState: step.BlockedState,
		Prompt:       step.Prompt,
		Fields:       step.Fields,
		DependsOn:    step.DependsOn,
		AllowDepFail: step.AllowDependencyFailure,
	}
	return []*Job{job}, nil
}

func (p *Planner) expandTriggerStep(step *Step, index int, env map[string]string) ([]*Job, error) {
	job := &Job{
		ID:              generateJobID(),
		Name:            fmt.Sprintf("Trigger: %s", step.Trigger),
		Type:            StepTypeTrigger,
		Key:             step.Key,
		State:           JobStatePending,
		StepIndex:       index,
		TriggerPipeline: step.Trigger,
		TriggerBuild:    step.Build,
		DependsOn:       step.DependsOn,
		AllowDepFail:    step.AllowDependencyFailure,
	}
	return []*Job{job}, nil
}

func (p *Planner) expandGroupStep(step *Step, index int, env map[string]string, defaultAgentRules []string) ([]*Job, error) {
	var jobs []*Job
	for i, childStep := range step.Steps {
		childJobs, err := p.expandStep(&childStep, index*1000+i, env, defaultAgentRules)
		if err != nil {
			return nil, fmt.Errorf("expanding group step %d: %w", i, err)
		}
		jobs = append(jobs, childJobs...)
	}
	return jobs, nil
}

func (p *Planner) expandCommandStep(step *Step, index int, env map[string]string, defaultAgentRules []string) ([]*Job, error) {
	// Handle matrix expansion first
	matrixCombinations := expandMatrix(step.Matrix)

	// Handle parallelism
	parallelism := step.Parallelism
	if parallelism < 1 {
		parallelism = 1
	}

	var jobs []*Job

	// For each matrix combination
	for _, matrixValues := range matrixCombinations {
		// Merge environments
		jobEnv := make(map[string]string)
		for k, v := range env {
			jobEnv[k] = v
		}
		for k, v := range step.Env {
			jobEnv[k] = ExpandEnvVars(v, env)
		}
		for k, v := range matrixValues {
			jobEnv[fmt.Sprintf("BUILDKITE_MATRIX_%s", strings.ToUpper(k))] = v
		}

		// Expand matrix variables in label
		label := step.Label
		if label == "" {
			label = step.Name
		}
		if label == "" {
			label = step.Command
			if len(step.Commands) > 0 {
				label = step.Commands[0]
			}
		}
		label = ExpandMatrixVars(label, matrixValues)
		label = ExpandEnvVars(label, jobEnv)

		// For each parallel job
		for pj := 0; pj < parallelism; pj++ {
			job := &Job{
				ID:               generateJobID(),
				Name:             label,
				Type:             StepTypeCommand,
				Key:              step.Key,
				Command:          step.Command,
				Commands:         step.Commands,
				Env:              jobEnv,
				Plugins:          step.Plugins,
				State:            JobStatePending,
				StepIndex:        index,
				DependsOn:        step.DependsOn,
				AllowDepFail:     step.AllowDependencyFailure,
				ConcurrencyGroup: step.ConcurrencyGroup,
				ConcurrencyLimit: step.Concurrency,
				ArtifactPaths:    step.ArtifactPaths,
				TimeoutInMinutes: step.TimeoutInMinutes,
				Retry:            step.Retry,
				MatrixValues:     matrixValues,
			}

			// Set agent rules
			if len(step.AgentQueryRules) > 0 {
				job.AgentQueryRules = step.AgentQueryRules
			} else {
				job.AgentQueryRules = defaultAgentRules
			}

			// Set parallelism info
			if parallelism > 1 {
				job.ParallelJob = pj
				job.ParallelJobCount = parallelism
				job.Env["BUILDKITE_PARALLEL_JOB"] = fmt.Sprintf("%d", pj)
				job.Env["BUILDKITE_PARALLEL_JOB_COUNT"] = fmt.Sprintf("%d", parallelism)
			}

			// Handle soft_fail
			if step.SoftFail != nil {
				switch sf := step.SoftFail.(type) {
				case bool:
					job.SoftFail = sf
				case []any:
					for _, rule := range sf {
						if m, ok := rule.(map[string]any); ok {
							if exitStatus, ok := m["exit_status"].(int); ok {
								job.SoftFailCode = append(job.SoftFailCode, exitStatus)
							}
						}
					}
				}
			}

			jobs = append(jobs, job)
		}
	}

	return jobs, nil
}

// expandMatrix generates all combinations of matrix values
func expandMatrix(matrix *MatrixConfig) []map[string]string {
	if matrix == nil || len(matrix.Setup) == 0 {
		return []map[string]string{{}}
	}

	// Get sorted keys for deterministic order
	var keys []string
	for k := range matrix.Setup {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Generate all combinations
	combinations := []map[string]string{{}}

	for _, key := range keys {
		values := matrix.Setup[key]
		var newCombinations []map[string]string

		for _, combo := range combinations {
			for _, value := range values {
				newCombo := make(map[string]string)
				for k, v := range combo {
					newCombo[k] = v
				}
				newCombo[key] = value
				newCombinations = append(newCombinations, newCombo)
			}
		}

		combinations = newCombinations
	}

	// Apply adjustments (skip combinations)
	if len(matrix.Adjustments) > 0 {
		var filtered []map[string]string
		for _, combo := range combinations {
			skip := false
			for _, adj := range matrix.Adjustments {
				if adj.Skip && matchesAdjustment(combo, adj.With) {
					skip = true
					break
				}
			}
			if !skip {
				filtered = append(filtered, combo)
			}
		}
		combinations = filtered
	}

	return combinations
}

func matchesAdjustment(combo map[string]string, with map[string]string) bool {
	for k, v := range with {
		if combo[k] != v {
			return false
		}
	}
	return true
}

func generateJobID() string {
	return uuid.New().String()
}

// CalculateMaxConcurrency analyzes the job graph and returns the maximum
// number of jobs that can run concurrently at any point
func CalculateMaxConcurrency(graph *JobGraph) int {
	graph.mu.RLock()
	defer graph.mu.RUnlock()

	if len(graph.Jobs) == 0 {
		return 1
	}

	// Group jobs by their "layer" (depth from root)
	layers := make(map[int][]*Job)
	depth := make(map[string]int)

	// Calculate depth for each job using BFS
	var queue []string
	for id := range graph.Jobs {
		deps := graph.dependencies[id]
		if len(deps) == 0 {
			depth[id] = 0
			queue = append(queue, id)
		}
	}

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		job := graph.Jobs[id]
		d := depth[id]
		layers[d] = append(layers[d], job)

		// Process dependents
		for _, depID := range graph.dependents[id] {
			// Dependent's depth is max of all dependencies' depths + 1
			depJob := graph.Jobs[depID]
			if depJob == nil {
				continue
			}

			allDepsResolved := true
			maxDepDepth := 0
			for _, parentID := range graph.dependencies[depID] {
				if d, ok := depth[parentID]; ok {
					if d > maxDepDepth {
						maxDepDepth = d
					}
				} else {
					allDepsResolved = false
				}
			}

			if allDepsResolved {
				if _, ok := depth[depID]; !ok {
					depth[depID] = maxDepDepth + 1
					queue = append(queue, depID)
				}
			}
		}
	}

	// Find max concurrent jobs (size of largest layer, excluding wait steps)
	maxConcurrent := 1
	for _, jobs := range layers {
		count := 0
		for _, job := range jobs {
			if job.Type == StepTypeCommand {
				count++
			}
		}
		if count > maxConcurrent {
			maxConcurrent = count
		}
	}

	return maxConcurrent
}

// InsertJobsAfter inserts new jobs into the graph after a specified job
// This is used for dynamic pipeline uploads
func (g *JobGraph) InsertJobsAfter(afterJobID string, newJobs []*Job) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.Jobs[afterJobID]; !ok {
		return fmt.Errorf("job %s not found", afterJobID)
	}

	// Get the dependents of the after job
	originalDependents := g.dependents[afterJobID]

	// Add new jobs
	var newJobIDs []string
	for _, job := range newJobs {
		g.Jobs[job.ID] = job
		g.orderedJobIDs = append(g.orderedJobIDs, job.ID)
		newJobIDs = append(newJobIDs, job.ID)

		// New jobs depend on the after job
		g.dependencies[job.ID] = append(g.dependencies[job.ID], afterJobID)
	}

	// Update the after job's dependents to include new jobs
	g.dependents[afterJobID] = newJobIDs

	// Original dependents now depend on the new jobs instead
	for _, depID := range originalDependents {
		// Remove old dependency
		deps := g.dependencies[depID]
		for i, d := range deps {
			if d == afterJobID {
				deps = append(deps[:i], deps[i+1:]...)
				break
			}
		}

		// Add dependencies on new jobs
		for _, newID := range newJobIDs {
			deps = append(deps, newID)
			g.dependents[newID] = append(g.dependents[newID], depID)
		}
		g.dependencies[depID] = deps
	}

	return nil
}
