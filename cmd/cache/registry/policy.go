package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	buildkite "github.com/buildkite/go-buildkite/v5"
	"github.com/goccy/go-yaml"
)

func readPolicy(path string, stdin io.Reader) (buildkite.CacheRegistryPolicy, error) {
	var (
		data   []byte
		err    error
		source string
	)
	if path == "-" {
		data, err = io.ReadAll(stdin)
		source = "stdin"
	} else {
		data, err = os.ReadFile(path)
		source = fmt.Sprintf("file %q", path)
	}
	if err != nil {
		return nil, fmt.Errorf("reading cache registry policy from %s: %w", source, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("cache registry policy from %s cannot be empty", source)
	}

	jsonData, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, fmt.Errorf("parsing cache registry policy from %s: %w", source, err)
	}

	var decoded any
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		return nil, fmt.Errorf("parsing cache registry policy from %s: %w", source, err)
	}
	policy, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cache registry policy from %s must be an object", source)
	}

	return buildkite.CacheRegistryPolicy(policy), nil
}
