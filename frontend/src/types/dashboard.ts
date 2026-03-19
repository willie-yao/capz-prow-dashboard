// Types matching the Go backend JSON output

export interface RunSummary {
  build_id: string;
  passed: boolean;
  timestamp: string;
  duration_seconds?: number;
  tests_total?: number;
  tests_passed?: number;
  tests_failed?: number;
  tests_skipped?: number;
}

export interface JobSummary {
  name: string;
  tab_name: string;
  category: string;
  branch: string;
  description: string;
  minimum_interval: string;
  timeout: string;
  config_file: string;
  overall_status: "PASSING" | "FAILING" | "FLAKY";
  last_run: RunSummary | null;
  recent_runs: RunSummary[];
  pass_rate_7d: number;
  pass_rate_30d: number;
}

export interface Dashboard {
  generated_at: string;
  jobs: JobSummary[];
}

export interface TestCase {
  name: string;
  status: "passed" | "failed" | "skipped";
  duration_seconds: number;
  failure_message?: string;
  failure_body?: string;
  failure_location?: string;
  failure_location_url?: string;
}

export interface BuildResult {
  build_id: string;
  job_name: string;
  started: string;
  finished: string;
  passed: boolean;
  result: string;
  duration_seconds: number;
  commit: string;
  repo_version?: string;
  prow_url: string;
  build_log_url: string;
  junit_url?: string;
  test_cases: TestCase[];
  tests_total: number;
  tests_passed: number;
  tests_failed: number;
  tests_skipped: number;
}

export interface JobDetail {
  name: string;
  runs: BuildResult[];
}
