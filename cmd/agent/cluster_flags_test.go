package agent

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestAgentClusterFlags(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"install", "run"} {
		for _, tt := range []struct {
			name               string
			flags              []string
			wantName, wantUUID string
			wantErr            bool
		}{
			{name: "default"},
			{name: "name", flags: []string{"--cluster-name", "Default cluster"}, wantName: "Default cluster"},
			{name: "uuid", flags: []string{"--cluster-uuid", "cluster-id"}, wantUUID: "cluster-id"},
			{name: "conflict", flags: []string{"--cluster-name", "AWS", "--cluster-uuid", "cluster-id"}, wantErr: true},
		} {
			t.Run(command+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				var cli struct {
					Install InstallCmd `cmd:""`
					Run     RunCmd     `cmd:""`
				}
				parser, err := kong.New(&cli)
				if err != nil {
					t.Fatal(err)
				}
				_, err = parser.Parse(append([]string{command}, tt.flags...))
				if tt.wantErr {
					if err == nil || !strings.Contains(err.Error(), "only one of --cluster-uuid or --cluster-name") {
						t.Fatalf("unexpected error: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				name, uuid := cli.Install.ClusterName, cli.Install.ClusterUUID
				if command == "run" {
					name, uuid = cli.Run.ClusterName, cli.Run.ClusterUUID
				}
				if name != tt.wantName || uuid != tt.wantUUID {
					t.Fatalf("name, UUID = %q, %q; want %q, %q", name, uuid, tt.wantName, tt.wantUUID)
				}
			})
		}
	}
}
