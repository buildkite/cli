package scopes

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type Scope string

const (
	// Agent scopes
	ReadAgents  Scope = "read_agents"
	WriteAgents Scope = "write_agents"

	// Cluster scopes
	ReadClusters  Scope = "read_clusters"
	WriteClusters Scope = "write_clusters"

	// Team scopes
	ReadTeams  Scope = "read_teams"
	WriteTeams Scope = "write_teams"

	// Artifact scopes
	ReadArtifacts  Scope = "read_artifacts"
	WriteArtifacts Scope = "write_artifacts"

	// Build scopes
	ReadBuilds  Scope = "read_builds"
	WriteBuilds Scope = "write_builds"

	// Build logs and environment scopes
	ReadJobEnv     Scope = "read_job_env"
	ReadBuildLogs  Scope = "read_build_logs"
	WriteBuildLogs Scope = "write_build_logs"

	// Organization scopes
	ReadOrganizations Scope = "read_organizations"

	// Pipeline scopes
	ReadPipelines  Scope = "read_pipelines"
	WritePipelines Scope = "write_pipelines"

	// Pipeline template scopes
	ReadPipelineTemplates  Scope = "read_pipeline_templates"
	WritePipelineTemplates Scope = "write_pipeline_templates"

	// Rule scopes
	ReadRules  Scope = "read_rules"
	WriteRules Scope = "write_rules"

	// User scopes
	ReadUser Scope = "read_user"

	// Test suite scopes
	ReadSuites  Scope = "read_suites"
	WriteSuites Scope = "write_suites"

	// Test plan scopes
	ReadTestPlan  Scope = "read_test_plan"
	WriteTestPlan Scope = "write_test_plan"

	// Registry scopes
	ReadRegistries   Scope = "read_registries"
	WriteRegistries  Scope = "write_registries"
	DeleteRegistries Scope = "delete_registries"

	// Package scopes
	ReadPackages   Scope = "read_packages"
	WritePackages  Scope = "write_packages"
	DeletePackages Scope = "delete_packages"

	// GraphQL scope
	GraphQL Scope = "graphql"
)

type CommandScopes struct {
	Required []Scope
}

func GetCommandScopes(cmd *cobra.Command) CommandScopes {
	required := []Scope{}

	if reqScopes, ok := cmd.Annotations["requiredScopes"]; ok {
		for _, scope := range strings.Split(reqScopes, ",") {
			required = append(required, Scope(strings.TrimSpace(scope)))
		}
	}

	return CommandScopes{
		Required: required,
	}
}

func ValidateScopes(cmdScopes CommandScopes, tokenScopes []string) error {
	missingRequired := []string{}

	for _, requiredScope := range cmdScopes.Required {
		found := false
		for _, tokenScope := range tokenScopes {
			if string(requiredScope) == tokenScope {
				found = true
				break
			}
		}
		if !found {
			missingRequired = append(missingRequired, string(requiredScope))
		}
	}

	if len(missingRequired) > 0 {
		return fmt.Errorf("missing required scopes: %s", strings.Join(missingRequired, ", "))
	}

	return nil
}
