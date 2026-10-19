package build

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	buildkite "github.com/buildkite/go-buildkite/v5"
)

type comparisonJob struct {
	ID                string   `json:"id" yaml:"id"`
	Name              string   `json:"name" yaml:"name"`
	State             string   `json:"state" yaml:"state"`
	SoftFailed        bool     `json:"soft_failed" yaml:"soft_failed"`
	ExitStatus        *int     `json:"exit_status,omitempty" yaml:"exit_status,omitempty"`
	SignalReason      string   `json:"signal_reason,omitempty" yaml:"signal_reason,omitempty"`
	RetriesCount      int      `json:"retries_count" yaml:"retries_count"`
	WebURL            string   `json:"web_url" yaml:"web_url"`
	ExecutionSeconds  *float64 `json:"execution_seconds,omitempty" yaml:"execution_seconds,omitempty"`
	SchedulingSeconds *float64 `json:"scheduling_seconds,omitempty" yaml:"scheduling_seconds,omitempty"`
}

type stepComparison struct {
	Change                string         `json:"change" yaml:"change"`
	MatchMethod           string         `json:"match_method,omitempty" yaml:"match_method,omitempty"`
	StepKey               string         `json:"step_key,omitempty" yaml:"step_key,omitempty"`
	Type                  string         `json:"type" yaml:"type"`
	GroupKey              string         `json:"group_key,omitempty" yaml:"group_key,omitempty"`
	Matrix                any            `json:"matrix,omitempty" yaml:"matrix,omitempty"`
	ParallelIndex         *int           `json:"parallel_index,omitempty" yaml:"parallel_index,omitempty"`
	ParallelTotal         *int           `json:"parallel_total,omitempty" yaml:"parallel_total,omitempty"`
	UnmatchedReason       string         `json:"unmatched_reason,omitempty" yaml:"unmatched_reason,omitempty"`
	Target                *comparisonJob `json:"target,omitempty" yaml:"target,omitempty"`
	Baseline              *comparisonJob `json:"baseline,omitempty" yaml:"baseline,omitempty"`
	ExecutionDeltaSeconds *float64       `json:"execution_delta_seconds,omitempty" yaml:"execution_delta_seconds,omitempty"`
}

func comparisonDuration(start, end *buildkite.Timestamp) *float64 {
	if start == nil || end == nil || end.Before(start.Time) {
		return nil
	}
	seconds := end.Sub(start.Time).Seconds()
	return &seconds
}

func summarizeComparisonJob(job buildkite.Job, buildURL string) *comparisonJob {
	webURL := job.WebURL
	if webURL == "" && buildURL != "" && job.Step != nil && job.Step.ID != "" {
		webURL = fmt.Sprintf("%s/list?sid=%s&tab=output", buildURL, job.Step.ID)
	}
	return &comparisonJob{
		ID: job.ID, Name: job.Name, State: job.State, SoftFailed: job.SoftFailed,
		ExitStatus: job.ExitStatus, SignalReason: job.SignalReason, RetriesCount: job.RetriesCount,
		WebURL: webURL, ExecutionSeconds: comparisonDuration(job.StartedAt, job.FinishedAt),
		SchedulingSeconds: comparisonDuration(job.ScheduledAt, job.StartedAt),
	}
}

// Key and name namespaces are separate. Coordinates retain absent vs zero.
func comparisonIdentity(job buildkite.Job) string {
	method, identifier, group := "step_key", job.StepKey, ""
	if identifier == "" {
		if strings.TrimSpace(job.Name) == "" {
			return ""
		}
		method, identifier, group = "name_fallback", job.Name, job.GroupKey
	}
	identity, err := json.Marshal([]any{method, identifier, group, job.Type, job.Matrix, job.ParallelGroupIndex, job.ParallelGroupTotal})
	if err != nil {
		return ""
	}
	return string(identity)
}

func comparisonFailed(job *comparisonJob) bool {
	return job.State == "failed" || job.State == "timed_out" || job.State == "expired"
}

