package job

import (
	"testing"

	bkGraphQL "github.com/buildkite/cli/v3/internal/graphql"
)

func TestValidateUnblockResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   *bkGraphQL.UnblockJobResponse
		wantErr bool
	}{
		{
			name:    "nil response",
			input:   nil,
			wantErr: true,
		},
		{
			name: "nil payload",
			input: &bkGraphQL.UnblockJobResponse{
				JobTypeBlockUnblock: nil,
			},
			wantErr: true,
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
			name: "non-nil payload with finished state",
			input: &bkGraphQL.UnblockJobResponse{
				JobTypeBlockUnblock: &bkGraphQL.UnblockJobJobTypeBlockUnblockJobTypeBlockUnblockPayload{
					JobTypeBlock: bkGraphQL.UnblockJobJobTypeBlockUnblockJobTypeBlockUnblockPayloadJobTypeBlock{
						State: bkGraphQL.JobStatesFinished,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateUnblockResponse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUnblockResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
