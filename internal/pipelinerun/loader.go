package pipelinerun

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadPipeline loads a pipeline from a file path
func LoadPipeline(path string) (*Pipeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading pipeline file: %w", err)
	}

	return ParsePipeline(data)
}

// FindPipelineFile searches for a pipeline file in common locations
func FindPipelineFile(dir string) (string, error) {
	candidates := []string{
		"pipeline.yml",
		"pipeline.yaml",
		".buildkite/pipeline.yml",
		".buildkite/pipeline.yaml",
		"buildkite.yml",
		"buildkite.yaml",
	}

	for _, candidate := range candidates {
		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no pipeline file found in %s", dir)
}

// ParsePipeline parses pipeline YAML data
func ParsePipeline(data []byte) (*Pipeline, error) {
	var raw rawPipeline
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing pipeline YAML: %w", err)
	}

	return convertRawPipeline(&raw)
}

// rawPipeline represents the raw YAML structure
type rawPipeline struct {
	Env    map[string]string `yaml:"env"`
	Agents rawAgents         `yaml:"agents"`
	Steps  []yaml.Node       `yaml:"steps"`
}

// rawAgents can be a map or a list of query rules
type rawAgents struct {
	QueryRules []string
}

func (a *rawAgents) UnmarshalYAML(node *yaml.Node) error {
	// Try as map first (queue: hosted, etc.)
	if node.Kind == yaml.MappingNode {
		var m map[string]string
		if err := node.Decode(&m); err != nil {
			return err
		}
		for k, v := range m {
			a.QueryRules = append(a.QueryRules, fmt.Sprintf("%s=%s", k, v))
		}
		return nil
	}

	// Try as list of strings
	if node.Kind == yaml.SequenceNode {
		return node.Decode(&a.QueryRules)
	}

	return fmt.Errorf("agents must be a map or list")
}

func convertRawPipeline(raw *rawPipeline) (*Pipeline, error) {
	p := &Pipeline{
		Env:             raw.Env,
		AgentQueryRules: raw.Agents.QueryRules,
	}

	for i, node := range raw.Steps {
		step, err := parseStep(&node)
		if err != nil {
			return nil, fmt.Errorf("parsing step %d: %w", i, err)
		}
		p.Steps = append(p.Steps, *step)
	}

	return p, nil
}

func parseStep(node *yaml.Node) (*Step, error) {
	// Check if it's a simple wait string
	if node.Kind == yaml.ScalarNode {
		if node.Value == "wait" || node.Value == "waiter" {
			return &Step{Type: StepTypeWait}, nil
		}
		if node.Value == "block" {
			return &Step{Type: StepTypeBlock}, nil
		}
		return nil, fmt.Errorf("unknown scalar step: %s", node.Value)
	}

	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("step must be a mapping or string, got %v", node.Kind)
	}

	// Decode into a generic map first to determine type
	var raw map[string]yaml.Node
	if err := node.Decode(&raw); err != nil {
		return nil, err
	}

	step := &Step{}

	// Determine step type based on keys present
	if _, ok := raw["wait"]; ok {
		step.Type = StepTypeWait
	} else if _, ok := raw["waiter"]; ok {
		step.Type = StepTypeWait
	} else if _, ok := raw["block"]; ok {
		step.Type = StepTypeBlock
	} else if _, ok := raw["input"]; ok {
		step.Type = StepTypeInput
	} else if _, ok := raw["trigger"]; ok {
		step.Type = StepTypeTrigger
	} else if _, ok := raw["group"]; ok {
		step.Type = StepTypeGroup
	} else if _, ok := raw["command"]; ok {
		step.Type = StepTypeCommand
	} else if _, ok := raw["commands"]; ok {
		step.Type = StepTypeCommand
	} else {
		// Default to command if has label/name
		if _, ok := raw["label"]; ok {
			step.Type = StepTypeCommand
		} else if _, ok := raw["name"]; ok {
			step.Type = StepTypeCommand
		} else {
			return nil, fmt.Errorf("cannot determine step type from keys: %v", mapKeys(raw))
		}
	}

	// Parse common fields
	if n, ok := raw["key"]; ok {
		step.Key = n.Value
	}
	if n, ok := raw["label"]; ok {
		step.Label = n.Value
	}
	if n, ok := raw["name"]; ok {
		step.Name = n.Value
	}
	if n, ok := raw["if"]; ok {
		step.If = n.Value
	}

	// Parse depends_on
	if n, ok := raw["depends_on"]; ok {
		deps, err := parseDependsOn(&n)
		if err != nil {
			return nil, fmt.Errorf("parsing depends_on: %w", err)
		}
		step.DependsOn = deps
	}

	if n, ok := raw["allow_dependency_failure"]; ok {
		step.AllowDependencyFailure = n.Value == "true"
	}

	// Parse type-specific fields
	switch step.Type {
	case StepTypeCommand:
		if err := parseCommandStep(step, raw); err != nil {
			return nil, err
		}
	case StepTypeWait:
		if n, ok := raw["continue_on_failure"]; ok {
			step.ContinueOnFailure = n.Value == "true"
		}
	case StepTypeBlock, StepTypeInput:
		if err := parseBlockStep(step, raw); err != nil {
			return nil, err
		}
	case StepTypeTrigger:
		if err := parseTriggerStep(step, raw); err != nil {
			return nil, err
		}
	case StepTypeGroup:
		if err := parseGroupStep(step, raw); err != nil {
			return nil, err
		}
	}

	return step, nil
}

