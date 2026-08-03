package analytics

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/posthog/posthog-go"
)

func TestCloseDoesNotWaitForPostHogUploadTimeout(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var signalRequestStarted sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		signalRequestStarted.Do(func() { close(requestStarted) })
		<-releaseRequest
	}))
	defer server.Close()

	originalAPIHost := apiHost
	apiHost = server.URL
	client = nil
	once = sync.Once{}
	t.Cleanup(func() {
		apiHost = originalAPIHost
		client = nil
		once = sync.Once{}
	})
	t.Setenv("BK_ANALYTICS_KEY", "test-key")
	t.Setenv("CI", "")

	tracker := Init("test", true)
	defer tracker.Close()
	defer close(releaseRequest)

	for range posthog.DefaultBatchSize {
		tracker.TrackCommand("build list", []string{"build", "list"}, nil)
	}
	select {
	case <-requestStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("expected queued telemetry to start uploading")
	}

	start := time.Now()
	tracker.Close()
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("Close waited %s for unavailable telemetry; expected less than 2s", elapsed)
	}
}
