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
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/satori/go.uuid"
)

type build struct {
	ID     string
	Number int
	URL    string
}

type job struct {
	ID               string
	Build            build
	State            string
	ProjectSlug      string
	PipelineSlug     string
	OrganizationSlug string
	ArtifactPaths    string
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
	PluginJSON       string

	exitCh chan int
	writer io.Writer
}

var (
	uuidRegexp = regexp.MustCompile(
		"([a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12})")
)

type apiServer struct {
	agents          *agentPool
	pipelineUploads chan pipelineUpload

	sync.Mutex
	jobs []*job
}

func newApiServer(agentPool *agentPool) *apiServer {
	return &apiServer{
		agents:          agentPool,
		pipelineUploads: make(chan pipelineUpload),
		jobs:            []*job{},
	}
}

func (s *apiServer) Execute(job job, w io.Writer) chan int {
	job.exitCh = make(chan int, 1)
	job.writer = w

	s.Lock()
	defer s.Unlock()
	s.jobs = append(s.jobs, &job)

	return job.exitCh
}

func (s *apiServer) AddJob(job job) {
	s.Lock()
	defer s.Unlock()
	s.jobs = append(s.jobs, &job)
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

func (s *apiServer) changeJobState(jobID string, from, to string) (*job, error) {
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

func (s *apiServer) getJobByID(jobID string) (*job, error) {
	for idx, j := range s.jobs {
		if j.ID == jobID {
			return s.jobs[idx], nil
		}
	}

	return nil, fmt.Errorf("No job with id %q found", jobID)
}

func (s *apiServer) nextJob() (*job, bool) {
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
	case `POST /jobs/:uuid/data/set`:
		a.handleMetadataSet(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	case `POST /jobs/:uuid/pipelines`:
		a.handlePipelineUpload(w, r, uuidRegexp.FindStringSubmatch(r.URL.Path)[1])
	default:
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
		PingInterval:      2,
		JobStatusInterval: 5,
		HeartbeatInterval: 5,
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
	if job.PluginJSON != "" {
		pluginJSON = job.PluginJSON
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
		`BUILDKITE_ARTIFACT_PATHS`:            job.ArtifactPaths,
		`BUILDKITE_PROJECT_PROVIDER`:          `local`,
		`BUILDKITE_ORGANIZATION_SLUG`:         job.OrganizationSlug,
		`BUILDKITE_PIPELINE_PROVIDER`:         `local`,
		`BUILDKITE_AGENT_META_DATA_QUEUE`:     `default`,
		`BUILDKITE_REBUILT_FROM_BUILD_ID`:     ``,
		`BUILDKITE_REBUILT_FROM_BUILD_NUMBER`: ``,
		`BUILDKITE_PIPELINE_DEFAULT_BRANCH`:   `master`,
		`BUILDKITE_TRIGGERED_FROM_BUILD_ID`:   ``,
		`BUILDKITE_PLUGINS`:                   pluginJSON,
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

func (a *apiServer) handleMetadataExists(w http.ResponseWriter, r *http.Request, jobID string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&struct{}{})
}

func (a *apiServer) handleMetadataSet(w http.ResponseWriter, r *http.Request, jobID string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&struct{}{})
}

func (a *apiServer) handleHeaderTimes(w http.ResponseWriter, r *http.Request, jobID string) {
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
		log.Printf("Failed to parse pipeline: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.pipelineUploads <- pur.pipelineUpload

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&struct{}{})
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	go func() {
		_ = http.Serve(listener, a)
	}()

	return fmt.Sprintf("http://%s", listener.Addr().String()), nil
}

func (a *apiServer) authenticateAgentFromHeader(h http.Header) (string, error) {
	authToken := strings.TrimPrefix(h.Get(`Authorization`), `Token `)
	return a.agents.Authenticate(authToken)
}