func parseCommandStep(step *Step, raw map[string]yaml.Node) error {
	// Command(s)
	if n, ok := raw["command"]; ok {
		switch n.Kind {
		case yaml.ScalarNode:
			step.Command = n.Value
		case yaml.SequenceNode:
			var cmds []string
			if err := n.Decode(&cmds); err != nil {
				return fmt.Errorf("parsing command list: %w", err)
			}
			step.Commands = cmds
		}
	}
	if n, ok := raw["commands"]; ok {
		var cmds []string
		if err := n.Decode(&cmds); err != nil {
			return fmt.Errorf("parsing commands: %w", err)
		}
		step.Commands = cmds
	}

	// Environment
	if n, ok := raw["env"]; ok {
		var env map[string]string
		if err := n.Decode(&env); err != nil {
			return fmt.Errorf("parsing env: %w", err)
		}
		step.Env = env
	}

	// Plugins
	if n, ok := raw["plugins"]; ok {
		plugins, err := parsePlugins(&n)
		if err != nil {
			return fmt.Errorf("parsing plugins: %w", err)
		}
		step.Plugins = plugins
	}

	// Parallelism
	if n, ok := raw["parallelism"]; ok {
		var p int
		if err := n.Decode(&p); err != nil {
			return fmt.Errorf("parsing parallelism: %w", err)
		}
		step.Parallelism = p
	}

	// Matrix
	if n, ok := raw["matrix"]; ok {
		matrix, err := parseMatrix(&n)
		if err != nil {
			return fmt.Errorf("parsing matrix: %w", err)
		}
		step.Matrix = matrix
	}

	// Concurrency
	if n, ok := raw["concurrency"]; ok {
		var c int
		if err := n.Decode(&c); err != nil {
			return fmt.Errorf("parsing concurrency: %w", err)
		}
		step.Concurrency = c
	}
	if n, ok := raw["concurrency_group"]; ok {
		step.ConcurrencyGroup = n.Value
	}

	// Artifacts
	if n, ok := raw["artifact_paths"]; ok {
		var paths []string
		if n.Kind == yaml.ScalarNode {
			paths = []string{n.Value}
		} else {
			if err := n.Decode(&paths); err != nil {
				return fmt.Errorf("parsing artifact_paths: %w", err)
			}
		}
		step.ArtifactPaths = paths
	}

	// Agents
	if n, ok := raw["agents"]; ok {
		var agents rawAgents
		if err := n.Decode(&agents); err != nil {
			return fmt.Errorf("parsing agents: %w", err)
		}
		step.AgentQueryRules = agents.QueryRules
	}

	// Timeout
	if n, ok := raw["timeout_in_minutes"]; ok {
		var t int
		if err := n.Decode(&t); err != nil {
			return fmt.Errorf("parsing timeout_in_minutes: %w", err)
		}
		step.TimeoutInMinutes = t
	}

	// Soft fail
	if n, ok := raw["soft_fail"]; ok {
		var sf any
		if err := n.Decode(&sf); err != nil {
			return fmt.Errorf("parsing soft_fail: %w", err)
		}
		step.SoftFail = sf
	}

	// Retry
	if n, ok := raw["retry"]; ok {
		retry, err := parseRetry(&n)
		if err != nil {
			return fmt.Errorf("parsing retry: %w", err)
		}
		step.Retry = retry
	}

	return nil
}

