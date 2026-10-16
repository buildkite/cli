package analytics

// noopLogger implements the posthog.Logger interface
// It suppresses all PostHog SDK logs
type noopLogger struct{}

func (noopLogger) Debugf(format string, args ...interface{}) {}

func (noopLogger) Logf(format string, args ...interface{}) {}

func (noopLogger) Warnf(format string, args ...interface{}) {}

func (noopLogger) Errorf(format string, args ...interface{}) {}
