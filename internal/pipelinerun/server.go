package pipelinerun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Server is a mock Buildkite API server for local pipeline execution
type Server struct {
	scheduler *Scheduler
	graph     *JobGraph
	port      int

	// HTTP server
	server   *http.Server
	listener net.Listener

	// Agent tracking
	mu         sync.Mutex
	agents     map[string]*AgentInfo
	jobAssigns map[string]string // jobID -> agentID

	// Pipeline upload handler
	onPipelineUpload func(jobID string, pipeline *Pipeline) error

	// Logging
	debug bool
}

// AgentInfo tracks a registered agent
type AgentInfo struct {
	ID           string
	Name         string
	Hostname     string
	RegisteredAt time.Time
	LastPingAt   time.Time
}

// NewServer creates a new mock Buildkite server
func NewServer(scheduler *Scheduler, graph *JobGraph, port int) *Server {
	return &Server{
		scheduler:  scheduler,
		graph:      graph,
		port:       port,
		agents:     make(map[string]*AgentInfo),
		jobAssigns: make(map[string]string),
	}
}

// SetDebug enables debug logging
func (s *Server) SetDebug(debug bool) {
	s.debug = debug
}

// SetPipelineUploadHandler sets the handler for pipeline uploads
func (s *Server) SetPipelineUploadHandler(handler func(jobID string, pipeline *Pipeline) error) {
	s.onPipelineUpload = handler
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Agent API endpoints
	mux.HandleFunc("/v3/register", s.handleRegister)
	mux.HandleFunc("/v3/connect", s.handleConnect)
	mux.HandleFunc("/v3/disconnect", s.handleDisconnect)
	mux.HandleFunc("/v3/ping", s.handlePing)
	mux.HandleFunc("/v3/heartbeat", s.handleHeartbeat)

	// Job endpoints
	mux.HandleFunc("/v3/builds/", s.handleBuilds)
	mux.HandleFunc("/v3/jobs/", s.handleJobs)

	// Pipeline upload
	mux.HandleFunc("/v3/pipelines/", s.handlePipelines)

	// Artifact endpoints (stub)
	mux.HandleFunc("/v3/artifacts/", s.handleArtifacts)

	// Meta endpoint
	mux.HandleFunc("/v3/meta", s.handleMeta)

	var err error
	s.listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", s.port, err)
	}

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	return nil
}

// Port returns the actual port the server is listening on
func (s *Server) Port() int {
	if s.listener == nil {
		return s.port
	}
	return s.listener.Addr().(*net.TCPAddr).Port
}