func parseBlockStep(step *Step, raw map[string]yaml.Node) error {
	// Get the block/input label
	if n, ok := raw["block"]; ok && n.Kind == yaml.ScalarNode && n.Value != "" {
		step.Label = n.Value
	}
	if n, ok := raw["input"]; ok && n.Kind == yaml.ScalarNode && n.Value != "" {
		step.Label = n.Value
	}

	if n, ok := raw["prompt"]; ok {
		step.Prompt = n.Value
	}
	if n, ok := raw["blocked_state"]; ok {
		step.BlockedState = n.Value
	}
	if n, ok := raw["branches"]; ok {
		step.Branches = n.Value
	}
	if n, ok := raw["allowed_teams"]; ok {
		var teams []string
		if err := n.Decode(&teams); err != nil {
			return fmt.Errorf("parsing allowed_teams: %w", err)
		}
		step.AllowedTeams = teams
	}

	if n, ok := raw["fields"]; ok {
		fields, err := parseInputFields(&n)
		if err != nil {
			return fmt.Errorf("parsing fields: %w", err)
		}
		step.Fields = fields
	}

	return nil
}

func parseTriggerStep(step *Step, raw map[string]yaml.Node) error {
	if n, ok := raw["trigger"]; ok {
		step.Trigger = n.Value
	}
	if n, ok := raw["async"]; ok {
		step.Async = n.Value == "true"
	}
	if n, ok := raw["build"]; ok {
		var build map[string]any
		if err := n.Decode(&build); err != nil {
			return fmt.Errorf("parsing build: %w", err)
		}
		step.Build = build
	}
	return nil
}

func parseGroupStep(step *Step, raw map[string]yaml.Node) error {
	if n, ok := raw["group"]; ok {
		step.Group = n.Value
	}

	if n, ok := raw["steps"]; ok {
		if n.Kind != yaml.SequenceNode {
			return fmt.Errorf("group steps must be a sequence")
		}
		for i := 0; i < len(n.Content); i++ {
			childStep, err := parseStep(n.Content[i])
			if err != nil {
				return fmt.Errorf("parsing group step %d: %w", i, err)
			}
			step.Steps = append(step.Steps, *childStep)
		}
	}

	return nil
}

func parseDependsOn(node *yaml.Node) ([]string, error) {
	// Can be a single string or list of strings/objects
	switch node.Kind {
	case yaml.ScalarNode:
		return []string{node.Value}, nil
	case yaml.SequenceNode:
		var deps []string
		for _, n := range node.Content {
			switch n.Kind {
			case yaml.ScalarNode:
				deps = append(deps, n.Value)
			case yaml.MappingNode:
				// {step: "key", allow_failure: true}
				var m map[string]yaml.Node
				if err := n.Decode(&m); err != nil {
					return nil, err
				}
				if stepNode, ok := m["step"]; ok {
					deps = append(deps, stepNode.Value)
				}
			}
		}
		return deps, nil
	}

	return nil, fmt.Errorf("depends_on must be a string or list")
}

func parsePlugins(node *yaml.Node) ([]Plugin, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("plugins must be a sequence")
	}

	var plugins []Plugin
	for _, n := range node.Content {
		if n.Kind != yaml.MappingNode {
			continue
		}

		// Each plugin is a map with one key (the plugin name)
		var m map[string]yaml.Node
		if err := n.Decode(&m); err != nil {
			return nil, err
		}

		for name, configNode := range m {
			var config map[string]any
			if configNode.Kind == yaml.MappingNode {
				if err := configNode.Decode(&config); err != nil {
					return nil, fmt.Errorf("parsing plugin %s config: %w", name, err)
				}
			}
			plugins = append(plugins, Plugin{Name: name, Config: config})
		}
	}

	return plugins, nil
}

func parseMatrix(node *yaml.Node) (*MatrixConfig, error) {
	matrix := &MatrixConfig{
		Setup: make(map[string][]string),
	}

	// Matrix can be a simple list or a complex object
	if node.Kind == yaml.SequenceNode {
		// Simple list: matrix: ["a", "b", "c"]
		var values []string
		if err := node.Decode(&values); err != nil {
			return nil, err
		}
		matrix.Setup["matrix"] = values
		return matrix, nil
	}

	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("matrix must be a list or mapping")
	}

	var raw map[string]yaml.Node
	if err := node.Decode(&raw); err != nil {
		return nil, err
	}

	// Check for setup key (complex matrix)
	if setupNode, ok := raw["setup"]; ok {
		if err := parseMatrixSetup(&setupNode, matrix); err != nil {
			return nil, err
		}
	} else {
		// Simple mapping: matrix: {os: [linux, darwin], arch: [amd64, arm64]}
		for key, valNode := range raw {
			if key == "adjustments" {
				continue
			}
			var values []string
			if err := valNode.Decode(&values); err != nil {
				return nil, fmt.Errorf("parsing matrix key %s: %w", key, err)
			}
			matrix.Setup[key] = values
		}
	}

	// Parse adjustments
	if adjNode, ok := raw["adjustments"]; ok {
		var adjustments []MatrixAdjustment
		if err := adjNode.Decode(&adjustments); err != nil {
			return nil, fmt.Errorf("parsing matrix adjustments: %w", err)
		}
		matrix.Adjustments = adjustments
	}

	return matrix, nil
}

