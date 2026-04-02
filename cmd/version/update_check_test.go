package version

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckForUpdate_NewerVersionAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name": "v2.0.0"}`)
	}))
	defer server.Close()

	releaseURL = server.URL

	latest, hasUpdate := CheckForUpdate("1.0.0")
	if !hasUpdate {
		t.Fatal("expected hasUpdate to be true")
	}
	if latest != "2.0.0" {
		t.Fatalf("expected latest to be 2.0.0, got %s", latest)
	}
}

func TestCheckForUpdate_SameVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name": "v1.0.0"}`)
	}))
	defer server.Close()

	releaseURL = server.URL

	_, hasUpdate := CheckForUpdate("1.0.0")
	if hasUpdate {
		t.Fatal("expected hasUpdate to be false for same version")
	}
}

func TestCheckForUpdate_OlderVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name": "v0.9.0"}`)
	}))
	defer server.Close()

	releaseURL = server.URL

	_, hasUpdate := CheckForUpdate("1.0.0")
	if hasUpdate {
		t.Fatal("expected hasUpdate to be false for older version")
	}
}

func TestCheckForUpdate_DevVersion(t *testing.T) {
	_, hasUpdate := CheckForUpdate("DEV")
	if hasUpdate {
		t.Fatal("expected hasUpdate to be false for DEV version")
	}
}

func TestCheckForUpdate_NonReleaseVersionSkipsLookup(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		fmt.Fprint(w, `{"tag_name": "v9.9.9"}`)
	}))
	defer server.Close()

	releaseURL = server.URL

	_, hasUpdate := CheckForUpdate("v3.1.0-12-gabc1234")
	if hasUpdate {
		t.Fatal("expected hasUpdate to be false for non-release version")
	}
	if requestCount != 0 {
		t.Fatalf("expected no release lookup for non-release version, got %d requests", requestCount)
	}
}

func TestCheckForUpdate_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	releaseURL = server.URL

	_, hasUpdate := CheckForUpdate("1.0.0")
	if hasUpdate {
		t.Fatal("expected hasUpdate to be false on server error")
	}
}

func TestCheckForUpdate_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json`)
	}))
	defer server.Close()

	releaseURL = server.URL

	_, hasUpdate := CheckForUpdate("1.0.0")
	if hasUpdate {
		t.Fatal("expected hasUpdate to be false on invalid JSON")
	}
}

func TestCheckForUpdate_ServerDown(t *testing.T) {
	releaseURL = "http://127.0.0.1:1" // nothing listening

	_, hasUpdate := CheckForUpdate("1.0.0")
	if hasUpdate {
		t.Fatal("expected hasUpdate to be false when server is unreachable")
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"2.0.0", "1.0.0", true},
		{"1.1.0", "1.0.0", true},
		{"1.0.1", "1.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"0.9.0", "1.0.0", false},
		{"1.0.0", "1.0.1", false},
		{"10.0.0", "9.0.0", true},
		{"1.10.0", "1.9.0", true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_vs_%s", tt.a, tt.b), func(t *testing.T) {
			got := isNewer(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"1.2.3", true},
		{"0.0.0", true},
		{"10.20.30", true},
		{"1.2", false},
		{"1.2.3.4", false},
		{"a.b.c", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseVersion(tt.input)
			if tt.valid && result == nil {
				t.Errorf("parseVersion(%q) returned nil, expected valid", tt.input)
			}
			if !tt.valid && result != nil {
				t.Errorf("parseVersion(%q) returned %v, expected nil", tt.input, result)
			}
		})
	}
}
