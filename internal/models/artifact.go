package models

// Artifact represents a Buildkite artifact
type Artifact struct {
	ID            string `json:"id,omitempty" yaml:"id,omitempty"`
	JobID         string `json:"job_id,omitempty" yaml:"job_id,omitempty"`
	URL           string `json:"url,omitempty" yaml:"url,omitempty"`
	DownloadURL   string `json:"download_url,omitempty" yaml:"download_url,omitempty"`
	State         string `json:"state,omitempty" yaml:"state,omitempty"`
	Path          string `json:"path,omitempty" yaml:"path,omitempty"`
	Dirname       string `json:"dirname,omitempty" yaml:"dirname,omitempty"`
	Filename      string `json:"filename,omitempty" yaml:"filename,omitempty"`
	MimeType      string `json:"mime_type,omitempty" yaml:"mime_type,omitempty"`
	FileSize      int64  `json:"file_size,omitempty" yaml:"file_size,omitempty"`
	GlobPath      string `json:"glob_path,omitempty" yaml:"glob_path,omitempty"`
	OriginalPath  string `json:"original_path,omitempty" yaml:"original_path,omitempty"`
	Sha1Sum       string `json:"sha1sum,omitempty" yaml:"sha1sum,omitempty"`
	CreatedAt     string `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UploadedAt    string `json:"uploaded_at,omitempty" yaml:"uploaded_at,omitempty"`
}