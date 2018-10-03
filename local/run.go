package local

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	uuid "github.com/satori/go.uuid"
)

type RunParams struct {
	Branch           string
	Commit           string
	Command          string
	Message          string
	Label            string
	Tag              string
	Repository       string
	OrganizationSlug string
	PipelineSlug     string
}

func Run(ctx context.Context, params RunParams) error {
	agentPool := newAgentPool()
	scheduler := newScheduler()

	server := &apiServer{
		agents:          agentPool,
		scheduler:       scheduler,
		pipelineUploads: make(chan pipelineUpload),
	}

	steps := newStepQueue()
	go func() {
		for p := range server.pipelineUploads {
			if p.Replace {
				steps.Replace(p)
			} else {
				steps.Append(p)
			}
		}
	}()

	endpoint, err := server.ListenAndServe()
	if err != nil {
		return err
	}

	log.Printf("Serving API on %s", endpoint)

	err = runAgent(ctx, endpoint)
	if err != nil {
		return err
	}

	log.Printf("Started Agent")

	build := build{
		ID:     uuid.NewV4().String(),
		Number: 1,
	}

	scheduler.Schedule(job{
		ID:               uuid.NewV4().String(),
		Build:            build,
		Command:          params.Command,
		Label:            params.Label,
		Commit:           params.Commit,
		Branch:           params.Branch,
		Tag:              params.Tag,
		Message:          params.Message,
		Repository:       params.Repository,
		OrganizationSlug: params.OrganizationSlug,
		PipelineSlug:     params.PipelineSlug,
	})

	log.Printf("Scheduled initial step")

	for {
		select {
		case <-ctx.Done():
			log.Printf("Shutting down runner")
			return nil
		case <-time.NewTimer(time.Second).C:
			if step := steps.Next(); step != nil {
				switch {
				case step.Command != nil:
					log.Printf("Processing a command step")
					plugins, err := marshalPlugins(step.Command.Plugins)
					if err != nil {
						log.Printf("Error marshaling plugins")
						continue
					}

					scheduler.Schedule(job{
						ID:               uuid.NewV4().String(),
						Build:            build,
						Command:          strings.Join(step.Command.Commands, "\n"),
						Label:            step.Command.Label,
						Commit:           params.Commit,
						Branch:           params.Branch,
						Tag:              params.Tag,
						Message:          params.Message,
						Repository:       params.Repository,
						OrganizationSlug: params.OrganizationSlug,
						PipelineSlug:     params.PipelineSlug,
						PluginJSON:       plugins,
					})
				default:
					log.Printf("Unknown step type")
					spew.Dump(step)
				}
			}
		}
	}
}

func runAgent(ctx context.Context, endpoint string) error {
	cmd := exec.CommandContext(ctx, "buildkite-agent", "start")

	cmd.Stdout = ioutil.Discard
	cmd.Stderr = ioutil.Discard
	cmd.Env = append(os.Environ(),
		`BUILDKITE_AGENT_ENDPOINT=`+endpoint,
		`BUILDKITE_AGENT_TOKEN=llamas`,
		`BUILDKITE_AGENT_NAME=local`,
	)

	return cmd.Start()
}

type stepWithEnv struct {
	step
	env map[string]string
}

type stepQueue struct {
	sync.Mutex
	steps []stepWithEnv
}

func newStepQueue() *stepQueue {
	return &stepQueue{
		steps: []stepWithEnv{},
	}
}

func (s *stepQueue) Replace(p pipelineUpload) {
	panic("Replace not implemented")
}

func (s *stepQueue) Append(p pipelineUpload) {
	s.Lock()
	defer s.Unlock()

	for _, step := range p.Pipeline.Steps {
		log.Printf("Appending step")
		s.steps = append(s.steps, stepWithEnv{
			step: step,
			env:  p.Pipeline.Env,
		})
	}
}

func (s *stepQueue) Next() *stepWithEnv {
	s.Lock()
	defer s.Unlock()

	if len(s.steps) == 0 {
		return nil
	}
	var next stepWithEnv
	next, s.steps = s.steps[0], s.steps[1:]
	return &next
}
