package concept4

import (
	"os"
	"strings"
	"testing"
)

func TestDemo(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")
	out := Demo()
	for _, want := range []string{
		"preflight #4821",
		"main @ a1b2c3d",
		"Lint",
		"Unit Tests",
		"Integration Tests",
		"E2E Tests",
		"Deploy Preview",
		"TestUserAuth/login_with_expired_token",
		"context deadline exceeded",
		"● running...",
		"· waiting",
		"──:──",
		"failed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
	t.Log("\n" + out)
}
