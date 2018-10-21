package local

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type step struct {
	Branches []string     `json:"-"`
	Command  *commandStep `json:"-"`
	Wait     *waitStep    `json:"-"`
	Block    *blockStep   `json:"-"`
	Trigger  *triggerStep `json:"-"`
}

func (s *step) UnmarshalJSON(data []byte) error {
	var stringStep string

	// Handle steps that are just strings, e.g "wait"
	if err := json.Unmarshal(data, &stringStep); err == nil {
		switch stringStep {
		case "wait":
			s.Wait = &waitStep{}
			return nil
		default:
			return fmt.Errorf("Unknown step type %q", stringStep)
		}
	}

	var intermediate map[string]interface{}

	// Determine the type of step it is
	if err := json.Unmarshal(data, &intermediate); err != nil {
		return err
	}

	var branches = intermediate["branch"]
	if b, ok := intermediate["branches"]; ok {
		branches = b
	}

	// Handle various types of branch vs branches
	if branches != nil {
		switch b := branches.(type) {
		case []interface{}:
			for _, bi := range b {
				s.Branches = append(s.Branches, strings.Split(bi.(string), ",")...)
			}
		case string:
			s.Branches = append(s.Branches, strings.Split(b, ",")...)
		default:
			log.Printf("Branches is unhandled type %T", branches)
		}
	}

	if _, ok := intermediate["wait"]; ok {
		return json.Unmarshal(data, &s.Wait)
	}

	if _, ok := intermediate["block"]; ok {
		return json.Unmarshal(data, &s.Block)
	}

	if _, ok := intermediate["trigger"]; ok {
		return json.Unmarshal(data, &s.Trigger)
	}

	return json.Unmarshal(data, &s.Command)
}

func (s step) MatchBranch(branch string) bool {
	if len(s.Branches) == 0 {
		return true
	}
	for _, b := range s.Branches {
		if b == branch {
			return true
		}
	}
	return false
}

func (s step) Label() string {
	if s.Command != nil {
		return s.Command.Label
	} else if s.Block != nil {
		return "Block"
	} else if s.Wait != nil {
		return "Wait"
	} else if s.Trigger != nil {
		return "Trigger"
	}
	return ""
}

func (s step) String() string {
	if s.Command != nil {
		return fmt.Sprintf("{Command: %+v}", *s.Command)
	} else if s.Block != nil {
		return fmt.Sprintf("{Block: %+v}", *s.Block)
	} else if s.Wait != nil {
		return fmt.Sprintf("{Wait: %+v}", *s.Wait)
	} else if s.Trigger != nil {
		return fmt.Sprintf("{Trigger: %+v} ", *s.Trigger)
	}
	return "Unknown"
}

type blockStep struct {
	Block string `json:"block"`
}

type waitStep struct {
	Wait              string `json:"wait"`
	ContinueOnFailure bool   `json:"continue_on_failure"`
}

type triggerStep struct {
	Trigger string `json:"trigger"`
}

type commandStep struct {
	Label         string   `json:"label"`
	Commands      []string `json:"-"`
	Plugins       []Plugin `json:"-"`
	Env           []string `json:"-"`
	ArtifactPaths []string `json:"-"`
}

func (s *commandStep) UnmarshalJSON(data []byte) error {
	var intermediate struct {
		Label         string        `json:"label"`
		Name          string        `json:"name"`
		Commands      stringOrSlice `json:"commands"`
		Command       stringOrSlice `json:"command"`
		Env           envMapOrSlice `json:"env"`
		Environment   envMapOrSlice `json:"environment"`
		ArtifactPaths stringOrSlice `json:"artifact_paths"`
		Branch        stringOrSlice `json:"branch"`
		Branches      stringOrSlice `json:"branches"`
	}

	if err := json.Unmarshal(data, &intermediate); err != nil {
		return err
	}

	s.ArtifactPaths = []string(intermediate.ArtifactPaths)

	// Normalize name vs label
	s.Label = intermediate.Label
	if intermediate.Name != "" {
		s.Label = intermediate.Name
	}

	// Normalize command vs commands (note plural)
	if len(intermediate.Command) > 0 {
		s.Commands = append(s.Commands, intermediate.Command...)
	} else {
		s.Commands = append(s.Commands, intermediate.Commands...)
	}

	var pluginSlice struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}

	// Normalize env vs environment
	s.Env = []string(intermediate.Env)
	if len(intermediate.Environment) > 0 {
		s.Env = []string(intermediate.Environment)
	}

	if err := json.Unmarshal(data, &pluginSlice); err == nil {
		for _, p := range pluginSlice.Plugins {
			for k, v := range p {
				s.Plugins = append(s.Plugins, Plugin{
					Name:   k,
					Params: v.(map[string]interface{}),
				})
			}
		}
	}

	var pluginMap struct {
		Plugins map[string]interface{} `json:"plugins"`
	}

	if err := json.Unmarshal(data, &pluginMap); err == nil {
		for k, v := range pluginMap.Plugins {
			s.Plugins = append(s.Plugins, Plugin{
				Name:   k,
				Params: v,
			})
		}
	}

	return nil
}

type pipelineUpload struct {
	Pipeline pipeline `json:"pipeline"`
	Replace  bool     `json:"replace"`
}

type pipeline struct {
	Steps []step            `json:"steps"`
	Env   map[string]string `json:"env"`
}

func (p pipeline) Filter(f func(s step) bool) pipeline {
	filtered := p
	filtered.Steps = []step{}
	for _, s := range p.Steps {
		if f(s) {
			filtered.Steps = append(filtered.Steps, s)
		}
	}
	return filtered
}

type stringOrSlice []string

func (s *stringOrSlice) UnmarshalJSON(data []byte) error {
	var str string

	if err := json.Unmarshal(data, &str); err == nil {
		*s = []string{str}
		return nil
	}

	var strSlice []string

	if err := json.Unmarshal(data, &strSlice); err != nil {
		return err
	}

	*s = strSlice
	return nil
}

type envMapOrSlice []string

func (s *envMapOrSlice) UnmarshalJSON(data []byte) error {
	var m map[string]string

	if err := json.Unmarshal(data, &m); err == nil {
		for k, v := range m {
			*s = append(*s, fmt.Sprintf("%s=%s", k, v))
		}
		return nil
	}

	var envSlice []string

	if err := json.Unmarshal(data, &envSlice); err != nil {
		return err
	}

	*s = envSlice
	return nil
}
