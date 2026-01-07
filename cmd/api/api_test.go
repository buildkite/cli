package api

import (
	"testing"
)

func TestBuildFullEndpoint(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		endpoint     string
		orgSlug      string
		isAnalytics  bool
		wantEndpoint string
	}{
		"endpoint with leading slash": {
			endpoint:     "/pipelines/dummy/builds/5085",
			orgSlug:      "test-org",
			isAnalytics:  false,
			wantEndpoint: "v2/organizations/test-org/pipelines/dummy/builds/5085",
		},
		"endpoint without leading slash": {
			endpoint:     "pipelines/dummy/builds/5085",
			orgSlug:      "test-org",
			isAnalytics:  false,
			wantEndpoint: "v2/organizations/test-org/pipelines/dummy/builds/5085",
		},
		"empty endpoint": {
			endpoint:     "",
			orgSlug:      "test-org",
			isAnalytics:  false,
			wantEndpoint: "v2/organizations/test-org/",
		},
		"root endpoint": {
			endpoint:     "/",
			orgSlug:      "test-org",
			isAnalytics:  false,
			wantEndpoint: "v2/organizations/test-org/",
		},
		"analytics endpoint with leading slash": {
			endpoint:     "/suites",
			orgSlug:      "test-org",
			isAnalytics:  true,
			wantEndpoint: "v2/analytics/organizations/test-org/suites",
		},
		"analytics endpoint without leading slash": {
			endpoint:     "suites",
			orgSlug:      "test-org",
			isAnalytics:  true,
			wantEndpoint: "v2/analytics/organizations/test-org/suites",
		},
		"pipeline endpoint without leading slash": {
			endpoint:     "pipelines",
			orgSlug:      "acme-inc",
			isAnalytics:  false,
			wantEndpoint: "v2/organizations/acme-inc/pipelines",
		},
	}

	for name, tc := range testcases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := buildFullEndpoint(tc.endpoint, tc.orgSlug, tc.isAnalytics)

			if got != tc.wantEndpoint {
				t.Errorf("buildFullEndpoint(%q, %q, %v) = %q, want %q",
					tc.endpoint, tc.orgSlug, tc.isAnalytics, got, tc.wantEndpoint)
			}
		})
	}
}
