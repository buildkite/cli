package local

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
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
	Env         []string
	Dir         string
	Command     string
	Prompt      bool
	StepFilter  *regexp.Regexp
	DryRun      bool
	JobTemplate Job
}

func Run(ctx context.Context, params RunParams) error {
	agentPool := newAgentPool()
	server := newApiServer(agentPool)
	steps := newStepQueue()

	// Consume pipeline uploads from the server and apply any filters
	go func() {
		for p := range server.pipelineUploads {
			filtered := p.Pipeline.Filter(func(s step) bool {

				// Apply the step filter to the label
				if params.StepFilter != nil {
					return params.StepFilter.MatchString(s.Label())
				}

				// Apply any branch filters
				if !s.MatchBranch(params.JobTemplate.Branch) {
					return false
				}

				return true
			})
			if p.Replace {
				steps.Replace(filtered)
			} else {
				steps.Append(filtered)
			}
		}
	}()

	endpoint, err := server.ListenAndServe()
	if err != nil {
		return err
	}

	debugf("Serving API on %s", endpoint)

	agent := Agent{
		Dir:      params.Dir,
		Env:      params.Env,
		Endpoint: endpoint,
	}
	if err := agent.Run(ctx); err != nil {
		return err
	}
	defer func() {
		// this ensures the agent always stops gracefully
		_ = agent.Stop()
	}()

	headerColor := color.New(color.FgWhite, color.Bold)
	headerColor.Printf(">>> Starting local agent 🤖\n")

	build := Build{
		ID:     uuid.NewV4().String(),
		Number: 1,
	}

	initialJob := params.JobTemplate
	initialJob.ID = uuid.NewV4().String()
	initialJob.Label = ":pipeline:"
	initialJob.Build = build
	initialJob.Command = params.Command

	ejl, err := newEmojiLoader()
	if err != nil {
		return err
	}

	w := newBuildLogFormatter(ejl)
	timer := time.Now()

	headerColor.Printf(">>> Starting build 👟\n")
	headerColor.Printf(">>> Executing initial command: %s\n", params.Command)

	var initialJobWriter io.Writer
	initialJobWriter = w

	if !Debug {
		initialJobWriter = ioutil.Discard
	}

	err = executeJob(ctx, server, initialJobWriter, initialJob)
	if err != nil {
		return fmt.Errorf("Initial command failed: %v", err)
	}

	// Process each step that we receive
	for step := range processSteps(ctx, steps, server) {
		debugf("Processing step %s", step)

		if step.Wait != nil {
			continue
		}

		if params.Prompt {
			fmt.Println()
			prompt := promptui.Prompt{
				Label:     fmt.Sprintf("Run %s", ejl.Render(step.Label())),
				IsConfirm: true,
				Default:   "y",
			}

			result, err := prompt.Run()
			if err != nil {
				return err
			}

			fmt.Println()

			if result == "n" {
				continue
			}
		}

		if step.Command != nil {
			dryRunNote := ""
			if params.DryRun {
				dryRunNote = " (dry-run)"
			}
			headerColor.Printf(">>> Executing command step %s%s\n",
				ejl.Render(step.Command.Label),
				dryRunNote)

			if !params.DryRun {
				// load the step into a job
				j := params.JobTemplate
				j.ID = uuid.NewV4().String()
				j.Build = build
				j.Command = strings.Join(step.Command.Commands, "\n")
				j.Label = step.Command.Label
				j.Plugins = step.Command.Plugins
				j.Env = step.Command.Env
				j.ArtifactPaths = step.Command.ArtifactPaths

				if err = executeJob(ctx, server, w, j); err != nil {
					headerColor.Printf(">>> 🚨 Command failed in %v\n", time.Now().Sub(timer))
					return err
				} else {
					headerColor.Printf(">>> Command succeeded in %v\n", time.Now().Sub(timer))
				}
			}
		} else {
			return fmt.Errorf("Unknown step type: %s", step)
		}
	}

	color.Green(">>> Build finished in %v\n", time.Now().Sub(timer))
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

	// add some trailing whitespace
	defer func() {
		fmt.Fprintf(w, "\n")
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case exitCode := <-exitCh:
		if exitCode != 0 {
			return fmt.Errorf("Job failed with code %d", exitCode)
		}
	}

	return nil
}

type Agent struct {
	Dir      string
	Env      []string
	Endpoint string

	sync.Mutex
	stopFunc func() error
	stopping bool
	stopped  bool
}

func (a *Agent) Run(ctx context.Context) error {
	bootstrap, err := createAgentBootstrap(a.Dir)
	if err != nil {
		return err
	}

	args := []string{"start"}
	if Debug {
		args = append(args, "--debug")
	}

	cmd := exec.Command("buildkite-agent", args...)
	if Debug {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = ioutil.Discard
		cmd.Stderr = ioutil.Discard
	}

	cmd.Env = append(a.Env,
		`HOME=`+os.Getenv(`HOME`),
		`PATH=`+os.Getenv(`PATH`),
		`BUILDKITE_AGENT_ENDPOINT=`+a.Endpoint,
		`BUILDKITE_AGENT_TOKEN=llamas`,
		`BUILDKITE_AGENT_NAME=local`,
		`BUILDKITE_BOOTSTRAP_SCRIPT_PATH=`+bootstrap.Name(),
	)

	// this function is called at the end of Run()
	// it kills the agent
	a.stopFunc = func() error {
		defer os.Remove(bootstrap.Name())
		_ = cmd.Process.Signal(os.Interrupt)
		return cmd.Wait()
	}

	// if the context is cancelled (from ctrl-c)
	// we need to lock so that the above stopFunc doesn't
	// send a signal, as the ctrl-c was sent to the process group
	// which would lead to a double signal
	go func() {
		<-ctx.Done()
		a.Lock()
		defer a.Unlock()
		_ = cmd.Wait()
	}()

	return cmd.Start()
}

func (a *Agent) Stop() error {
	a.Lock()
	defer a.Unlock()
	if a.stopping || a.stopped {
		log.Printf("Already stopped or stopping")
		return nil
	}
	a.stopping = true
	err := a.stopFunc()
	a.stopped = true
	return err
}

func createAgentBootstrap(checkoutPath string) (*os.File, error) {
	tmpFile, err := ioutil.TempFile(os.TempDir(), "bootstrap-")
	if err != nil {
		return nil, err
	}

	debugf("Creating bootrap script at %s", tmpFile.Name())

	text := []byte(fmt.Sprintf(`#!/bin/sh
	export BUILDKITE_BUILD_CHECKOUT_PATH=%s
	export BUILDKITE_BOOTSTRAP_PHASES=plugin,command
	buildkite-agent bootstrap`, checkoutPath))

	if _, err = tmpFile.Write(text); err != nil {
		return nil, err
	}

	if err = os.Chmod(tmpFile.Name(), 0700); err != nil {
		return nil, err
	}

	return tmpFile, nil
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

func (s *stepQueue) Replace(p pipeline) {
	panic("Replace not implemented")
}

func (s *stepQueue) Append(p pipeline) {
	s.Lock()
	defer s.Unlock()

	for _, step := range p.Steps {
		s.steps = append(s.steps, stepWithEnv{
			step: step,
			env:  p.Env,
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
