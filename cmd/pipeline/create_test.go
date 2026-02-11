package pipeline

import (
	"testing"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
)

func TestIsGitHubURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "GitHub SSH URL",
			url:      "git@github.com:org/repo.git",
			expected: true,
		},
		{
			name:     "GitHub HTTPS URL",
			url:      "https://github.com/org/repo.git",
			expected: true,
		},
		{
			name:     "GitHub HTTPS URL without .git",
			url:      "https://github.com/org/repo",
			expected: true,
		},
		{
			name:     "GitHub Enterprise SSH URL",
			url:      "git@github.mycompany.com:org/repo.git",
			expected: true,
		},
		{
			name:     "GitHub Enterprise HTTPS URL",
			url:      "https://github.mycompany.com/org/repo.git",
			expected: true,
		},
		{
			name:     "GitLab SSH URL",
			url:      "git@gitlab.com:org/repo.git",
			expected: false,
		},
		{
			name:     "GitLab HTTPS URL",
			url:      "https://gitlab.com/org/repo.git",
			expected: false,
		},
		{
			name:     "Bitbucket SSH URL",
			url:      "git@bitbucket.org:org/repo.git",
			expected: false,
		},
		{
			name:     "Bitbucket HTTPS URL",
			url:      "https://bitbucket.org/org/repo.git",
			expected: false,
		},
		{
			name:     "empty string",
			url:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isGitHubURL(tt.url)
			if got != tt.expected {
				t.Errorf("isGitHubURL(%q) = %v, want %v", tt.url, got, tt.expected)
			}
		})
	}
}

func TestGetRepositoryURL(t *testing.T) {
	t.Parallel()

	t.Run("returns flag value when provided", func(t *testing.T) {
		t.Parallel()
		f := &factory.Factory{}
		repoURL := getRepositoryURL(f, "git@github.com:org/repo.git")
		if repoURL != "git@github.com:org/repo.git" {
			t.Errorf("expected flag value, got %q", repoURL)
		}
	})

	t.Run("returns empty when no flag and nil git repo", func(t *testing.T) {
		t.Parallel()
		f := &factory.Factory{}
		repoURL := getRepositoryURL(f, "")
		if repoURL != "" {
			t.Errorf("expected empty string, got %q", repoURL)
		}
	})

	t.Run("returns empty when factory is nil and no flag", func(t *testing.T) {
		t.Parallel()
		repoURL := getRepositoryURL(nil, "")
		if repoURL != "" {
			t.Errorf("expected empty string, got %q", repoURL)
		}
	})

	t.Run("returns flag value even when factory is nil", func(t *testing.T) {
		t.Parallel()
		repoURL := getRepositoryURL(nil, "git@github.com:org/repo.git")
		if repoURL != "git@github.com:org/repo.git" {
			t.Errorf("expected flag value, got %q", repoURL)
		}
	})
}

func TestExtractRepoPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "GitHub SSH", input: "git@github.com:org/repo.git", expected: "org/repo"},
		{name: "GitHub HTTPS", input: "https://github.com/org/repo.git", expected: "org/repo"},
		{name: "GitHub HTTPS no .git", input: "https://github.com/org/repo", expected: "org/repo"},
		{name: "non-GitHub URL", input: "git@gitlab.com:org/repo.git", expected: "git@gitlab.com:org/repo.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractRepoPath(tt.input)
			if got != tt.expected {
				t.Errorf("extractRepoPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