func parseMatrixSetup(node *yaml.Node, matrix *MatrixConfig) error {
	if node.Kind == yaml.SequenceNode {
		// List of mappings
		for _, n := range node.Content {
			var m map[string]string
			if err := n.Decode(&m); err != nil {
				return err
			}
			for k, v := range m {
				matrix.Setup[k] = appendUnique(matrix.Setup[k], v)
			}
		}
		return nil
	}

	if node.Kind == yaml.MappingNode {
		var m map[string][]string
		if err := node.Decode(&m); err != nil {
			return err
		}
		matrix.Setup = m
		return nil
	}

	return fmt.Errorf("matrix setup must be a list or mapping")
}

func parseInputFields(node *yaml.Node) ([]InputField, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("fields must be a sequence")
	}

	var fields []InputField
	for _, n := range node.Content {
		var raw map[string]yaml.Node
		if err := n.Decode(&raw); err != nil {
			return nil, err
		}

		field := InputField{}
		if v, ok := raw["key"]; ok {
			field.Key = v.Value
		}
		if v, ok := raw["text"]; ok {
			field.Text = v.Value
		}
		if v, ok := raw["hint"]; ok {
			field.Hint = v.Value
		}
		if v, ok := raw["required"]; ok {
			field.Required = v.Value == "true"
		}
		if v, ok := raw["default"]; ok {
			field.Default = v.Value
		}
		if v, ok := raw["select"]; ok {
			field.Select = v.Value
		}
		if v, ok := raw["multiple"]; ok {
			field.Multiple = v.Value == "true"
		}

		if optNode, ok := raw["options"]; ok {
			var opts []SelectOption
			if err := optNode.Decode(&opts); err != nil {
				return nil, fmt.Errorf("parsing field options: %w", err)
			}
			field.Options = opts
		}

		fields = append(fields, field)
	}

	return fields, nil
}

func parseRetry(node *yaml.Node) (*RetryConfig, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("retry must be a mapping")
	}

	var raw map[string]yaml.Node
	if err := node.Decode(&raw); err != nil {
		return nil, err
	}

	retry := &RetryConfig{}

	if autoNode, ok := raw["automatic"]; ok {
		// Can be bool, single object, or list
		switch autoNode.Kind {
		case yaml.ScalarNode:
			if autoNode.Value == "true" {
				retry.Automatic = []AutomaticRetry{{Limit: 2}}
			}
		case yaml.SequenceNode:
			var rules []AutomaticRetry
			if err := autoNode.Decode(&rules); err != nil {
				return nil, fmt.Errorf("parsing automatic retry: %w", err)
			}
			retry.Automatic = rules
		case yaml.MappingNode:
			var rule AutomaticRetry
			if err := autoNode.Decode(&rule); err != nil {
				return nil, fmt.Errorf("parsing automatic retry: %w", err)
			}
			retry.Automatic = []AutomaticRetry{rule}
		}
	}

	if manualNode, ok := raw["manual"]; ok {
		var manual ManualRetry
		if manualNode.Kind == yaml.ScalarNode {
			manual.Allowed = manualNode.Value == "true"
		} else {
			if err := manualNode.Decode(&manual); err != nil {
				return nil, fmt.Errorf("parsing manual retry: %w", err)
			}
		}
		retry.Manual = &manual
	}

	return retry, nil
}

func mapKeys(m map[string]yaml.Node) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func appendUnique(slice []string, value string) []string {
	for _, v := range slice {
		if v == value {
			return slice
		}
	}
	return append(slice, value)
}

// ExpandEnvVars expands environment variables in a string using the provided env map
func ExpandEnvVars(s string, env map[string]string) string {
	return os.Expand(s, func(key string) string {
		if v, ok := env[key]; ok {
			return v
		}
		return os.Getenv(key)
	})
}

// ExpandMatrixVars expands {{matrix.key}} placeholders in a string
func ExpandMatrixVars(s string, matrixValues map[string]string) string {
	result := s
	for key, value := range matrixValues {
		placeholder := fmt.Sprintf("{{matrix.%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
		// Also handle simple {{matrix}} for single-value matrices
		if key == "matrix" {
			result = strings.ReplaceAll(result, "{{matrix}}", value)
		}
	}
	return result
}
