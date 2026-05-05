package auth

import "testing"

func TestOrganizationIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		org      string
		wantSlug string
		wantUUID string
	}{
		{
			name:     "slug",
			org:      "buildkite",
			wantSlug: "buildkite",
			wantUUID: "",
		},
		{
			name:     "uuid",
			org:      "018f2f7e-7e99-7d77-b4d3-a95cb01805f4",
			wantSlug: "",
			wantUUID: "018f2f7e-7e99-7d77-b4d3-a95cb01805f4",
		},
		{
			name:     "uppercase uuid",
			org:      "018F2F7E-7E99-7D77-B4D3-A95CB01805F4",
			wantSlug: "",
			wantUUID: "018F2F7E-7E99-7D77-B4D3-A95CB01805F4",
		},
		{
			name:     "uuid-like slug without hyphens",
			org:      "018f2f7e7e997d77b4d3a95cb01805f4",
			wantSlug: "018f2f7e7e997d77b4d3a95cb01805f4",
			wantUUID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotSlug, gotUUID := organizationIdentifier(tt.org)
			if gotSlug != tt.wantSlug || gotUUID != tt.wantUUID {
				t.Fatalf("organizationIdentifier(%q) = (%q, %q), want (%q, %q)", tt.org, gotSlug, gotUUID, tt.wantSlug, tt.wantUUID)
			}
		})
	}
}
