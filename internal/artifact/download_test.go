package artifact

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadToFileCreatesParentDirAndWritesBody(t *testing.T) {
	t.Parallel()

	const body = "artifact-bytes"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	destPath := filepath.Join(t.TempDir(), "nested", "dir", "file.bin")

	if err := DownloadToFile(context.Background(), newTestClient(t, server.URL), server.URL, destPath); err != nil {
		t.Fatalf("DownloadToFile() error = %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != body {
		t.Fatalf("file contents = %q, want %q", got, body)
	}
}
