package artifacts

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/internal/graphql"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
)

func NewCmdArtifactsDownload(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "download <artifact UUID>",
		Short:                 "Download an artifact by its UUID",
		Args:                  cobra.ExactArgs(1),
		Long: heredoc.Doc(`
			Use this command to download a specific artifact. 
		`),
		Example: heredoc.Doc(`			
			$ bk artifacts download 0191727d-b5ce-4576-b37d-477ae0ca830c 
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			artifactId := args[0]

			var err error
			var downloadDir string
			spinErr := spinner.New().
				Title("Downloading artifact").
				Action(func() {
					downloadDir, err = download(cmd.Context(), f, artifactId)
				}).
				Run()
			if spinErr != nil {
				return spinErr
			}
			if err != nil {
				fmt.Println("EXITING due to ERROR HERE")
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Downloaded artifact to: %s\n", downloadDir)

			return err
		},
	}

	return &cmd
}

func download(ctx context.Context, f *factory.Factory, artifactId string) (string, error) {
	var err error
	var resp *graphql.GetArtifactsResponse

	resp, err = graphql.GetArtifacts(ctx, f.GraphQLClient, artifactId)
	if err != nil {
		return "", err
	}

	if resp == nil || resp.Artifact == nil {
		return "", fmt.Errorf("no artifact found with ID: %s", artifactId)
	}

	directory := fmt.Sprintf("artifact-%s", artifactId)
	err = os.MkdirAll(directory, os.ModePerm)
	if err != nil {
		return "", err
	}

	filename := filepath.Base(resp.Artifact.Path)
	out, fileErr := os.Create(filepath.Join(directory, filename))
	if fileErr != nil {
		return "", fileErr
	}
	defer out.Close()

	apiResp, apiErr := http.Get(resp.Artifact.DownloadURL)
	if apiErr != nil {
		return "", apiErr
	}
	defer apiResp.Body.Close()

	// Writer the body to file
	_, err = io.Copy(out, apiResp.Body)
	if err != nil {
		return "", err
	}

	return directory, nil
}
