// Package models defines the shared data types for the CAPZ Prow Dashboard.
package models

import "time"

// ProwJob represents a prow job definition parsed from test-infra YAML configs.
type ProwJob struct {
	Name            string `json:"name" yaml:"name"`
	TabName         string `json:"tab_name"`
	Category        string `json:"category"`
	Branch          string `json:"branch"`
	Description     string `json:"description"`
	MinimumInterval string `json:"minimum_interval" yaml:"minimum_interval"`
	Timeout         string `json:"timeout"`
	ConfigFile      string `json:"config_file"`
}

// BuildInfo represents metadata for a single prow build.
type BuildInfo struct {
	BuildID          string    `json:"build_id"`
	JobName          string    `json:"job_name"`
	Started          time.Time `json:"started"`
	Finished         time.Time `json:"finished"`
	Passed           bool      `json:"passed"`
	Result           string    `json:"result"`
	DurationSeconds  float64   `json:"duration_seconds"`
	Commit           string    `json:"commit"`
	RepoVersion      string    `json:"repo_version,omitempty"`
	ProwURL          string    `json:"prow_url"`
	BuildLogURL      string    `json:"build_log_url"`
	JUnitURL         string    `json:"junit_url,omitempty"`
}

// TestCase represents a single test case from JUnit XML.
type TestCase struct {
	Name             string  `json:"name"`
	Status           string  `json:"status"` // "passed", "failed", "skipped"
	DurationSeconds  float64 `json:"duration_seconds"`
	FailureMessage   string  `json:"failure_message,omitempty"`
	FailureBody      string  `json:"failure_body,omitempty"`
	FailureLocation  string  `json:"failure_location,omitempty"`
	FailureLocURL    string  `json:"failure_location_url,omitempty"`
}

// BuildResult is a complete result for a single build: metadata + test cases.
type BuildResult struct {
	BuildInfo
	TestCases    []TestCase `json:"test_cases"`
	TestsTotal   int        `json:"tests_total"`
	TestsPassed  int        `json:"tests_passed"`
	TestsFailed  int        `json:"tests_failed"`
	TestsSkipped int        `json:"tests_skipped"`
}

// JobSummary represents aggregated data for a job on the landing page.
type JobSummary struct {
	ProwJob
	OverallStatus string      `json:"overall_status"` // "PASSING", "FAILING", "FLAKY"
	LastRun       *RunSummary `json:"last_run,omitempty"`
	RecentRuns    []RunSummary `json:"recent_runs"`
	PassRate7d    float64     `json:"pass_rate_7d"`
	PassRate30d   float64     `json:"pass_rate_30d"`
}

// RunSummary is a compact summary of a single build run.
type RunSummary struct {
	BuildID         string    `json:"build_id"`
	Passed          bool      `json:"passed"`
	Timestamp       time.Time `json:"timestamp"`
	DurationSeconds float64   `json:"duration_seconds,omitempty"`
	TestsTotal      int       `json:"tests_total,omitempty"`
	TestsPassed     int       `json:"tests_passed,omitempty"`
	TestsFailed     int       `json:"tests_failed,omitempty"`
	TestsSkipped    int       `json:"tests_skipped,omitempty"`
}

// Dashboard is the top-level structure for dashboard.json.
type Dashboard struct {
	GeneratedAt time.Time    `json:"generated_at"`
	Jobs        []JobSummary `json:"jobs"`
}

// JobDetail is the per-job detail structure for jobs/{job-name}.json.
type JobDetail struct {
	Name string        `json:"name"`
	Runs []BuildResult `json:"runs"`
}

// FailureClassification indicates the type of failure.
type FailureClassification string

const (
	ClassificationPersistent FailureClassification = "persistent"
	ClassificationFlaky      FailureClassification = "flaky"
	ClassificationOneOff     FailureClassification = "one-off"
)
