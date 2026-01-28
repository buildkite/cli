# Pipeline Run Implementation Plan

## Implement bk pipeline run command

type: epic

Run Buildkite pipelines locally using a local buildkite-agent. Supports parallelism, matrix builds, wait steps, and dynamic pipeline uploads.

## Core types for pipeline run

type: task
parent: Implement bk pipeline run command

Create `internal/pipelinerun/types.go` with:
- Job struct (ID, command, env, dependencies, parallel info)
- JobGraph struct (jobs map, dependency edges)
- JobState enum (pending, running, passed, failed)
- StepType enum (command, wait, block, trigger, group)

## Pipeline YAML loader

type: task
parent: Implement bk pipeline run command
deps: Core types for pipeline run

Create `internal/pipelinerun/loader.go`:
- Parse pipeline.yaml/pipeline.yml
- Handle parallelism, matrix, concurrency, concurrency_group
- Expand environment variables
- Return structured pipeline representation

## Job graph planner

type: task
parent: Implement bk pipeline run command
deps: Pipeline YAML loader

Create `internal/pipelinerun/planner.go`:
- Convert pipeline steps to JobGraph DAG
- Expand parallelism N into N distinct jobs
- Expand matrix combinations
- Handle depends_on and implicit wait step dependencies
- CalculateMaxConcurrency() to find max parallel jobs

## Job scheduler

type: task
parent: Implement bk pipeline run command
deps: Job graph planner

Create `internal/pipelinerun/scheduler.go`:
- Event-driven job dispatch loop
- Track job states and transitions
- Respect dependencies before dispatching
- Handle concurrency limits
- Support dynamic pipeline uploads

## Mock Buildkite API server

type: task
parent: Implement bk pipeline run command
deps: Job scheduler

Create `internal/pipelinerun/server.go`:
- HTTP server mimicking Buildkite agent API
- Handle agent registration
- Distribute jobs to agents
- Handle job state updates
- Handle pipeline uploads
- Inject BUILDKITE_PARALLEL_JOB env vars

## Agent wrapper

type: task
parent: Implement bk pipeline run command
deps: Mock Buildkite API server

Create `internal/pipelinerun/agent.go`:
- Find/validate buildkite-agent binary
- Start agent with --spawn=N for concurrency
- Configure agent to talk to mock server
- Handle agent lifecycle

## Run orchestrator

type: task
parent: Implement bk pipeline run command
deps: Agent wrapper

Create `internal/pipelinerun/run.go`:
- Load pipeline and build graph
- Calculate or accept --spawn concurrency
- Start mock server and agent
- Process job queue until completion
- Report final status

## CLI command integration

type: task
parent: Implement bk pipeline run command
deps: Run orchestrator

Create `cmd/pipeline/run.go`:
- Wire up bk pipeline run command
- Flags: --file, --spawn, --env, --dry-run, --agent-bin, --port
- Integrate with existing pipeline command structure
- Update main.go PipelineCmd

## Unit tests for planner

type: task
parent: Implement bk pipeline run command
deps: Job graph planner

Create `internal/pipelinerun/planner_test.go`:
- Test basic step expansion
- Test parallelism expansion
- Test matrix expansion
- Test wait step handling
- Test CalculateMaxConcurrency
