package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/buildkite/cli/graphql"
)

type ArtifactDownloadCommandContext struct {
	TerminalContext
	KeyringContext

	Debug     bool
	DebugHTTP bool

	Build string
	Job   string
}

func ArtifactDownloadCommand(ctx ArtifactDownloadCommandContext) error {
	bk, err := ctx.BuildkiteGraphQLClient()
	if err != nil {
		return NewExitError(err, 1)
	}

	if ctx.Job == "" && ctx.Build == "" {
		return NewExitError(errors.New("--job or --build required"), 1)
	}

	try := ctx.Try()
	try.Start("Downloading artifacts")

	var artifacts []artifact

	if ctx.Build != "" {
		artifacts, err = findBuildkiteBuildArtifacts(bk, ctx.Build)
		if err != nil {
			try.Failure("Failed")
			return NewExitError(err, 1)
		}
	} else if ctx.Job != "" {
		artifacts, err = findBuildkiteJobArtifacts(bk, ctx.Job)
		if err != nil {
			try.Failure("Failed")
			return NewExitError(err, 1)
		}
	}

	total := len(artifacts)

	for _, artifact := range artifacts {
		err := downloadArtifact(artifact)
		if err != nil {
			try.Failure("Failed")
			return NewExitError(err, 1)
		}

		try.Println(fmt.Sprintf("%s (%d bytes)", artifact.Path, artifact.Size))
	}

	try.Success(fmt.Sprintf("Downloaded %d artifacts", total))

	return nil
}

const artifactDownloadLimit = 100

type artifact struct {
	ID          string
	Path        string
	Size        int
	DownloadURL string
}

func findBuildkiteJobArtifacts(client *graphql.Client, jobID string) ([]artifact, error) {
	resp, err := client.Do(`
		query($jobID: ID!, $limit: Int!) {
			job(uuid: $jobID) {
				...on JobTypeCommand {
					artifacts(first: $limit) {
						count
						edges {
							node {
								id
								path
								size
								downloadURL
							}
						}
					}
				}
			}
		}
	`, map[string]interface{}{
		"jobID": jobID,
		"limit": artifactDownloadLimit,
	})
	if err != nil {
		return []artifact{}, fmt.Errorf("Failed perform GraphQL query: %v", err)
	}

	var parsedResp struct {
		Data struct {
			Job struct {
				Artifacts struct {
					Count int `json:"count"`
					Edges []struct {
						Node struct {
							ID          string `json:"id"`
							Path        string `json:"path"`
							Size        int    `json:"size"`
							DownloadURL string `json:"downloadURL"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"artifacts"`
			} `json:"job"`
		} `json:"data"`
	}

	if err = resp.DecodeInto(&parsedResp); err != nil {
		return []artifact{}, fmt.Errorf("Failed to parse GraphQL response: %v", err)
	}

	if parsedResp.Data.Job.Artifacts.Count > artifactDownloadLimit {
		// GraphQL CommandJob.artifacts only supports `first` so cannot paginate
		return []artifact{}, fmt.Errorf("Too many artifacts\n\nJob has %d artifacts but this tool can only download %d",
			parsedResp.Data.Job.Artifacts.Count, artifactDownloadLimit)
	}

	var artifacts []artifact

	for _, artifactEdge := range parsedResp.Data.Job.Artifacts.Edges {
		artifacts = append(artifacts, artifact{
			ID:          artifactEdge.Node.ID,
			Path:        artifactEdge.Node.Path,
			Size:        artifactEdge.Node.Size,
			DownloadURL: artifactEdge.Node.DownloadURL,
		})
	}

	return artifacts, nil
}

func findBuildkiteBuildArtifacts(client *graphql.Client, buildID string) ([]artifact, error) {
	resp, err := client.Do(`
		query($buildID: ID!, $limit: Int!) {
			build(uuid: $buildID) {
				jobs(first: $limit) {
					count
					edges {
						node {
							...on JobTypeCommand {
								artifacts(first: $limit) {
									count
									edges {
										node {
											id
											path
											size
											downloadURL
										}
									}
								}
							}
						}
					}
				}
			}
		}
	`, map[string]interface{}{
		"buildID": buildID,
		"limit":   artifactDownloadLimit,
	})
	if err != nil {
		return []artifact{}, fmt.Errorf("Failed perform GraphQL query: %v", err)
	}

	var parsedResp struct {
		Data struct {
			Build struct {
				Jobs struct {
					Count int `json:"count"`
					Edges []struct {
						Node struct {
							Artifacts struct {
								Count int `json:"count"`
								Edges []struct {
									Node struct {
										ID          string `json:"id"`
										Path        string `json:"path"`
										Size        int    `json:"size"`
										DownloadURL string `json:"downloadURL"`
									} `json:"node"`
								} `json:"edges"`
							} `json:"artifacts"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"jobs"`
			} `json:"build"`
		} `json:"data"`
	}

	if err = resp.DecodeInto(&parsedResp); err != nil {
		return []artifact{}, fmt.Errorf("Failed to parse GraphQL response: %v", err)
	}

	if parsedResp.Data.Build.Jobs.Count > artifactDownloadLimit {
		return []artifact{}, fmt.Errorf("Too many jobs\n\nBuild has %d jobs but this tool can only download %d",
			parsedResp.Data.Build.Jobs.Count, artifactDownloadLimit)
	}

	var artifacts []artifact
	for _, jobEdge := range parsedResp.Data.Build.Jobs.Edges {
		if jobEdge.Node.Artifacts.Count > artifactDownloadLimit {
			return []artifact{}, fmt.Errorf("Too many artifacts\n\nJob has %d artifacts but this tool can only download %d",
				jobEdge.Node.Artifacts.Count, artifactDownloadLimit)
		}

		for _, artifactEdge := range jobEdge.Node.Artifacts.Edges {
			artifacts = append(artifacts, artifact{
				ID:          artifactEdge.Node.ID,
				Path:        artifactEdge.Node.Path,
				Size:        artifactEdge.Node.Size,
				DownloadURL: artifactEdge.Node.DownloadURL,
			})
		}
	}

	return artifacts, nil
}

func downloadArtifact(artifact artifact) error {
	path := artifact.Path

	dir := filepath.Dir(artifact.Path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	resp, err := http.Get(artifact.DownloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
