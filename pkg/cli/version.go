package cli

import (
	"context"
	"fmt"

	"github.com/buildkite/cli/v3/pkg/factory"
)

// Version command
type VersionSub struct{}

func (v *VersionSub) Run(ctx context.Context, f *factory.Factory) error {
	fmt.Printf("bk version %s\n", f.Version)
	return nil
}
