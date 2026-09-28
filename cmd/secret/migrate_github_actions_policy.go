package secret

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxSecretsPolicyBytes = 64 << 10

var (
	organizationSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	uuidPattern             = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	buildkiteSecretPattern  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,254}$`)
	githubSecretPattern     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func validateMigrationSecretName(name string) error {
	if !buildkiteSecretPattern.MatchString(name) {
		return fmt.Errorf("secret name %q must start with a letter, contain only letters, numbers, or underscores, and be at most 255 characters", name)
	}
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "bk") || strings.HasPrefix(lower, "buildkite") {
		return fmt.Errorf("secret name %q uses a prefix reserved by Buildkite", name)
	}
	return nil
}

func readSecretsPolicy(policyPath string) (string, error) {
	file, err := os.Open(policyPath)
	if err != nil {
		return "", fmt.Errorf("read policy: %w", err)
	}
	contents, readErr := io.ReadAll(io.LimitReader(file, maxSecretsPolicyBytes+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return "", fmt.Errorf("read policy: %w", err)
	}
	if len(contents) > maxSecretsPolicyBytes {
		return "", fmt.Errorf("policy exceeds %d bytes", maxSecretsPolicyBytes)
	}
	return validateSecretsPolicy(string(contents))
}

func validateSecretsPolicy(contents string) (string, error) {
	policy := strings.TrimSpace(strings.ReplaceAll(contents, "\r\n", "\n"))
	if policy == "" {
		return "", errors.New("policy must not be empty")
	}
	if strings.Contains(policy, "${{") {
		return "", errors.New("policy must not contain GitHub expression syntax")
	}

	var document yaml.Node
	if err := yaml.Unmarshal([]byte(policy), &document); err != nil {
		return "", fmt.Errorf("parse policy YAML: %w", err)
	}
	for pending := []*yaml.Node{&document}; len(pending) > 0; {
		node := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if node.Kind == yaml.AliasNode {
			return "", errors.New("policy must not contain YAML aliases")
		}
		if strings.HasPrefix(node.Tag, "!") && !strings.HasPrefix(node.Tag, "!!") {
			return "", errors.New("policy must not contain custom YAML tags")
		}
		if node.Kind == yaml.ScalarNode && node.Style == 0 {
			switch strings.ToLower(node.Value) {
			case "yes", "no", "on", "off":
				return "", fmt.Errorf("policy value %q must be quoted", node.Value)
			}
		}
		pending = append(pending, node.Content...)
	}

	var rules []map[string]any
	if err := document.Decode(&rules); err != nil {
		return "", fmt.Errorf("parse policy YAML: %w", err)
	}
	if len(rules) == 0 {
		return "", errors.New("policy must contain at least one rule")
	}
	claims := map[string]bool{
		"cluster_id": true, "cluster_queue_id": true, "pipeline_id": true,
		"build_creator_team": true, "build_source": false, "cluster_queue_key": false,
		"pipeline_slug": false, "build_branch": false, "build_creator": false,
	}
	for ruleIndex, rule := range rules {
		if len(rule) == 0 {
			return "", fmt.Errorf("policy rule %d must contain at least one claim", ruleIndex+1)
		}
		for claim, conditions := range rule {
			requiresUUID, known := claims[claim]
			if !known {
				return "", fmt.Errorf("policy rule %d contains unknown claim %q", ruleIndex+1, claim)
			}
			if !validPolicyConditions(conditions, requiresUUID) {
				expected := "non-empty string"
				if requiresUUID {
					expected = "UUID"
				}
				return "", fmt.Errorf("policy rule %d claim %q must be a %s or list of %ss", ruleIndex+1, claim, expected, expected)
			}
		}
	}
	return policy + "\n", nil
}

func validPolicyConditions(value any, requiresUUID bool) bool {
	valid := func(value any) bool {
		text, ok := value.(string)
		return ok && text != "" && (!requiresUUID || uuidPattern.MatchString(text))
	}
	switch conditions := value.(type) {
	case string:
		return valid(conditions)
	case []any:
		if len(conditions) == 0 {
			return false
		}
		for _, condition := range conditions {
			if !valid(condition) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
