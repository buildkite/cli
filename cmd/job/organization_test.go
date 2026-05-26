package job

import "testing"

func TestConfiguredOrganization(t *testing.T) {
	t.Parallel()

	got, err := configuredOrganization("buildkite")
	if err != nil {
		t.Fatalf("configuredOrganization() error = %v", err)
	}
	if got != "buildkite" {
		t.Fatalf("configuredOrganization() = %q", got)
	}
}

func TestConfiguredOrganizationRequiresOrganization(t *testing.T) {
	t.Parallel()

	if _, err := configuredOrganization(""); err == nil {
		t.Fatal("configuredOrganization() error = nil, want error")
	}
}
