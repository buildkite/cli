package local

import (
	"compress/gzip"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar"
	"github.com/fatih/color"
	homedir "github.com/mitchellh/go-homedir"
	uuid "github.com/satori/go.uuid"
)

type Build struct {
	ID     string
	Number int
	URL    string
}

type Job struct {
	ID               string
	Build            Build
	State            string
	ProjectSlug      string
	PipelineSlug     string
	OrganizationSlug string
	ArtifactPaths    []string
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
	Plugins          []Plugin
	Env              []string
	Artifacts        []Artifact
}

type Artifact struct {
	ID                string `json:"-"`
	Path              string `json:"path"`
	AbsolutePath      string `json:"absolute_path"`
	GlobPath          string `json:"glob_path"`
	FileSize          int64  `json:"file_size"`
	Sha1Sum           string `json:"sha1sum"`
	URL               string `json:"url,omitempty"`
	UploadDestination string `json:"upload_destination,omitempty"`

	uploaded  bool
	localPath string
}

type jobEnvelope struct {
	Job
	exitCh chan int
	writer io.Writer
}

var (
	uuidRegexp     = regexp.MustCompile("([a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12})")
	filenameRegexp = regexp.MustCompile(`filename="(.*?)"`)
)

type apiServer struct {
	agents          *agentPool
	pipelineUploads chan pipelineUpload
	listener        net.Listener
	listenPort      int

	sync.Mutex
	jobs      []*jobEnvelope
	artifacts *orderedMap
	metadata  *orderedMap
}

func newApiServer(agentPool *agentPool, listenPort int) *apiServer {
	return &apiServer{
		agents:          agentPool,
		pipelineUploads: make(chan pipelineUpload),
		jobs:            []*jobEnvelope{},
		artifacts:       newOrderedMap(),
		metadata:        newOrderedMap(),
		listenPort:      listenPort,
	}
}

func (s *apiServer) Execute(job Job, w io.Writer) chan int {
	js := &jobEnvelope{
		Job:    job,
		exitCh: make(chan int, 1),
		writer: w,
	}

	s.Lock()
	defer s.Unlock()
	s.jobs = append(s.jobs, js)

	return js.exitCh
}

func (s *apiServer) HasUnfinishedJobs() bool {
	s.Lock()
	defer s.Unlock()
	for _, j := range s.jobs {
		if j.State != "finished" {
			return true
		}
	}
	return false
}

func (s *apiServer) changeJobState(jobID string, from, to string) (*jobEnvelope, error) {
	j, err := s.getJobByID(jobID)
	if err != nil {
		return nil, err
	}

	if j.State != from {
		return nil, fmt.Errorf("Job state is %q, expected %q", j.State, from)
	}
	j.State = to
	return j, nil
}

func (s *apiServer) getJobByID(jobID string) (*jobEnvelope, error) {
	for idx, j := range s.jobs {
		if j.ID == jobID {
			return s.jobs[idx], nil
		}
	}

	return nil, fmt.Errorf("No job with id %q found", jobID)
}

func (s *apiServer) nextJob() (*jobEnvelope, bool) {
	for _, j := range s.jobs {
		if j.State == "" {
			j.State = "scheduled"
			return j, true
		}
	}
	return nil, false
}

