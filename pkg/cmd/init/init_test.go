package init

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindExistingPipelineFileWithNoFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "bk-cli-*")
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(dir)

	if found, _ := findExistingPipelineFile(dir); found {
		t.Fail()
	}
}

func TestFindExistingPipelineFile(t *testing.T) {
	dir, err := os.MkdirTemp("", "bk-cli-*")
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(dir)
	_ = os.MkdirAll(filepath.Join(dir, ".buildkite"), 0755)
	f, _ := os.Create(filepath.Join(dir, ".buildkite", "pipeline.yml"))
	defer f.Close()

	if found, _ := findExistingPipelineFile(dir); !found {
		t.Fail()
	}
}
