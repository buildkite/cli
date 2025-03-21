# Internal Models Package

This package contains domain models used throughout the application. The models represent core entities in the Buildkite API, such as:

- Build
- Pipeline
- Job
- Artifact
- Annotation
- Agent
- Cluster
- User
- Organization

## Usage

The models are intended to be used as the primary data structures for internal application logic. They are separate from the API-specific models provided by the `go-buildkite` SDK.

All models include JSON and YAML struct tags that match the Buildkite API response format, making them suitable for direct serialization and deserialization.

For packages that need to maintain backwards compatibility with existing API structures, conversion functions are provided to transform between our internal models and the `go-buildkite` SDK models.

## Best Practices

1. Use these models as the primary data structures for internal logic
2. Convert to/from API-specific models at the boundaries of your application
3. Add conversion functions to the `convert.go` file as needed
4. Keep the models focused on their domain concerns without adding API-specific details
5. Leverage JSON/YAML serialization for configuration files and API responses

## Examples

### Converting from go-buildkite to internal models

```go
// Convert a go-buildkite Build to an internal Build model
bkBuild := getBuildFromAPI()
internalBuild := models.FromBuildkiteBuild(bkBuild)

// Process using internal model...
```

### Converting from internal models to go-buildkite

```go
// Convert an internal Artifact to a go-buildkite Artifact
internalArtifact := getArtifactFromInternalLogic()
bkArtifact := internalArtifact.ToBuildkiteArtifact()

// Use with go-buildkite API...
```

### Using models with existing packages that have their own structs

Many packages define their own struct types for compatibility reasons. In these cases,
conversion functions are provided to transform between the package-specific types and
the models package types:

```go
// Create a build.Build from a models.Build
modelBuild := &models.Build{...}
packageBuild := build.FromModel(modelBuild)

// Convert back
modelBuild = packageBuild.ToModel()
```

### JSON/YAML Serialization

Models can be easily serialized to JSON or YAML for API responses or configuration files:

```go
// JSON serialization
build := &models.Build{ID: "123", Number: 42}
jsonData, err := json.Marshal(build)

// YAML serialization
pipeline := &models.Pipeline{Name: "My Pipeline", Slug: "my-pipeline"}
yamlData, err := yaml.Marshal(pipeline)
```

Fields marked with `json:"-"` and `yaml:"-"` are for internal use only and will not be included in serialized output.