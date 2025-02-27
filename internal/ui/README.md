# UI Components Package

This package provides standardized UI components and styling for Buildkite CLI output, ensuring consistent visual presentation across the application.

## Features

- **Consistent Colors**: Standard color palette for different states (success, error, warning, etc.)
- **Reusable Icons**: Standard icons for different states and actions
- **Standardized Components**: Pre-styled components for common UI elements
- **Layout Helpers**: Utilities for creating consistent layouts
- **Text Formatting**: Utilities for text manipulation and formatting
- **Buildkite Entity Rendering**: Components for rendering Buildkite entities like builds, jobs, artifacts, etc.

## Usage

### Basic Styling

```go
import "github.com/buildkite/cli/v3/internal/ui"

// Use predefined styles
title := ui.Bold.Render("This is bold text")
subtitle := ui.Italic.Render("This is italic text")

// Use predefined colors
successText := ui.StatusStyle("success").Render("Operation successful")
errorText := ui.StatusStyle("error").Render("Operation failed")
```

### Layout Components

```go
import "github.com/buildkite/cli/v3/internal/ui"

// Create a section with title and content
section := ui.Section("Section Title", "Section content goes here")

// Create a row with columns
row := ui.Row("Column 1", "Column 2", "Column 3")

// Create a labeled value
label := ui.LabeledValue("Label", "Value")

// Create a table
headers := []string{"Name", "Value"}
rows := [][]string{
    {"Row 1", "Value 1"},
    {"Row 2", "Value 2"},
}
table := ui.Table(headers, rows)

// Create a card
card := ui.Card("Card Title", "Card content", ui.WithBorder(true))
```

### Buildkite Entity Rendering

```go
import "github.com/buildkite/cli/v3/internal/ui"

// Render build summary
buildSummary := ui.RenderBuildSummary(build)

// Render job summary
jobSummary := ui.RenderJobSummary(job)

// Render artifact
artifactSummary := ui.RenderArtifact(artifact)

// Render annotation
annotationSummary := ui.RenderAnnotation(annotation)

// Render agent summary
agentSummary := ui.RenderAgentSummary(agent)

// Render cluster summary
clusterSummary := ui.RenderClusterSummary(cluster)
```

### Text Formatting

```go
import "github.com/buildkite/cli/v3/internal/ui"

// Truncate text
truncated := ui.TruncateText("Long text to truncate", 10)

// Strip HTML tags
plain := ui.StripHTMLTags("<p>HTML <b>content</b></p>")

// Format bytes
human := ui.FormatBytes(1500000) // "1.5MB"

// Format date
date := ui.FormatDate(time.Now())
```

## Design Principles

1. **Consistency**: All UI components follow the same design patterns and color schemes
2. **Modularity**: Components are designed to be reusable and composable
3. **Separation of Concerns**: UI rendering is separated from business logic
4. **Testability**: Components are designed to be easily testable

By using this package for all UI rendering, we ensure a consistent look and feel across the application while reducing code duplication.
