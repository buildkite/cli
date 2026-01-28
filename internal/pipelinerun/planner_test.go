package pipelinerun

import (
	"testing"
)

func TestPlanBasicSteps(t *testing.T) {
	pipelineYAML := `
steps:
  - command: echo "hello"
    label: "Step 1"
  - command: echo "world"
    label: "Step 2"
`
	pipeline, err := ParsePipeline([]byte(pipelineYAML))
	if err != nil {
		t.Fatalf("failed to parse pipeline: %v", err)
	}

	planner := NewPlanner(pipeline)
	graph, err := planner.Plan()
	if err != nil {
		t.Fatalf("failed to plan: %v", err)
	}

	if len(graph.Jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(graph.Jobs))
	}

	jobs := graph.GetOrderedJobs()
	if jobs[0].Name != "Step 1" {
		t.Errorf("expected first job name 'Step 1', got '%s'", jobs[0].Name)
	}
	if jobs[1].Name != "Step 2" {
		t.Errorf("expected second job name 'Step 2', got '%s'", jobs[1].Name)
	}
}

func TestPlanWaitStep(t *testing.T) {
	pipelineYAML := `
steps:
  - command: echo "before"
    label: "Before"
  - wait
  - command: echo "after"
    label: "After"
`
	pipeline, err := ParsePipeline([]byte(pipelineYAML))
	if err != nil {
		t.Fatalf("failed to parse pipeline: %v", err)
	}

	planner := NewPlanner(pipeline)
	graph, err := planner.Plan()
	if err != nil {
		t.Fatalf("failed to plan: %v", err)
	}

	if len(graph.Jobs) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(graph.Jobs))
	}

	// Find the "After" job and check dependencies
	var afterJob *Job
	for _, job := range graph.Jobs {
		if job.Name == "After" {
			afterJob = job
			break
		}
	}

	if afterJob == nil {
		t.Fatal("could not find 'After' job")
	}

	deps := graph.GetDependencies(afterJob.ID)
	if len(deps) != 1 {
		t.Errorf("expected 'After' to depend on 1 job (wait), got %d", len(deps))
	}
}

func TestPlanParallelism(t *testing.T) {
	pipelineYAML := `
steps:
  - command: echo "parallel"
    label: "Parallel Job"
    parallelism: 3
`
	pipeline, err := ParsePipeline([]byte(pipelineYAML))
	if err != nil {
		t.Fatalf("failed to parse pipeline: %v", err)
	}

	planner := NewPlanner(pipeline)
	graph, err := planner.Plan()
	if err != nil {
		t.Fatalf("failed to plan: %v", err)
	}

	if len(graph.Jobs) != 3 {
		t.Errorf("expected 3 jobs for parallelism:3, got %d", len(graph.Jobs))
	}

	// Check parallel job indices
	for _, job := range graph.Jobs {
		if job.ParallelJobCount != 3 {
			t.Errorf("expected ParallelJobCount=3, got %d", job.ParallelJobCount)
		}
		if job.ParallelJob < 0 || job.ParallelJob >= 3 {
			t.Errorf("ParallelJob out of range: %d", job.ParallelJob)
		}

		// Check environment variables
		if job.Env["BUILDKITE_PARALLEL_JOB_COUNT"] != "3" {
			t.Errorf("expected BUILDKITE_PARALLEL_JOB_COUNT=3, got %s", job.Env["BUILDKITE_PARALLEL_JOB_COUNT"])
		}
	}
}

func TestPlanMatrix(t *testing.T) {
	pipelineYAML := `
steps:
  - command: echo "build {{matrix}}"
    label: "Build ({{matrix}})"
    matrix:
      - linux
      - darwin
      - windows
`
	pipeline, err := ParsePipeline([]byte(pipelineYAML))
	if err != nil {
		t.Fatalf("failed to parse pipeline: %v", err)
	}

	planner := NewPlanner(pipeline)
	graph, err := planner.Plan()
	if err != nil {
		t.Fatalf("failed to plan: %v", err)
	}

	if len(graph.Jobs) != 3 {
		t.Errorf("expected 3 jobs for matrix with 3 values, got %d", len(graph.Jobs))
	}

	// Check that matrix values are set
	foundLinux := false
	foundDarwin := false
	foundWindows := false

	for _, job := range graph.Jobs {
		switch job.MatrixValues["matrix"] {
		case "linux":
			foundLinux = true
		case "darwin":
			foundDarwin = true
		case "windows":
			foundWindows = true
		}
	}

	if !foundLinux || !foundDarwin || !foundWindows {
		t.Errorf("missing matrix combinations: linux=%v darwin=%v windows=%v", foundLinux, foundDarwin, foundWindows)
	}
}

func TestPlanMultiDimensionMatrix(t *testing.T) {
	pipelineYAML := `
steps:
  - command: echo "build"
    label: "Build"
    matrix:
      os:
        - linux
        - darwin
      arch:
        - amd64
        - arm64
`
	pipeline, err := ParsePipeline([]byte(pipelineYAML))
	if err != nil {
		t.Fatalf("failed to parse pipeline: %v", err)
	}

	planner := NewPlanner(pipeline)
	graph, err := planner.Plan()
	if err != nil {
		t.Fatalf("failed to plan: %v", err)
	}

	// 2 OS * 2 arch = 4 combinations
	if len(graph.Jobs) != 4 {
		t.Errorf("expected 4 jobs for 2x2 matrix, got %d", len(graph.Jobs))
	}
}

