package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/buildkite/cli/v3/internal/preflight"
)

type job struct {
	name     string
	duration time.Duration
	promoted bool
}

type testResult struct {
	name   string
	passed bool
	time   time.Duration
}

func main() {
	s := preflight.NewScreen(os.Stdout)
	header := s.AddRegion("header")
	completed := s.AddRegion("completed")
	jobsRegion := s.AddRegion("jobs")
	testHeader := s.AddRegion("test-header")
	testsRegion := s.AddRegion("tests")
	status := s.AddRegion("status")

	header.SetLines([]string{
		"",
		"  Build #42 — my-org/my-pipeline",
		"  ────────────────────────────────",
	})

	// Generate 100 jobs with staggered durations.
	categories := []string{
		"lint", "vet", "typecheck", "security-scan",
		"unit-test", "integration-test", "e2e-test",
		"build-linux", "build-darwin", "build-windows",
		"docker-build", "docker-push",
		"deploy-staging", "deploy-canary", "smoke-test",
	}

	jobs := make([]job, 100)
	for i := range jobs {
		cat := categories[i%len(categories)]
		jobs[i] = job{
			name:     fmt.Sprintf("%s-%d", cat, i/len(categories)),
			duration: time.Duration(2+rand.Intn(18)) * time.Second,
		}
	}

	// Test results arrive independently over time.
	testNames := []string{
		"TestUserCreate", "TestUserDelete", "TestUserUpdate", "TestUserList",
		"TestAuthLogin", "TestAuthLogout", "TestAuthRefresh", "TestAuthMFA",
		"TestBuildCreate", "TestBuildCancel", "TestBuildRetry", "TestBuildList",
		"TestPipelineResolve", "TestPipelineUpload", "TestPipelineValidate",
		"TestSnapshotCommitted", "TestSnapshotUntracked", "TestSnapshotIndex",
		"TestConfigGet", "TestConfigSet", "TestConfigList", "TestConfigExperiments",
		"TestAgentInstall", "TestAgentRun", "TestAgentStop", "TestAgentStatus",
		"TestJobList", "TestJobLog", "TestJobRetry", "TestJobCancel",
		"TestArtifactUpload", "TestArtifactDownload", "TestArtifactList",
		"TestClusterCreate", "TestClusterList", "TestClusterQueue",
		"TestAPITokenRotate", "TestAPIRateLimit", "TestAPIPagination",
		"TestWebhookVerify", "TestWebhookDeliver",
	}
	var tests []testResult
	testInterval := 300 * time.Millisecond
	nextTest := time.Now().Add(500 * time.Millisecond)
	testIdx := 0
	maxVisibleTests := 8

	start := time.Now()
	tick := 0
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	for {
		elapsed := time.Since(start)

		// --- Discover new test results ---
		if testIdx < len(testNames) && time.Now().After(nextTest) {
			passed := rand.Float64() > 0.08 // ~8% failure rate
			tests = append(tests, testResult{
				name:   testNames[testIdx],
				passed: passed,
				time:   time.Duration(rand.Intn(500)+10) * time.Millisecond,
			})
			testIdx++
			nextTest = time.Now().Add(testInterval)
		}

		// --- Promote completed jobs ---
		for i := range jobs {
			if !jobs[i].promoted && elapsed >= jobs[i].duration {
				jobs[i].promoted = true
				line := fmt.Sprintf("  \033[32m✓\033[0m %-40s \033[32mpassed\033[0m  \033[2m(%s)\033[0m",
					jobs[i].name, jobs[i].duration)
				completed.AppendLine(line)
			}
		}

		// --- Render active jobs (max 6 visible + summary) ---
		var live []string
		var running, scheduled int
		for i := range jobs {
			if jobs[i].promoted {
				continue
			}
			if elapsed < jobs[i].duration {
				dur := elapsed.Truncate(time.Second)
				if running < 6 {
					live = append(live, fmt.Sprintf("  \033[36m●\033[0m %-40s \033[36mrunning\033[0m \033[2m%s\033[0m",
						jobs[i].name, dur))
				}
				running++
			} else {
				scheduled++
			}
		}
		if running > 6 {
			live = append(live, fmt.Sprintf("  \033[90m… and %d more running\033[0m", running-6))
		}
		if scheduled > 0 {
			live = append(live, fmt.Sprintf("  \033[90m◌ %d scheduled\033[0m", scheduled))
		}
		jobsRegion.SetLines(live)

		// --- Render test results (tail of recent tests) ---
		if len(tests) > 0 {
			totalPassed := 0
			totalFailed := 0
			for _, tr := range tests {
				if tr.passed {
					totalPassed++
				} else {
					totalFailed++
				}
			}
			hdr := fmt.Sprintf("  Tests: \033[32m%d passed\033[0m", totalPassed)
			if totalFailed > 0 {
				hdr += fmt.Sprintf(", \033[31m%d failed\033[0m", totalFailed)
			}
			if testIdx < len(testNames) {
				hdr += fmt.Sprintf(" \033[90m(%d/%d)\033[0m", len(tests), len(testNames))
			} else {
				hdr += fmt.Sprintf(" \033[90m(done)\033[0m")
			}
			testHeader.SetLines([]string{"", hdr})

			// Show last N test results.
			startIdx := 0
			if len(tests) > maxVisibleTests {
				startIdx = len(tests) - maxVisibleTests
			}
			var testLines []string
			for _, tr := range tests[startIdx:] {
				icon := "\033[32m✓\033[0m"
				if !tr.passed {
					icon = "\033[31m✗\033[0m"
				}
				testLines = append(testLines, fmt.Sprintf("    %s %-40s \033[2m%s\033[0m",
					icon, tr.name, tr.time))
			}
			if startIdx > 0 {
				testLines = append(testLines, fmt.Sprintf("    \033[90m… %d earlier tests\033[0m", startIdx))
			}
			testsRegion.SetLines(testLines)
		}

		// --- Status line ---
		allDone := running == 0 && scheduled == 0
		if allDone && testIdx >= len(testNames) {
			jobsRegion.SetLines(nil)
			failed := 0
			for _, tr := range tests {
				if !tr.passed {
					failed++
				}
			}
			if failed > 0 {
				status.SetLines([]string{"", fmt.Sprintf("  \033[31m❌ Build failed (%d test failures)\033[0m", failed), ""})
			} else {
				status.SetLines([]string{"", "  \033[32m✅ Build passed!\033[0m", ""})
			}
			s.Flush()
			break
		}

		spinner := spinnerFrames[tick%len(spinnerFrames)]
		promoted := 0
		for _, j := range jobs {
			if j.promoted {
				promoted++
			}
		}
		status.SetLines([]string{
			"",
			fmt.Sprintf("  \033[36m%s\033[0m Watching build #42… %d/%d jobs complete",
				spinner, promoted, len(jobs)),
			"  \033[90mUse `bk log view <job-id>` to view logs\033[0m",
		})

		tick++
		time.Sleep(100 * time.Millisecond)
	}
}
