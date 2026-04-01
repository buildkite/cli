package job

import (
	"strings"
	"testing"

	bkGraphQL "github.com/buildkite/cli/v3/internal/graphql"
)

func TestValidateUnblockResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   *bkGraphQL.UnblockJobResponse
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil response",
			input:   nil,
			wantErr: true,
			errMsg:  "failed to unblock job",
		},
		{
			name: "nil payload",
			input: &bkGraphQL.UnblockJobResponse{
				JobTypeBlockUnblock: nil,
			},
			wantErr: true,
			errMsg:  "failed to unblock job",
		},
		{
			name: "successful unblock",
			input: &bkGraphQL.UnblockJobResponse{
				JobTypeBlockUnblock: &bkGraphQL.UnblockJobJobTypeBlockUnblockJobTypeBlockUnblockPayload{
					JobTypeBlock: bkGraphQL.UnblockJobJobTypeBlockUnblockJobTypeBlockUnblockPayloadJobTypeBlock{
						State: bkGraphQL.JobStatesUnblocked,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "unexpected state BLOCKED",
			input: &bkGraphQL.UnblockJobResponse{
				JobTypeBlockUnblock: &bkGraphQL.UnblockJobJobTypeBlockUnblockJobTypeBlockUnblockPayload{
					JobTypeBlock: bkGraphQL.UnblockJobJobTypeBlockUnblockJobTypeBlockUnblockPayloadJobTypeBlock{
						State: bkGraphQL.JobStatesBlocked,
					},
				},
			},
			wantErr: true,
			errMsg:  "BLOCKED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUnblockResponse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUnblockResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateUnblockResponse() error = %q, want substring %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}