func TestPlanDependsOn(t *testing.T) {
	pipelineYAML := `
steps:
  - command: echo "build"
    label: "Build"
    key: build
  - command: echo "test"
    label: "Test"
    key: test
  - command: echo "deploy"
    label: "Deploy"
    depends_on:
      - build
      - test
`
	pipeline, err := ParsePipeline([]byte(pipelineYAML))
	if err != nil {
		t.Fatalf("failed to parse pipeline: %v", err)
	}

	planner := NewPlanner(pipeline)
	graph, err := planner.Plan()
	if err != nil {
		t.Fatalf("failed to plan: %v", err)
	}

	// Find the deploy job
	var deployJob *Job
	for _, job := range graph.Jobs {
		if job.Name == "Deploy" {
			deployJob = job
			break
		}
	}

	if deployJob == nil {
		t.Fatal("could not find 'Deploy' job")
	}

	deps := graph.GetDependencies(deployJob.ID)
	if len(deps) != 2 {
		t.Errorf("expected 'Deploy' to depend on 2 jobs, got %d", len(deps))
	}
}

func TestPlanGroup(t *testing.T) {
	pipelineYAML := `
steps:
  - group: "Tests"
    steps:
      - command: echo "unit"
        label: "Unit Tests"
      - command: echo "integration"
        label: "Integration Tests"
`
	pipeline, err := ParsePipeline([]byte(pipelineYAML))
	if err != nil {
		t.Fatalf("failed to parse pipeline: %v", err)
	}

	planner := NewPlanner(pipeline)
	graph, err := planner.Plan()
	if err != nil {
		t.Fatalf("failed to plan: %v", err)
	}

	if len(graph.Jobs) != 2 {
		t.Errorf("expected 2 jobs from group, got %d", len(graph.Jobs))
	}
}

func TestCalculateMaxConcurrency(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected int
	}{
		{
			name: "single job",
			yaml: `
steps:
  - command: echo "hello"
`,
			expected: 1,
		},
		{
			name: "two parallel jobs",
			yaml: `
steps:
  - command: echo "a"
  - command: echo "b"
`,
			expected: 2,
		},
		{
			name: "jobs with wait",
			yaml: `
steps:
  - command: echo "a"
  - command: echo "b"
  - wait
  - command: echo "c"
  - command: echo "d"
  - command: echo "e"
`,
			expected: 3,
		},
		{
			name: "parallelism",
			yaml: `
steps:
  - command: echo "parallel"
    parallelism: 5
`,
			expected: 5,
		},
		{
			name: "matrix",
			yaml: `
steps:
  - command: echo "build"
    matrix:
      - a
      - b
      - c
      - d
`,
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := ParsePipeline([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("failed to parse pipeline: %v", err)
			}

			planner := NewPlanner(pipeline)
			graph, err := planner.Plan()
			if err != nil {
				t.Fatalf("failed to plan: %v", err)
			}

			maxConcurrency := CalculateMaxConcurrency(graph)
			if maxConcurrency != tt.expected {
				t.Errorf("expected max concurrency %d, got %d", tt.expected, maxConcurrency)
			}
		})
	}
}

func TestPlanBlockStep(t *testing.T) {
	pipelineYAML := `
steps:
  - command: echo "build"
    label: "Build"
  - block: "Deploy to production?"
    prompt: "Are you sure?"
  - command: echo "deploy"
    label: "Deploy"
`
	pipeline, err := ParsePipeline([]byte(pipelineYAML))
	if err != nil {
		t.Fatalf("failed to parse pipeline: %v", err)
	}

	planner := NewPlanner(pipeline)
	graph, err := planner.Plan()
	if err != nil {
		t.Fatalf("failed to plan: %v", err)
	}

	if len(graph.Jobs) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(graph.Jobs))
	}

	// Find block job
	var blockJob *Job
	for _, job := range graph.Jobs {
		if job.Type == StepTypeBlock {
			blockJob = job
			break
		}
	}

	if blockJob == nil {
		t.Fatal("could not find block job")
	}

	if blockJob.Prompt != "Are you sure?" {
		t.Errorf("expected prompt 'Are you sure?', got '%s'", blockJob.Prompt)
	}
}

func TestPlanEnvExpansion(t *testing.T) {
	pipelineYAML := `
env:
  GREETING: hello

steps:
  - command: echo "$GREETING"
    label: "Say $GREETING"
    env:
      TARGET: world
`
	pipeline, err := ParsePipeline([]byte(pipelineYAML))
	if err != nil {
		t.Fatalf("failed to parse pipeline: %v", err)
	}

	planner := NewPlanner(pipeline)
	planner.WithEnv(map[string]string{"EXTRA": "value"})

	graph, err := planner.Plan()
	if err != nil {
		t.Fatalf("failed to plan: %v", err)
	}

	job := graph.GetOrderedJobs()[0]

	if job.Env["GREETING"] != "hello" {
		t.Errorf("expected GREETING=hello, got %s", job.Env["GREETING"])
	}
	if job.Env["TARGET"] != "world" {
		t.Errorf("expected TARGET=world, got %s", job.Env["TARGET"])
	}
}
