package local

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	uuid "github.com/satori/go.uuid"
)

type RunParams struct {
	Prompt      bool
	Filter      func(Job) bool
	JobTemplate Job
}

func Run(ctx context.Context, params RunParams) error {
	agentPool := newAgentPool()
	server := newApiServer(agentPool)
	steps := newStepQueue()

	// consume pipeline uploads from the server
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

	debugf("Serving API on %s", endpoint)

	err = runAgent(ctx, endpoint)
	if err != nil {
		return err
	}

	debugf("Started Agent")

	build := Build{
		ID:     uuid.NewV4().String(),
		Number: 1,
	}

	pipelineUploadJob := params.JobTemplate
	pipelineUploadJob.ID = uuid.NewV4().String()
	pipelineUploadJob.Label = ":pipeline"
	pipelineUploadJob.Build = build

	ejl, err := newEmojiLoader()
	if err != nil {
		return err
	}

	w := newBuildLogFormatter(ejl)

	pipelineUploadWriter := ioutil.Discard
	if Debug {
		pipelineUploadWriter = w
	}

	timer := time.Now()
	headerColor := color.New(color.FgWhite)

	headerColor.Printf("Starting Build 👟\n")

	err = executeJob(ctx, server, pipelineUploadWriter, pipelineUploadJob)
	if err != nil {
		return fmt.Errorf("Initial pipeline upload failed: %v", err)
	}

	// Process each step that we receive
	for step := range processSteps(ctx, steps, server) {
		if params.Prompt {
			prompt := promptui.Prompt{
				Label:     fmt.Sprintf("Run %s", ejl.Render(step.Command.Label)),
				IsConfirm: true,
				Default:   "y",
			}

			result, err := prompt.Run()
			if err != nil {
				return err
			}

			if result == "n" {
				continue
			}
		}

		if step.Command != nil {
			headerColor.Printf("Executing %s\n", ejl.Render(step.Command.Label))

			j := params.JobTemplate
			j.ID = uuid.NewV4().String()
			j.Command = strings.Join(step.Command.Commands, "\n")
			j.Label = step.Command.Label
			j.Plugins = step.Command.Plugins

			if err = executeJob(ctx, server, w, j); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("Unknown step type: %s", step)
		}
	}

	color.Blue("🎉 Build finished in %v", time.Now().Sub(timer))
	return nil
}

var subtleHeaderRegexp = regexp.MustCompile(`^~~~`)
var expandedHeaderRegexp = regexp.MustCompile(`^\+\+\+`)
var minimizedHeaderRegexp = regexp.MustCompile(`^---`)
var headerRegexp = regexp.MustCompile(`^(~~~|\+\+\+|---)`)

func newBuildLogFormatter(ejl *emojiLoader) io.Writer {
	subtle := color.New(color.FgWhite)
	expanded := color.New(color.FgHiWhite, color.Underline)
	minimized := color.New(color.FgWhite, color.Faint)

	return newLineWriter(func(line string) {
		if headerRegexp.MatchString(line) {
			line = ejl.Render(line)
		}
		if subtleHeaderRegexp.MatchString(line) {
			subtle.Printf("\n%s\n", line)
		} else if expandedHeaderRegexp.MatchString(line) {
			expanded.Printf("\n%s\n", line)
		} else if minimizedHeaderRegexp.MatchString(line) {
			minimized.Printf("\n%s\n", line)
		} else if line == "^^^ +++" {
			// skip this one
		} else {
			fmt.Println(line)
		}
	})
}

func processSteps(ctx context.Context, s *stepQueue, server *apiServer) chan stepWithEnv {
	ch := make(chan stepWithEnv)

	go func() {
		for {
			select {
			case <-ctx.Done():
				debugf("Context done, stopping processing steps")
				close(ch)
				return

			case <-time.NewTimer(time.Second).C:
				step, ok := s.Next()
				if ok {
					ch <- step
				} else if !server.HasUnfinishedJobs() {
					debugf("Steps finished and server done, stopping processing step queue")
					close(ch)
					return
				}
			}
		}
	}()

	return ch
}

func executeJob(ctx context.Context, server *apiServer, w io.Writer, j Job) error {
	exitCh := server.Execute(j, w)

	select {
	case <-ctx.Done():
		return nil
	case exitCode := <-exitCh:
		if exitCode != 0 {
			return fmt.Errorf("Job failed with code %d", exitCode)
		}
	}

	return nil
}

func runAgent(ctx context.Context, endpoint string) error {
	args := []string{"start"}
	if Debug {
		args = append(args, "--debug")
	}

	cmd := exec.CommandContext(ctx, "buildkite-agent", args...)

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

func (s *stepQueue) Len() int {
	s.Lock()
	defer s.Unlock()

	return len(s.steps)
}

func (s *stepQueue) Replace(p pipelineUpload) {
	panic("Replace not implemented")
}

func (s *stepQueue) Append(p pipelineUpload) {
	s.Lock()
	defer s.Unlock()

	for _, step := range p.Pipeline.Steps {
		s.steps = append(s.steps, stepWithEnv{
			step: step,
			env:  p.Pipeline.Env,
		})
	}
}

func (s *stepQueue) Next() (stepWithEnv, bool) {
	s.Lock()
	defer s.Unlock()

	if len(s.steps) == 0 {
		return stepWithEnv{}, false
	}

	var next stepWithEnv
	next, s.steps = s.steps[0], s.steps[1:]

	return next, true
}
