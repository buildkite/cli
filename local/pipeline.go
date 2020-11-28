package local

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
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

	if branches != nil {
		var err error
		s.Branches, err = ParseBranchPattern(branches)
		if err != nil {
			return err
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

func ParseBranchPattern(branches interface{}) ([]string, error) {
	var result []string

	switch b := branches.(type) {
	case []interface{}:
		for _, bi := range b {
			result = append(result, strings.Fields(bi.(string))...)
		}
	case string:
		result = append(result, strings.Fields(b)...)
	default:
		return nil, fmt.Errorf("Branches is unhandled type %T", branches)
	}

	return result, nil
}

func MatchBranchPattern(branch string, pattern string) bool {
	expected := true

	// Handle negation at the start
	for strings.HasPrefix(pattern, `!`) {
		expected = !expected
		pattern = strings.TrimPrefix(pattern, `!`)
	}

	// Compile a regex for the rest
	re, err := regexp.Compile(`^` + strings.Replace(pattern, `*`, `.*?`, -1) + `$`)
	if err != nil {
		log.Printf("Failed to compile regex: %v", err)
		return false
	}

	// Test it against the branch
	if re.MatchString(branch) == expected {
		return true
	}

	return false
}

func (s step) MatchBranch(branch string) bool {
	if len(s.Branches) == 0 {
		return true
	}

	// Apply a heuristic, if we have multiple negatives it's AND-ed
	// otherwise it's OR-d. Gross, but we know what the user meant.

	var negationCount int
	for _, b := range s.Branches {
		if strings.HasPrefix(b, `!`) {
			negationCount += 1
		}
	}

	if negationCount > 1 {
		// Has multiple negatives, so the patterns are AND-ed
		for _, pattern := range s.Branches {
			if !MatchBranchPattern(branch, pattern) {
				return false
			}
		}
		return true
	}

	// Has zero of one negatives, so the patterns are OR-ed
	for _, pattern := range s.Branches {
		if MatchBranchPattern(branch, pattern) {
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

type blockField struct {
	Text     string              `json:"text"`
	Select   string              `json:"select"`
	Key      string              `json:"key"`
	Hint     string              `json:"hint"`
	Required bool                `json:"required"`
	Default  string              `json:"default"`
	Options  []blockSelectOption `json:"options"`
}

type blockSelectOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type blockStep struct {
	Block  string       `json:"block"`
	Fields []blockField `json:"fields"`
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
		return fmt.Errorf("invalid command step: %v", err)
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

	// Normalize env vs environment
	s.Env = []string(intermediate.Env)
	if len(intermediate.Environment) > 0 {
		s.Env = []string(intermediate.Environment)
	}

	s.Plugins = nil

	var pluginSlice struct {
		Plugins []map[string]interface{} `json:"plugins"`
	}

	if err := json.Unmarshal(data, &pluginSlice); err == nil {
		for _, p := range pluginSlice.Plugins {
			for k, v := range p {
				switch vv := v.(type) {
				case map[string]interface{}:
					s.Plugins = append(s.Plugins, Plugin{Name: k, Params: vv})
				case nil:
					s.Plugins = append(s.Plugins, Plugin{Name: k})
				default:
					return fmt.Errorf("Unknown plugin value type %T", v)
				}
			}
		}
	}

	var pluginStringSlice struct {
		Plugins []string `json:"plugins"`
	}

	if err := json.Unmarshal(data, &pluginStringSlice); err == nil {
		for _, p := range pluginStringSlice.Plugins {
			s.Plugins = append(s.Plugins, Plugin{
				Name: p,
			})
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
	Steps []step        `json:"steps"`
	Env   envMapOrSlice `json:"env"`
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

type stringable string

func (s *stringable) UnmarshalJSON(data []byte) error {
	var target interface{}

	if err := json.Unmarshal(data, &target); err != nil {
		return err
	}

	switch target.(type) {
	case string, int, int64, bool, float32, float64:
		*s = stringable(fmt.Sprintf("%v", target))
	default:
		return fmt.Errorf("Unstringable type of %T", target)
	}

	return nil
}

type stringOrSlice []string

func (s *stringOrSlice) UnmarshalJSON(data []byte) error {
	var str stringable
	*s = []string{}

	if err := json.Unmarshal(data, &str); err == nil {
		*s = []string{string(str)}
		return nil
	}

	var strSlice []stringable

	if err := json.Unmarshal(data, &strSlice); err != nil {
		return err
	}

	for _, str := range strSlice {
		*s = append(*s, string(str))
	}

	return nil
}

type envMapOrSlice []string

func (s *envMapOrSlice) UnmarshalJSON(data []byte) error {
	var m map[string]stringable
	*s = []string{}

	if err := json.Unmarshal(data, &m); err == nil {
		for k, v := range m {
			*s = append(*s, fmt.Sprintf("%s=%s", k, v))
		}

		// maps are unordered, this makes them predictable
		sorted := sort.StringSlice(*s)
		sorted.Sort()

		return nil
	}

	var envSlice []string

	if err := json.Unmarshal(data, &envSlice); err != nil {
		return fmt.Errorf("env must be a slice or a map: %v", err)
	}

	*s = envSlice
	return nil
}