func (a *apiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	debugf("[http] %s %s %s", r.Method, r.RequestURI, r.RemoteAddr)

	requestPath := uuidRegexp.ReplaceAllString(r.Method+" "+r.URL.Path, ":uuid")

	switch requestPath {
	case `POST /register`:
		a.handleRegister(w, r)
	case `POST /connect`:
		a.handleConnect(w, r)
	case `POST /disconnect`:
		a.handleDisconnect(w, r)
	case `POST /heartbeat`:
		a.handleHeartbeat(w, r)
	case `GET /ping`:
		a.handlePing(w, r)
	case `GET /jobs/:uuid`:
		a.handleGetJob(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `PUT /jobs/:uuid/start`:
		a.handleStartJob(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `PUT /jobs/:uuid/accept`:
		a.handleAcceptJob(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `PUT /jobs/:uuid/finish`:
		a.handleFinishJob(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `POST /jobs/:uuid/chunks`:
		a.handleLogChunks(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `POST /jobs/:uuid/header_times`:
		a.handleHeaderTimes(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `POST /jobs/:uuid/data/exists`:
		a.handleMetadataExists(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `POST /jobs/:uuid/data/keys`:
		a.handleMetadataKeys(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `POST /jobs/:uuid/data/set`:
		a.handleMetadataSet(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `POST /jobs/:uuid/data/get`:
		a.handleMetadataGet(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `POST /jobs/:uuid/pipelines`:
		a.handlePipelineUpload(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `POST /jobs/:uuid/annotations`:
		a.handleAnnotations(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `POST /jobs/:uuid/artifacts`:
		a.handleArtifactsUploadInstructions(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `POST /jobs/:uuid/artifacts/upload`:
		a.handleArtifactsUpload(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `PUT /jobs/:uuid/artifacts`:
		a.handleArtifactsUpdate(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `GET /builds/:uuid/artifacts/search`:
		a.handleArtifactsSearch(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `GET /artifacts/:uuid`:
		a.handleArtifactDownload(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	default:
		color.Red(">>> 😓 An unknown agent API endpoint was requested (%s).\n"+
			"File an issue at https://github.com/buildkite/cli/issues and we'll see what we can do!\n",
			requestPath,
		)
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	}
}

func (a *apiServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	u := uuid.NewV4()

	agent := agent{
		ID:          u.String(),
		AccessToken: fmt.Sprintf("%x", sha1.Sum(u.Bytes())),
		Name:        "local",
	}

	a.agents.Register(agent)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		ID                string      `json:"id,omitempty"`
		Name              string      `json:"name,omitempty"`
		Endpoint          interface{} `json:"endpoint,omitempty"`
		AccessToken       string      `json:"access_token,omitempty"`
		PingInterval      int         `json:"ping_interval,omitempty"`
		JobStatusInterval int         `json:"job_status_interval,omitempty"`
		HeartbeatInterval int         `json:"heartbeat_interval,omitempty"`
		MetaData          []string    `json:"meta_data,omitempty"`
	}{
		ID:                agent.ID,
		Name:              agent.Name,
		AccessToken:       agent.AccessToken,
		PingInterval:      1,
		JobStatusInterval: 1,
		HeartbeatInterval: 1,
		MetaData:          []string{"queue=default"},
	})
}

func (a *apiServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	agentID, err := a.authenticateAgentFromHeader(r.Header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	a.agents.Connect(agentID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		ID              string `json:"id"`
		ConnectionState string `json:"connection_state"`
	}{
		agentID, "connected",
	})
}

func (a *apiServer) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	agentID, err := a.authenticateAgentFromHeader(r.Header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	a.agents.Disconnect(agentID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		ConnectionState string `json:"connection_state"`
	}{
		"disconnected",
	})
}

func (a *apiServer) handlePing(w http.ResponseWriter, r *http.Request) {
	_, err := a.authenticateAgentFromHeader(r.Header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	a.Lock()
	defer a.Unlock()

	j, ok := a.nextJob()
	if !ok {
		json.NewEncoder(w).Encode(struct{}{})
		return
	}

	json.NewEncoder(w).Encode(map[string]map[string]string{
		"job": map[string]string{
			"id": j.ID,
		},
	})
}

func (a *apiServer) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	_, err := a.authenticateAgentFromHeader(r.Header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}

func (a *apiServer) handleAcceptJob(w http.ResponseWriter, r *http.Request, jobID string) {
	agentID, err := a.authenticateAgentFromHeader(r.Header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	agent, err := a.agents.Get(agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	a.Lock()
	defer a.Unlock()

	job, err := a.changeJobState(jobID, "scheduled", "accepted")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pluginJSON := "[]"
	if len(job.Plugins) > 0 {
		var err error
		pluginJSON, err = marshalPlugins(job.Plugins)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	env := map[string]string{
		`CI`:                                  `true`,
		`BUILDKITE`:                           `true`,
		`BUILDKITE_TAG`:                       job.Tag,
		`BUILDKITE_REPO`:                      job.Repository,
		`BUILDKITE_LABEL`:                     job.Label,
		`BUILDKITE_BRANCH`:                    job.Branch,
		`BUILDKITE_COMMIT`:                    job.Commit,
		`BUILDKITE_JOB_ID`:                    job.ID,
		`BUILDKITE_SOURCE`:                    `local`,
		`BUILDKITE_COMMAND`:                   job.Command,
		`BUILDKITE_MESSAGE`:                   job.Message,
		`BUILDKITE_TIMEOUT`:                   fmt.Sprintf("%d", job.Timeout),
		`BUILDKITE_AGENT_ID`:                  agentID,
		`BUILDKITE_BUILD_ID`:                  job.Build.ID,
		`BUILDKITE_BUILD_URL`:                 job.Build.URL,
		`BUILDKITE_AGENT_NAME`:                agent.Name,
		`BUILDKITE_RETRY_COUNT`:               fmt.Sprintf("%d", job.RetryCount),
		`BUILDKITE_SCRIPT_PATH`:               ``,
		`BUILDKITE_BUILD_NUMBER`:              fmt.Sprintf("%d", job.Build.Number),
		`BUILDKITE_PROJECT_SLUG`:              job.ProjectSlug,
		`BUILDKITE_PULL_REQUEST`:              `false`,
		`BUILDKITE_PULL_REQUEST_REPO`:         ``,
		`BUILDKITE_PULL_REQUEST_BASE_BRANCH`:  ``,
		`BUILDKITE_BUILD_CREATOR`:             job.CreatorName,
		`BUILDKITE_BUILD_CREATOR_EMAIL`:       job.CreatorEmail,
		`BUILDKITE_PIPELINE_SLUG`:             job.PipelineSlug,
		`BUILDKITE_ARTIFACT_PATHS`:            strings.Join(job.ArtifactPaths, ","),
		`BUILDKITE_PROJECT_PROVIDER`:          `local`,
		`BUILDKITE_ORGANIZATION_SLUG`:         job.OrganizationSlug,
		`BUILDKITE_PIPELINE_PROVIDER`:         `local`,
		`BUILDKITE_AGENT_META_DATA_QUEUE`:     `default`,
		`BUILDKITE_REBUILT_FROM_BUILD_ID`:     ``,
		`BUILDKITE_REBUILT_FROM_BUILD_NUMBER`: ``,
		`BUILDKITE_PIPELINE_DEFAULT_BRANCH`:   getenv("BUILDKITE_CLI_PIPELINE_DEFAULT_BRANCH", "main"),
		`BUILDKITE_TRIGGERED_FROM_BUILD_ID`:   ``,
		`BUILDKITE_PLUGINS`:                   pluginJSON,
	}

	jobEnv := map[string]string{}

	// append step environment, letting later ones
	// overwrite earlier ones
	for _, envLine := range job.Env {
		envFrags := strings.SplitN(envLine, "=", 2)
		jobEnv[envFrags[0]] = envFrags[1]
	}

	// append stepEnv to the job environment but
	// don't overwrite any already values
	for k, v := range jobEnv {
		if _, exists := env[k]; !exists {
			env[k] = v
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		ID                 string            `json:"id"`
		State              string            `json:"state"`
		Env                map[string]string `json:"env"`
		Endpoint           string            `json:"endpoint"`
		ChunksMaxSizeBytes int               `json:"chunks_max_size_bytes"`
	}{
		ID:                 job.ID,
		State:              "accepted",
		Endpoint:           fmt.Sprintf("http://%s", r.Host),
		ChunksMaxSizeBytes: 102400,
		Env:                env,
	})
}

func (a *apiServer) handleGetJob(w http.ResponseWriter, r *http.Request, jobID string) {
	a.Lock()
	defer a.Unlock()

	job, err := a.getJobByID(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		State string `json:"state"`
	}{
		State: job.State,
	})
}

func (a *apiServer) handleStartJob(w http.ResponseWriter, r *http.Request, jobID string) {
	_, err := a.authenticateAgentFromHeader(r.Header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	a.Lock()
	defer a.Unlock()

	job, err := a.changeJobState(jobID, "accepted", "started")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		State string `json:"state"`
	}{
		State: job.State,
	})
}

func (a *apiServer) handleFinishJob(w http.ResponseWriter, r *http.Request, jobID string) {
	_, err := a.authenticateAgentFromHeader(r.Header)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var rr struct {
		ExitStatus string `json:"exit_status"`
	}
	if err := readRequestInto(r, &rr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.Lock()
	defer a.Unlock()

	job, err := a.changeJobState(jobID, "started", "finished")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	exitCodeInt, err := strconv.Atoi(rr.ExitStatus)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	job.exitCh <- exitCodeInt

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		State string `json:"state"`
	}{
		State: job.State,
	})
}

func (a *apiServer) handleMetadataKeys(w http.ResponseWriter, r *http.Request, jobID string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a.metadata.Keys())
}

func (a *apiServer) handleMetadataExists(w http.ResponseWriter, r *http.Request, jobID string) {
	w.Header().Set("Content-Type", "application/json")

	var parsed struct {
		Key string `json:"key"`
	}
	err := readRequestInto(r, &parsed)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !a.metadata.Contains(parsed.Key) {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(&struct {
		Exists bool `json:"exists"`
	}{
		Exists: true,
	})
}

func (a *apiServer) SetMetadata(k, v string) {
	a.metadata.Store(k, v)
}

func (a *apiServer) handleMetadataSet(w http.ResponseWriter, r *http.Request, jobID string) {
	w.Header().Set("Content-Type", "application/json")

	var parsed struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	err := readRequestInto(r, &parsed)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	a.metadata.Store(parsed.Key, parsed.Value)
	json.NewEncoder(w).Encode(&struct{}{})
}

func (a *apiServer) handleMetadataGet(w http.ResponseWriter, r *http.Request, jobID string) {
	w.Header().Set("Content-Type", "application/json")

	var parsed struct {
		Key string `json:"key"`
	}
	err := readRequestInto(r, &parsed)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	val, ok := a.metadata.Load(parsed.Key)
	if !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(&struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}{
		parsed.Key, val.(string),
	})
}

func (a *apiServer) handleHeaderTimes(w http.ResponseWriter, r *http.Request, jobID string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&struct{}{})
}

func (a *apiServer) handleAnnotations(w http.ResponseWriter, r *http.Request, jobID string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&struct{}{})
}

func (a *apiServer) handleLogChunks(w http.ResponseWriter, r *http.Request, jobID string) {
	gr, err := gzip.NewReader(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer gr.Close()

	b, err := ioutil.ReadAll(gr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	a.Lock()
	defer a.Unlock()

	job, err := a.getJobByID(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	job.writer.Write(b)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		ID string `json:"id"`
	}{
		ID: uuid.NewV4().String(),
	})
}

func (a *apiServer) handlePipelineUpload(w http.ResponseWriter, r *http.Request, jobID string) {
	var pur struct {
		UUID string `json:"uuid"`
		pipelineUpload
	}

	if err := readRequestInto(r, &pur); err != nil {
		log.Printf("Failed to parse pipeline upload: %v", err)
		http.Error(w, err.Error(), 422)
		return
	}

	a.pipelineUploads <- pur.pipelineUpload

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&struct{}{})
}

type uploadAction struct {
	URL       string `json:"url,omitempty"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	FileInput string `json:"file_input"`
}

type uploadInstructions struct {
	Data   map[string]string `json:"data"`
	Action uploadAction      `json:"action"`
}

func (a *apiServer) handleArtifactsUploadInstructions(w http.ResponseWriter, r *http.Request, jobID string) {
	var artifactBatch struct {
		ID                string     `json:"id"`
		Artifacts         []Artifact `json:"artifacts"`
		UploadDestination string     `json:"upload_destination"`
	}

	if err := readRequestInto(r, &artifactBatch); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.Lock()
	defer a.Unlock()

	job, err := a.getJobByID(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var artifactIDs []string

	for idx := range artifactBatch.Artifacts {
		artifactID := uuid.NewV4().String()
		artifactIDs = append(artifactIDs, artifactID)
		artifactBatch.Artifacts[idx].ID = artifactID
		artifactBatch.Artifacts[idx].URL = fmt.Sprintf("http://%s/artifacts/%s", a.listener.Addr().String(), artifactID)
	}

	job.Artifacts = artifactBatch.Artifacts

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&struct {
		ID                 string             `json:"id"`
		ArtifactIDs        []string           `json:"artifact_ids"`
		UploadInstructions uploadInstructions `json:"upload_instructions"`
	}{
		uuid.NewV4().String(),
		artifactIDs,
		uploadInstructions{
			Action: uploadAction{
				fmt.Sprintf("http://%s", a.listener.Addr().String()),
				"POST",
				fmt.Sprintf("/jobs/%s/artifacts/upload", jobID),
				"file",
			},
		},
	})
}

func (a *apiServer) handleArtifactsUpload(w http.ResponseWriter, r *http.Request, jobID string) {
	file, header, err := r.FormFile("file")
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	defer file.Close()

	contentDisposition := header.Header.Get("Content-Disposition")
	filename := filenameRegexp.FindStringSubmatch(contentDisposition)[1]

	cacheDir, err := artifactCachePath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	path := filepath.Join(cacheDir, jobID, filename)
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out, err := os.Create(path)
	if err != nil {
		log.Printf("Unable to create %s for writing. Check your write access privilege", path)
		return
	}

	defer out.Close()

	// write the content from POST to the file
	_, err = io.Copy(out, file)
	if err != nil {
		fmt.Fprintln(w, err)
	}

	a.Lock()
	defer a.Unlock()

	job, err := a.getJobByID(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for idx := range job.Artifacts {
		if filename == job.Artifacts[idx].Path {
			job.Artifacts[idx].uploaded = true
			job.Artifacts[idx].localPath = path
		}
	}

	fmt.Fprintf(w, "File uploaded successfully: %s", filename)
}

func (a *apiServer) handleArtifactsUpdate(w http.ResponseWriter, r *http.Request, jobID string) {
	var rr struct {
		Artifacts []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"artifacts"`
	}

	if err := readRequestInto(r, &rr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.Lock()
	defer a.Unlock()

	job, err := a.getJobByID(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, updatedArtifact := range rr.Artifacts {
		for _, jobArtifact := range job.Artifacts {
			if jobArtifact.ID == updatedArtifact.ID && updatedArtifact.State == `finished` {
				a.artifacts.Store(jobArtifact.ID, jobArtifact)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&struct{}{})
}

func (a *apiServer) handleArtifactsSearch(w http.ResponseWriter, r *http.Request, buildID string) {
	query := r.URL.Query().Get("query")

	a.Lock()
	defer a.Unlock()

	var artifacts []Artifact

	for _, key := range a.artifacts.Keys() {
		value, ok := a.artifacts.Load(key)
		if !ok {
			continue
		}

		artifact := value.(Artifact)
		match, err := doublestar.PathMatch(query, artifact.Path)
		if err != nil {
			log.Printf("Err: %v", err)
		}

		if match {
			artifacts = append(artifacts, artifact)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(artifacts)
}

func (a *apiServer) handleArtifactDownload(w http.ResponseWriter, r *http.Request, artifactID string) {
	a.Lock()
	defer a.Unlock()

	artifact, ok := a.artifacts.Load(artifactID)
	if !ok {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	}

	f, err := os.Open(artifact.(Artifact).localPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	_, err = io.Copy(w, f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func readRequestInto(r *http.Request, into interface{}) error {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	defer r.Body.Close()

	return json.Unmarshal(body, &into)
}

func (a *apiServer) ListenAndServe() (string, error) {
	var err error
	a.listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", a.listenPort))
	if err != nil {
		return "", err
	}

	go func() {
		_ = http.Serve(a.listener, a)
	}()

	return fmt.Sprintf("http://%s", a.listener.Addr().String()), nil
}

func (a *apiServer) authenticateAgentFromHeader(h http.Header) (string, error) {
	authToken := strings.TrimPrefix(h.Get(`Authorization`), `Token `)
	return a.agents.Authenticate(authToken)
}

func artifactCachePath() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".buildkite", "local", "artifacts"), nil
}

type orderedMapValue struct {
	key   string
	value interface{}
}

type orderedMap struct {
	idx  map[string]int
	vals []orderedMapValue
	mu   sync.RWMutex
}

func newOrderedMap() *orderedMap {
	return &orderedMap{
		idx:  map[string]int{},
		vals: []orderedMapValue{},
	}
}

func (o *orderedMap) Contains(key string) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	_, ok := o.idx[key]
	return ok
}

func (o *orderedMap) Keys() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	keys := []string{}
	for _, val := range o.vals {
		keys = append(keys, val.key)
	}
	return keys
}

func (o *orderedMap) Load(key string) (interface{}, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	idx, ok := o.idx[key]
	if !ok {
		return nil, false
	}
	return o.vals[idx].value, true
}

func (o *orderedMap) Store(key string, value interface{}) {
	o.mu.Lock()
	defer o.mu.Unlock()
	idx, ok := o.idx[key]
	if !ok {
		o.vals = append(o.vals, orderedMapValue{key, value})
		o.idx[key] = len(o.vals) - 1
	} else {
		o.vals[idx] = orderedMapValue{key, value}
	}
}

func getenv(key, def string) string {
	result := os.Getenv(key)
	if len(result) == 0 {
		return def
	}
	return result
}