func compareJobOutcomes(target, baseline *comparisonJob) string {
	switch {
	case baseline == nil:
		return "added"
	case target == nil:
		return "removed"
	case target.SoftFailed != baseline.SoftFailed:
		return "state_changed"
	case comparisonFailed(target) && baseline.State == "passed":
		return "newly_failing"
	case target.State == "passed" && comparisonFailed(baseline):
		return "recovered"
	case comparisonFailed(target) && comparisonFailed(baseline):
		return "still_failing"
	case target.State != baseline.State:
		return "state_changed"
	case target.RetriesCount != baseline.RetriesCount:
		return "retries_changed"
	default:
		return "unchanged"
	}
}

func compareBuildJobs(target, baseline []buildkite.Job, result *buildComparison) {
	targetByKey, baselineByKey := map[string][]buildkite.Job{}, map[string][]buildkite.Job{}
	for _, job := range target {
		key := comparisonIdentity(job)
		targetByKey[key] = append(targetByKey[key], job)
	}
	for _, job := range baseline {
		key := comparisonIdentity(job)
		baselineByKey[key] = append(baselineByKey[key], job)
	}
	usedFallback := false
	appendStep := func(job buildkite.Job, t, b *comparisonJob, reason string) {
		step := stepComparison{
			StepKey: job.StepKey, Type: job.Type, GroupKey: job.GroupKey, Matrix: job.Matrix,
			ParallelIndex: job.ParallelGroupIndex, ParallelTotal: job.ParallelGroupTotal,
			Target: t, Baseline: b, UnmatchedReason: reason, Change: compareJobOutcomes(t, b),
		}
		if reason != "" {
			step.Change = "unmatched"
		}
		if reason == "" && t != nil && b != nil {
			step.MatchMethod = "step_key"
			if job.StepKey == "" {
				step.MatchMethod = "name_fallback"
				usedFallback = true
			}
			if t.ExecutionSeconds != nil && b.ExecutionSeconds != nil {
				delta := *t.ExecutionSeconds - *b.ExecutionSeconds
				step.ExecutionDeltaSeconds = &delta
			}
		}
		result.ChangeCounts[step.Change]++
		result.Steps = append(result.Steps, step)
	}
	for _, job := range target {
		key := comparisonIdentity(job)
		reason := comparisonUnmatchedReason(key, targetByKey, baselineByKey)
		var b *comparisonJob
		if reason == "" && len(baselineByKey[key]) == 1 {
			b = summarizeComparisonJob(baselineByKey[key][0], result.Baseline.WebURL)
		}
		appendStep(job, summarizeComparisonJob(job, result.Target.WebURL), b, reason)
	}
	for _, job := range baseline {
		key := comparisonIdentity(job)
		reason := comparisonUnmatchedReason(key, targetByKey, baselineByKey)
		if reason == "" && len(targetByKey[key]) == 1 {
			continue
		}
		appendStep(job, nil, summarizeComparisonJob(job, result.Baseline.WebURL), reason)
	}
	if usedFallback {
		result.Warnings = append(result.Warnings, "Unkeyed jobs matched by unique exact name, type, group, matrix and parallel coordinates use match_method=name_fallback. These are heuristic matches, not stable identities; renames appear as added/removed.")
	}
	// Outcomes first, then largest execution deltas. Preserve API order for ties.
	priority := map[string]int{"newly_failing": 0, "recovered": 1, "still_failing": 2, "state_changed": 3, "retries_changed": 4, "added": 5, "removed": 6, "unmatched": 7, "unchanged": 8}
	sort.SliceStable(result.Steps, func(i, j int) bool {
		a, b := result.Steps[i], result.Steps[j]
		if priority[a.Change] != priority[b.Change] {
			return priority[a.Change] < priority[b.Change]
		}
		var da, db float64
		if a.ExecutionDeltaSeconds != nil {
			da = *a.ExecutionDeltaSeconds
		}
		if b.ExecutionDeltaSeconds != nil {
			db = *b.ExecutionDeltaSeconds
		}
		return da > db
	})
}

func comparisonUnmatchedReason(key string, target, baseline map[string][]buildkite.Job) string {
	if key == "" {
		return "missing step key and usable name"
	}
	if len(target[key]) > 1 || len(baseline[key]) > 1 {
		return "ambiguous step identity"
	}
	return ""
}