// Stop stops the HTTP server
func (s *Server) Stop() error {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

// URL returns the base URL of the server
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.Port())
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name     string `json:"name"`
		Hostname string `json:"hostname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	agentID := fmt.Sprintf("agent-%d", len(s.agents)+1)
	s.agents[agentID] = &AgentInfo{
		ID:           agentID,
		Name:         req.Name,
		Hostname:     req.Hostname,
		RegisteredAt: time.Now(),
		LastPingAt:   time.Now(),
	}
	s.mu.Unlock()

	if s.debug {
		log.Printf("[server] Agent registered: %s (%s)", agentID, req.Name)
	}

	resp := map[string]any{
		"id":           agentID,
		"name":         req.Name,
		"access_token": "local-token",
		"endpoint":     s.URL(),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Agent connect - just acknowledge
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"connected": true})
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return a job if one is available
	select {
	case job := <-s.scheduler.JobQueue():
		if job != nil {
			s.mu.Lock()
			// Extract agent ID from authorization header or request
			agentID := r.Header.Get("X-Agent-ID")
			if agentID == "" {
				agentID = "agent-1"
			}
			s.jobAssigns[job.ID] = agentID
			s.mu.Unlock()

			s.scheduler.HandleJobStarted(job.ID)

			if s.debug {
				log.Printf("[server] Assigned job %s to %s: %s", job.ID, agentID, job.Name)
			}

			_ = json.NewEncoder(w).Encode(s.jobToResponse(job))
			return
		}
	default:
	}

	// No job available
	_ = json.NewEncoder(w).Encode(map[string]any{"job": nil})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"heartbeat": "ok"})
}

func (s *Server) handleBuilds(w http.ResponseWriter, r *http.Request) {
	// Handle build-related endpoints
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":     "local-build-1",
		"number": 1,
		"state":  "running",
	})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v3/jobs/")
	parts := strings.Split(path, "/")

	if len(parts) < 1 {
		http.Error(w, "Job ID required", http.StatusBadRequest)
		return
	}

	jobID := parts[0]

	// Handle different job endpoints
	if len(parts) >= 2 {
		switch parts[1] {
		case "start":
			s.handleJobStart(w, r, jobID)
		case "finish":
			s.handleJobFinish(w, r, jobID)
		case "log":
			s.handleJobLog(w, r, jobID)
		case "chunks":
			s.handleJobChunks(w, r, jobID)
		default:
			http.Error(w, "Unknown endpoint", http.StatusNotFound)
		}
		return
	}

	// Get job details
	job, ok := s.graph.GetJob(jobID)
	if !ok {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	_ = json.NewEncoder(w).Encode(s.jobToResponse(job))
}

func (s *Server) handleJobStart(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.scheduler.HandleJobStarted(jobID)

	if s.debug {
		log.Printf("[server] Job started: %s", jobID)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleJobFinish(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ExitStatus     int    `json:"exit_status"`
		Signal         string `json:"signal"`
		SignalReason   string `json:"signal_reason"`
		ChunksFinished bool   `json:"chunks_finished"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	state := JobStatePassed
	if req.ExitStatus != 0 {
		state = JobStateFailed
	}

	// Check for soft fail
	job, ok := s.graph.GetJob(jobID)
	if ok && state == JobStateFailed {
		if job.SoftFail {
			state = JobStatePassed
		} else if len(job.SoftFailCode) > 0 {
			for _, code := range job.SoftFailCode {
				if code == req.ExitStatus {
					state = JobStatePassed
					break
				}
			}
		}
	}

	s.scheduler.HandleJobFinished(jobID, state, req.ExitStatus)

	// Mark dependents as failed if this job failed
	if state == JobStateFailed {
		s.scheduler.MarkDependentsFailed(jobID)
	}

	if s.debug {
		log.Printf("[server] Job finished: %s (exit=%d, state=%s)", jobID, req.ExitStatus, state)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleJobLog(w http.ResponseWriter, r *http.Request, jobID string) {
	// Accept log chunks
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleJobChunks(w http.ResponseWriter, r *http.Request, jobID string) {
	// Accept log chunks
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handlePipelines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse pipeline upload
	var req struct {
		Pipeline string `json:"pipeline"`
		Replace  bool   `json:"replace"`
	}

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		// YAML content
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.Pipeline = string(data)
	}

	// Parse the uploaded pipeline
	pipeline, err := ParsePipeline([]byte(req.Pipeline))
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse pipeline: %v", err), http.StatusBadRequest)
		return
	}

	// Get the job ID from the query or path
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		// Try to extract from path
		path := strings.TrimPrefix(r.URL.Path, "/v3/pipelines/")
		parts := strings.Split(path, "/")
		if len(parts) > 0 {
			jobID = parts[0]
		}
	}

	if s.onPipelineUpload != nil {
		if err := s.onPipelineUpload(jobID, pipeline); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if s.debug {
		log.Printf("[server] Pipeline uploaded from job %s (%d steps)", jobID, len(pipeline.Steps))
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"uploaded": true})
}

func (s *Server) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	// Stub for artifact uploads
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"uploaded": true})
		return
	}

	// Artifact list
	_ = json.NewEncoder(w).Encode([]any{})
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"version": "local",
	})
}

// jobToResponse converts a Job to the API response format
func (s *Server) jobToResponse(job *Job) map[string]any {
	// Build the command script
	command := job.Command
	if command == "" && len(job.Commands) > 0 {
		command = strings.Join(job.Commands, "\n")
	}

	// Build environment
	env := make(map[string]string)
	for k, v := range job.Env {
		env[k] = v
	}

	// Add parallelism env vars
	if job.ParallelJobCount > 0 {
		env["BUILDKITE_PARALLEL_JOB"] = fmt.Sprintf("%d", job.ParallelJob)
		env["BUILDKITE_PARALLEL_JOB_COUNT"] = fmt.Sprintf("%d", job.ParallelJobCount)
	}

	// Add matrix env vars
	for k, v := range job.MatrixValues {
		env[fmt.Sprintf("BUILDKITE_MATRIX_%s", strings.ToUpper(k))] = v
	}

	// Add standard Buildkite env vars
	env["BUILDKITE"] = "true"
	env["BUILDKITE_BUILD_ID"] = "local-build-1"
	env["BUILDKITE_BUILD_NUMBER"] = "1"
	env["BUILDKITE_JOB_ID"] = job.ID
	env["BUILDKITE_STEP_KEY"] = job.Key
	env["BUILDKITE_LABEL"] = job.Name
	env["BUILDKITE_COMMAND"] = command

	resp := map[string]any{
		"id":                   job.ID,
		"type":                 "script",
		"name":                 job.Name,
		"state":                string(job.State),
		"command":              command,
		"env":                  env,
		"agent_query_rules":    job.AgentQueryRules,
		"artifact_paths":       strings.Join(job.ArtifactPaths, ";"),
		"timeout_in_minutes":   job.TimeoutInMinutes,
		"step_key":             job.Key,
		"parallel_group_index": job.ParallelJob,
		"parallel_group_total": job.ParallelJobCount,
	}

	// Add plugins if present
	if len(job.Plugins) > 0 {
		var plugins []map[string]any
		for _, p := range job.Plugins {
			plugins = append(plugins, map[string]any{
				p.Name: p.Config,
			})
		}
		pluginsJSON, _ := json.Marshal(plugins)
		resp["plugins"] = string(pluginsJSON)
	}

	return map[string]any{"job": resp}
}
