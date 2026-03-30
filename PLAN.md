# CAPZ Prow E2E Dashboard — Design Plan

## Problem

The current [TestGrid dashboard](https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api-provider-azure) for CAPZ E2E tests is functional but has an outdated UI. It's hard to get a quick overview of what's failing, drill into specific failures, or track flakiness trends. We want a modern, purpose-built frontend that makes it easy for CAPZ maintainers to triage test health.

---

## Data Sources & APIs (Investigation Results)

### 1. Prow Job Configurations (Job Registry / Source of Truth) ✅ Publicly Accessible

All CAPZ prow job definitions live in the upstream [`kubernetes/test-infra`](https://github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/cluster-api-provider-azure) repo. These YAML files are the **authoritative registry** of every job the dashboard needs to track — no hardcoding or scraping needed.

**Config files:**
| File | Contents |
|------|----------|
| `cluster-api-provider-azure-periodics-main.yaml` | 9 periodic jobs for `main` branch (conformance, e2e, AKS, coverage) |
| `cluster-api-provider-azure-periodics-main-upgrades.yaml` | 4 k8s version upgrade jobs (1.31→1.32, 1.32→1.33, etc.) |
| `cluster-api-provider-azure-periodics-v1beta1-release-1.21.yaml` | 4 periodic jobs for `release-1.21` |
| `cluster-api-provider-azure-periodics-v1beta1-release-1.22.yaml` | 5 periodic jobs for `release-1.22` |
| `cluster-api-provider-azure-presubmits-main.yaml` | ~18 presubmit (PR) jobs |
| `cluster-api-provider-azure-presubmits-release-v1beta1.yaml` | Release branch presubmit jobs |
| `cluster-api-provider-azure-presets.yaml` | Shared label presets (Azure creds, etc.) |

**Each job entry contains everything we need:**
```yaml
- name: periodic-cluster-api-provider-azure-e2e-main      # Prow job name → GCS path
  minimum_interval: 48h                                     # Run frequency
  decoration_config:
    timeout: 4h                                             # Max duration
  extra_refs:
    - org: kubernetes-sigs
      repo: cluster-api-provider-azure
      base_ref: main                                        # Branch under test
  annotations:
    testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-azure
    testgrid-tab-name: capz-periodic-e2e-main               # Display name
    description: Runs workload cluster creation E2E tests... # Human description
```

**How this simplifies the data pipeline:**
- **Job discovery is automatic** — parse the YAML files to get the full list of job names, branches, descriptions, testgrid tab names, and run intervals. No need to scrape TestGrid or hardcode job lists.
- **The `name` field maps directly to the GCS artifact path** — `logs/{name}/{build-id}/`
- **Branch, description, and category can all be extracted** from the YAML, enabling automatic grouping in the UI.
- **The data fetcher can pull these YAMLs at startup** via GitHub raw content URLs, so the dashboard automatically picks up new jobs or config changes without code changes.

Raw content URL pattern:
```
https://raw.githubusercontent.com/kubernetes/test-infra/master/config/jobs/kubernetes-sigs/cluster-api-provider-azure/{filename}
```

### 2. GCS Artifacts (Primary Data Source) ✅ Publicly Accessible

Each prow job run stores artifacts in the `kubernetes-ci-logs` GCS bucket. Files are directly fetchable using the job `name` from the config YAML:

```
https://storage.googleapis.com/kubernetes-ci-logs/logs/{job-name}/{build-id}/{file}
```

**Per-build files:**
| File | Contents |
|------|----------|
| `started.json` | `{ timestamp, repos, repo-commit, repo-version }` |
| `finished.json` | `{ timestamp, passed: bool, result: "SUCCESS"/"FAILURE", revision }` |
| `prowjob.json` | Full ProwJob CRD (job config, status, cluster, refs) |
| `build-log.txt` | Raw build log output |
| `artifacts/junit.e2e_suite.1.xml` | JUnit XML with individual test case results |
| `artifacts/clusters/` | Cluster debug logs (CAPI resources, node logs) |

**JUnit XML structure** — the most valuable data:
```xml
<testsuites tests="57" disabled="20" errors="0" failures="9" time="6838.70">
  <testsuite name="capz-e2e">
    <testcase name="[It] Workload cluster creation ..." status="failed" time="4600.00">
      <failure message="Timed out waiting for 3 control plane machines...">
        [FAILED] ...stack trace...
      </failure>
    </testcase>
    <testcase name="[It] Conformance Tests ..." status="skipped" time="0"/>
    <testcase name="[It] ..." status="passed" time="123.45"/>
  </testsuite>
</testsuites>
```

### 3. GCSweb Listings ✅ Publicly Accessible

Build directories can be listed via HTML scraping:
```
https://gcsweb.k8s.io/gcs/kubernetes-ci-logs/logs/{job-name}/
```
Returns HTML with links to each `{build-id}/` directory, sortable by ID (newest last).

### 4. Prow API ⚠️ Works but Large

```
https://prow.k8s.io/prowjobs.js
```
Returns ALL active prowjobs as a single JSON blob. Needs client-side filtering by `spec.job` name. Very large payload (~MBs). **Not ideal for frequent polling.** Best used for getting currently-running job status.

### 5. TestGrid ⚠️ Limited

- The v1 API (`/api/v1/dashboards/...`) returns **404** on the public instance — not operational.
- The dashboard HTML embeds inline JSON with tab names and overall status (PASSING/FLAKY/FAILING), which can be scraped.
- The `/q/summary/` endpoint returns an HTML widget with aggregate counts.
- **Recommendation:** Don't depend on TestGrid API. Use GCS artifacts directly.

### 6. TestGrid Configuration (Reference)

The TestGrid dashboard config lives in [`kubernetes/test-infra`](https://github.com/kubernetes/test-infra/blob/master/config/testgrids/kubernetes/sig-cluster-lifecycle/config.yaml). Most tabs are auto-generated from prow job annotations (`testgrid-dashboards`, `testgrid-tab-name`), so the prow job YAMLs (section 1) are sufficient — we don't need to parse the TestGrid config separately.

---

## Current Test Landscape (57 tabs)

| Status | Count | Examples |
|--------|-------|---------|
| **FAILING** | 6 | `capz-periodic-conformance-k8s-ci-main`, `capz-periodic-conformance-k8s-ci-dra-main` |
| **FLAKY** | 31 | `capz-periodic-capi-e2e-main`, `capz-periodic-e2e-main`, `capz-periodic-e2e-aks-main` |
| **PASSING** | 20 | `capz-periodic-conformance-main`, `capz-periodic-apiversion-upgrade-main` |

**Test categories:**
- **Conformance** — k8s conformance & node conformance on CAPZ clusters
- **CAPI E2E** — Cluster API framework E2E tests (quick-start, self-hosted, MHC, scaling)
- **CAPZ E2E** — Workload cluster creation (HA, Windows, IPv6, dual-stack, private, AzureLinux, CNI v1, VMSS, etc.)
- **AKS E2E** — Managed Kubernetes (AKS) cluster tests
- **Upgrade** — API version upgrade, k8s version upgrade
- **Coverage** — Code coverage reports
- **Scalability** — Azure scalability tests

**Branches tested:** `main`, `release-1.21` (v1beta1), `release-1.22` (v1beta1)

---

## Proposed Architecture

### Option A: Static Site + Backend Fetcher (Recommended)

```
┌─────────────────────────────────────┐
│          React/Next.js SPA          │ ◄── GitHub Pages / Vercel / Netlify
│  (Landing page, job detail, test    │
│   drill-down, flakiness charts)     │
└────────────────┬────────────────────┘
                 │ fetch JSON
                 ▼
┌─────────────────────────────────────┐
│     Lightweight Backend / Worker    │ ◄── GitHub Actions cron / Cloud Function
│  (Polls GCS, parses JUnit XML,     │
│   aggregates results, writes JSON)  │
└────────────────┬────────────────────┘
                 │ reads
                 ▼
┌─────────────────────────────────────┐
│        GCS: kubernetes-ci-logs      │
│  started.json, finished.json,       │
│  junit.e2e_suite.1.xml              │
└─────────────────────────────────────┘
```

**Why a backend fetcher?**
- GCS has no list API without auth; gcsweb returns HTML that needs scraping
- JUnit XML files can be large (200KB+); parsing in-browser is slow
- We want to aggregate historical data (flakiness over time)
- Pre-processed JSON is much smaller and faster to serve

### Option B: Fully Client-Side (Simpler but Limited)

The frontend directly fetches `finished.json` and JUnit XML from GCS for known build IDs. Simpler to deploy but limited to recent builds and slower initial load.

**Recommendation: Start with Option A** — the backend worker is simple (a cron script) and makes the frontend much snappier.

---

## Frontend Pages & Features

### 1. Landing Page — Dashboard Overview

- **Status grid**: All jobs in a card/table layout with color-coded status (pass/fail/flaky)
- **Grouped by category**: Conformance, E2E, AKS, Upgrade, etc.
- **Grouped by branch**: main, release-1.21, release-1.22
- **Summary bar**: X passing, Y flaky, Z failing (like TestGrid's widget but better)
- **Quick filters**: Show only failing, only main branch, etc.
- **Last run time** and **trend arrow** (improving/degrading) for each job
- **Run history dots are clickable**: Each colored dot (pass/fail/flaky) in the sparkline links to that specific run's detail page

### 2. Job Detail Page — `/job/{job-name}`

- **Run history timeline**: Last N runs as colored dots/bars (green/red/yellow). **Each dot/bar is clickable** and links to the corresponding run's test results (loading that run's JUnit data into the test case table below).
- **Pass rate**: percentage over last 7/14/30 days
- **Latest run details**: duration, commit, timestamp
- **Test case breakdown**: Table of all test cases in the JUnit XML
  - Name, status (pass/fail/skip), duration
  - Sortable and filterable
  - **Failed tests float to top** with error summary inline
- **Link to Prow**: Direct link to `prow.k8s.io/view/gs/...` for the full Spyglass view
- **Link to GCS artifacts**: build-log.txt, cluster logs

### 3. Test Case Detail / Failure Investigation — `/job/{job-name}/test/{test-name}`

This is the core error investigation page — it should make root-causing a failure as fast as possible.

- **Error summary banner**: The `<failure message="...">` from JUnit XML, prominently displayed with syntax highlighting. Example:
  ```
  ❌ Timed out after 3600.001s.
  Timed out waiting for 3 control plane machines to exist
  Expected <int>: 2 to equal <int>: 3
  ```
- **Stack trace / failure body**: The full `<failure>` element text, including the Go file:line where it failed (e.g., `framework/controlplane_helpers.go:115`), with a link to the source on GitHub
- **Flakiness chart**: Pass/fail history of this specific test across runs. **Each bar is clickable** — links to that run's full detail page for the parent job, scrolled to this test.
- **Failure pattern detection**: Group similar failures across runs (e.g., "Timed out waiting for 3 control plane machines" appears in 8 of 9 failures — likely the same root cause)
- **Duration trend**: Is this test getting slower over time?
- **Deep-link to artifacts**: One-click links to investigate further (see section below)

### 4. Failure Artifact Deep Links — from any failed test

For each failed test in a run, the dashboard should provide direct deep-links to the relevant debug artifacts stored in GCS. The artifact tree for a CAPZ E2E run is structured as:

```
artifacts/
├── junit.e2e_suite.1.xml              ← Test results (parsed by the data worker)
├── clusters/
│   ├── bootstrap/                     ← Management cluster logs
│   ├── capz-e2e-{id}-ha/             ← Per-workload-cluster artifacts
│   │   ├── azure-activity-logs/       ← Azure ARM activity log (API call audit)
│   │   │   └── capz-e2e-{id}-ha.log
│   │   ├── machines/                  ← Per-machine debug logs
│   │   │   └── {machine-name}/
│   │   │       ├── cloud-init-output.log
│   │   │       ├── cloud-init.log
│   │   │       ├── containerd.log
│   │   │       ├── journal.log
│   │   │       ├── kern.log
│   │   │       ├── kube-apiserver.log
│   │   │       ├── kubelet.log
│   │   │       └── kubelet-version.txt
│   │   ├── kube-system/               ← Pod logs from kube-system namespace
│   │   ├── calico-system/             ← Calico CNI logs
│   │   └── nodes/                     ← Node-level info
│   └── capz-e2e-{id}-ipv6/           ← Another workload cluster...
└── repository/                        ← CAPI provider configs used
    ├── cluster-api/
    ├── infrastructure-azure/
    └── clusterctl-config.yaml
```

**The dashboard should surface these as contextual links from any failed test.** For example, if "[It] Creating a HA cluster" fails, the dashboard knows:
- The cluster name pattern (from the test name → `ha` cluster)
- The build ID (from the run)
- The full GCS path → can link directly to the relevant cluster's `azure-activity-logs/`, `machines/`, and `kube-system/` logs

**Artifact link panel for each failed test:**
| Link | What it shows | When it helps |
|------|---------------|---------------|
| 📋 Build Log | `build-log.txt` — full stdout of the test run | Overall test harness issues, setup failures |
| 📋 Build Log (tail) | Last 100 lines — includes Ginkgo failure summary | Quick scan of what failed and where |
| 🧪 JUnit XML | Raw test results | Programmatic analysis, data export |
| ☁️ Azure Activity Log | ARM API calls and errors | Azure API throttling, resource creation failures, quota issues |
| 🖥️ Machine Logs | `cloud-init.log`, `kubelet.log`, `kube-apiserver.log` per machine | VM bootstrap failures, kubelet crashes, apiserver issues |
| 📦 Pod Logs | `kube-system/`, `calico-system/` pod logs | CNI failures, cloud-provider issues, addon crashes |
| 🔍 Prow Spyglass | Full Spyglass UI at `prow.k8s.io/view/gs/...` | Interactive log viewer with search |

### 5. Flakiness Report — `/flaky`

- **Most flaky tests**: Ranked by flakiness rate (% of runs that flip pass↔fail)
- **Persistent failures**: Tests that have been failing for > N days
- **Recently broken**: Tests that started failing in the last 24-48h

---

## Backend Data Worker

### Approach: GitHub Actions Cron

A scheduled GitHub Actions workflow that runs every 30-60 minutes:

1. **Discover jobs from config (new!)**: Fetch the prow job YAML files from `kubernetes/test-infra` via GitHub raw content URLs. Parse them to extract the job registry: name, branch, description, testgrid tab name, run interval, timeout. This means the dashboard **automatically picks up new/removed jobs** without code changes.
2. **Discover builds**: For each job `name`, scrape the gcsweb listing at `gcsweb.k8s.io/gcs/kubernetes-ci-logs/logs/{name}/` to get recent build IDs
3. **Fetch metadata**: Download `finished.json` and `started.json` for each new build
4. **Parse JUnit XML**: Download and parse `artifacts/junit.e2e_suite.1.xml` — extract test names, statuses, durations, and **full failure messages + stack traces** for failed tests
5. **Discover cluster artifacts for failures**: For failed runs, scrape the `artifacts/clusters/` directory tree via gcsweb to discover per-cluster debug artifacts (azure-activity-logs, machine logs). Map cluster names to failed test cases by matching test name patterns (e.g., "HA cluster" → `*-ha` cluster dir, "ipv6" → `*-ipv6` cluster dir). Generate direct GCS URLs for each artifact file.
6. **Extract failure locations**: Parse the `<failure>` body to extract Go source file:line references (e.g., `framework/controlplane_helpers.go:115`) and generate GitHub source links
7. **Aggregate**: Compute per-job and per-test statistics (pass rates, flakiness, failure pattern grouping)
8. **Write output**: Commit pre-processed JSON files to a `gh-pages` branch or push to a data store

### Output JSON Schema

**`data/dashboard.json`** — landing page data (job metadata comes from prow config YAMLs):
```json
{
  "generated_at": "2026-03-18T23:00:00Z",
  "jobs": [
    {
      "name": "periodic-cluster-api-provider-azure-e2e-main",
      "tab_name": "capz-periodic-e2e-main",
      "category": "e2e",
      "branch": "main",
      "description": "Runs workload cluster creation E2E tests...",
      "minimum_interval": "48h",
      "timeout": "4h",
      "config_file": "cluster-api-provider-azure-periodics-main.yaml",
      "overall_status": "FLAKY",
      "last_run": {
        "build_id": "2034271404769153024",
        "timestamp": "2026-03-18T14:11:12Z",
        "passed": false,
        "duration_seconds": 7671,
        "tests_total": 57,
        "tests_passed": 28,
        "tests_failed": 9,
        "tests_skipped": 20
      },
      "recent_runs": [
        { "build_id": "...", "passed": false, "timestamp": "..." },
        { "build_id": "...", "passed": true, "timestamp": "..." }
      ],
      "pass_rate_7d": 0.42,
      "pass_rate_30d": 0.65
    }
  ]
}
```

**`data/jobs/{job-name}.json`** — per-job detail with full error information:
```json
{
  "name": "periodic-cluster-api-provider-azure-e2e-main",
  "runs": [
    {
      "build_id": "2034271404769153024",
      "started": "2026-03-18T14:11:12Z",
      "finished": "2026-03-18T16:22:31Z",
      "passed": false,
      "commit": "5ad29c78",
      "prow_url": "https://prow.k8s.io/view/gs/kubernetes-ci-logs/logs/periodic-cluster-api-provider-azure-e2e-main/2034271404769153024",
      "build_log_url": "https://storage.googleapis.com/.../build-log.txt",
      "test_cases": [
        {
          "name": "[It] Workload cluster creation Creating a HA cluster [REQUIRED] With 3 control-plane nodes...",
          "status": "failed",
          "duration_seconds": 4600,
          "failure_message": "Timed out after 3600.001s.\nTimed out waiting for 3 control plane machines to exist\nExpected\n    <int>: 2\nto equal\n    <int>: 3",
          "failure_location": "sigs.k8s.io/cluster-api/test@v1.12.3/framework/controlplane_helpers.go:115",
          "failure_location_url": "https://github.com/kubernetes-sigs/cluster-api/blob/v1.12.3/test/framework/controlplane_helpers.go#L115",
          "cluster_artifacts": {
            "cluster_name": "capz-e2e-l4w9sa-ha",
            "azure_activity_log": "https://storage.googleapis.com/.../azure-activity-logs/capz-e2e-l4w9sa-ha.log",
            "machines": [
              {
                "name": "capz-e2e-l4w9sa-ha-control-plane-d67wq",
                "logs": {
                  "kubelet": "https://storage.googleapis.com/.../kubelet.log",
                  "cloud_init": "https://storage.googleapis.com/.../cloud-init-output.log",
                  "kube_apiserver": "https://storage.googleapis.com/.../kube-apiserver.log",
                  "journal": "https://storage.googleapis.com/.../journal.log"
                }
              }
            ]
          }
        },
        {
          "name": "[It] Conformance Tests conformance-tests",
          "status": "skipped",
          "duration_seconds": 0
        }
      ]
    }
  ]
}
```

---

## Tech Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Frontend | **React + TypeScript** (Vite + React Router) | Fast static SPA, no SSR overhead needed for GitHub Pages |
| Styling | **Tailwind CSS v4** | Rapid UI development, consistent design, `@theme` CSS variables |
| Charts | **Recharts** | React-native declarative charting, duration trend charts |
| Icons | **react-icons (Heroicons v2)** | Modern, consistent SVG icons |
| Data fetcher | **Go 1.22+** | Native YAML/XML parsing, familiar to the k8s/CAPZ team, single binary |
| AI analysis | **Claude Sonnet 4.5 + Opus 4.6 via GitHub Copilot API** | CAPZ-aware failure analysis using `api.githubcopilot.com` |
| Notifications | **Slack incoming webhooks** | Persistent failure alerts with AI analysis |
| Scheduling | **GitHub Actions cron** (every 30 min) | Free, no infra to manage, lives in this repo |
| Hosting | **GitHub Pages** | Free, deploys from GitHub Actions |

---

## AI-Powered Failure Analysis

### Goal

Use Claude with CAPZ domain knowledge (from the `debug-capz-k8s` skill) to automatically analyze **every** non-transient test failure with evidence-based investigation.

### How It Works (As Implemented)

#### Two-Tier Analysis
1. **Quick summary** (Sonnet 4.5, all non-transient failures) — 1-2 sentence explanation with structured `is_transient` field, shown inline next to every failed test.
2. **Comprehensive analysis** (Opus 4.6, all non-transient failures) — evidence-based root cause investigation using actual artifact content, shown as expandable panel with severity, suggested fix, and relevant files.

#### Deviation from Original Plan
- **Original**: Deep analysis only for persistent failures (3+ consecutive). **Actual**: Comprehensive analysis for ALL non-transient failures — the user wanted thorough investigation for every failure.
- **Original**: GitHub Models API (`models.github.ai`). **Actual**: GitHub Copilot API (`api.githubcopilot.com`) with `Copilot-Integration-Id: copilot-developer-cli` header, since the Anthropic models weren't available on the Models API.
- **Original**: Simple error message + stack trace sent to AI. **Actual**: Full evidence collection — fetches Machine/AzureMachine YAML status, KCP status, build log error patterns, cloud-init/boot/kubelet logs, and Azure activity logs from GCS artifacts.

#### Known Transient Detection (Local, No API Call)
Patterns detected locally without calling AI:
- HTTP 429 / Azure API throttling
- Quota exceeded
- Context deadline during cleanup
- DNS resolution failures
- ImagePullBackOff
- Disk space exhausted

#### Evidence Collection Pipeline
For each non-transient failure, the fetcher:
1. Fetches build log and extracts error-pattern lines (FAIL, timeout, SkuNotAvailable, etc.)
2. Fetches Machine/AzureMachine/KCP YAML status from `bootstrap/resources/{namespace}/`
3. Fetches machine boot.log, cloud-init-output.log, kubelet.log (first machine with non-empty logs)
4. Fetches Azure activity log excerpt
5. Sends all evidence to Opus 4.6 with the CAPZ domain knowledge system prompt

#### Caching
- **AI file cache** (`ai_cache.json`): keyed on `(test_name, error_hash)`, 30-day expiry
- **Job data cache**: test cases with existing `AISummary` + `AIAnalysis` are skipped entirely
- **CI data cache**: `frontend/public/data/` directory cached between GitHub Actions runs

One-off flakes still get the quick summary (e.g., "Transient Azure API throttling caused VM provisioning timeout") so maintainers can quickly triage without digging into raw logs.

### How It Works

#### Step 1: Classify Failures (in the data worker)

After aggregating test results, classify each failing test:

```
For each test that failed in the latest run:
  - Count consecutive failures with the same/similar error message
  - If failed >= N times consecutively (e.g., N=3):
      → Mark as "persistent failure" → trigger DEEP AI analysis
  - If failed 1-2 times, or alternates pass/fail:
      → Mark as "flaky" or "one-off" → trigger QUICK AI summary only
```

**Similarity matching** for error messages: normalize whitespace, strip timestamps/IDs, then compare. Example — these are the same failure:
```
"Timed out waiting for 3 control plane machines to exist. Expected <int>: 2 to equal <int>: 3"
"Timed out waiting for 3 control plane machines to exist. Expected <int>: 1 to equal <int>: 3"
```

**Known transient errors to auto-ignore** (from CAPZ domain knowledge):
- Azure API throttling (429 errors)
- Temporary resource quota issues
- Intermittent network timeouts during image pulls
- `context deadline exceeded` during cluster deletion/cleanup
- Sporadic Azure DNS resolution failures

#### Step 2: Call Claude Opus 4.6 via GitHub Models API

The GitHub Models API is accessible from GitHub Actions using just `GITHUB_TOKEN` — no separate API keys or Copilot subscription needed at the workflow level.

```yaml
# In the GitHub Actions data worker workflow:
- name: Analyze persistent failures
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: |
    python3 scripts/analyze_failures.py
```

The analysis script sends a structured prompt per failure, at the appropriate depth:

**Quick summary** (all failures — lightweight, ~200 tokens output):
```
POST https://models.github.ai/inference/chat/completions
{
  "model": "anthropic/claude-opus-4-6",
  "messages": [
    { "role": "system", "content": "<CAPZ domain knowledge prompt>" },
    { "role": "user", "content": "Give a brief 1-2 sentence summary of why this CAPZ E2E test failed. Indicate if this looks transient or like a real bug.\n\nTest: [It] Workload cluster creation Creating a HA cluster...\nError: Timed out after 3600.001s. Timed out waiting for 3 control plane machines to exist...\nStack: framework/controlplane_helpers.go:115" }
  ]
}
```

**Deep analysis** (persistent failures — thorough, ~500 tokens output, with more artifact context):
```
POST https://models.github.ai/inference/chat/completions
{
  "model": "anthropic/claude-opus-4-6",
  "messages": [
    { "role": "system", "content": "<CAPZ domain knowledge prompt>" },
    { "role": "user", "content": "Analyze this persistent CAPZ E2E test failure:\n\n
        Test: [It] Workload cluster creation Creating a HA cluster...\n
        Failed 5 consecutive times (last 5 runs)\n\n
        Error: Timed out after 3600.001s. Timed out waiting for 3 control plane machines to exist...\n
        Stack: framework/controlplane_helpers.go:115\n\n
        Build log tail (last 50 lines): ...\n
        Azure activity log excerpt: ...\n
        Machine kubelet.log excerpt (if available): ..." }
  ]
}
```

#### Step 3: System Prompt with CAPZ Domain Knowledge

The system prompt encodes the equivalent of the `debug-capz-k8s` skill's knowledge:

```
You are a CAPZ (Cluster API Provider Azure) E2E test failure analyst.

You have deep knowledge of:
- CAPZ architecture: management clusters, workload clusters, AzureMachines, AzureClusters
- CAPI framework: KubeadmControlPlane, MachineDeployments, MachineHealthChecks
- Azure infrastructure: VMs, VMSS, VNets, NSGs, load balancers, managed identities
- Addon deployment: Calico CNI, cloud-provider-azure, CSI drivers
- Common failure patterns:
  - Control plane machines failing to provision (Azure quota, image not found, cloud-init failures)
  - kubelet not starting (containerd issues, certificate problems)
  - Nodes not joining cluster (networking/NSG misconfig, kube-apiserver unreachable)
  - Timeout waiting for machines (Azure API throttling, slow VM provisioning)

Transient errors to IGNORE (do not flag these as bugs):
- HTTP 429 / throttling from Azure ARM APIs
- Temporary quota exceeded (usually auto-resolves)
- "context deadline exceeded" during cleanup
- Intermittent DNS resolution failures
- Image pull backoff that resolves on retry

For each failure, provide:
1. **Root cause assessment** (1-2 sentences)
2. **Severity**: Critical / High / Medium / Low / Transient-Ignore
3. **Suggested fix or investigation** (specific, actionable) — only for deep analysis
4. **Relevant code/config to check** (file paths in CAPZ or CAPI repos) — only for deep analysis

For quick summaries, respond in 1-2 sentences only. State whether the failure
looks like a transient infrastructure issue or a real bug.
```

#### Step 4: Store and Display Results

AI analysis is stored in the pre-processed JSON at two levels:

**Quick summary** (on every failed test case):
```json
{
  "name": "[It] Workload cluster creation Creating a ipv6 cluster...",
  "status": "failed",
  "consecutive_failures": 1,
  "failure_classification": "flake",
  "ai_summary": {
    "generated_at": "2026-03-18T23:30:00Z",
    "summary": "Transient Azure infrastructure issue — the 3rd control plane VM took too long to provision, likely due to regional capacity constraints. This is a one-off flake, not a code bug.",
    "is_transient": true
  }
}
```

**Deep analysis** (on persistent failures, in addition to the quick summary):
```json
{
  "name": "[It] Workload cluster creation Creating a HA cluster...",
  "status": "failed",
  "consecutive_failures": 5,
  "failure_classification": "persistent",
  "ai_summary": {
    "generated_at": "2026-03-18T23:30:00Z",
    "summary": "Control plane machines consistently fail to reach 3 replicas. The Azure activity log shows SkuNotAvailable errors — this is a persistent infrastructure config issue, not transient.",
    "is_transient": false
  },
  "ai_analysis": {
    "generated_at": "2026-03-18T23:30:00Z",
    "model": "claude-opus-4-6",
    "root_cause": "Control plane machines are timing out because only 2 of 3 VMs are being provisioned. The Azure activity log shows the 3rd VM creation is failing with a SkuNotAvailable error for Standard_D2s_v3 in the target region.",
    "severity": "High",
    "suggested_fix": "Check Azure VM SKU availability in the test region. Update the CAPZ e2e test config to use a different VM size or add a fallback SKU. See templates/cluster-template.yaml for the machine size configuration.",
    "relevant_files": [
      "templates/cluster-template.yaml",
      "test/e2e/azure_test.go",
      "test/e2e/config/azure.yaml"
    ]
  }
}
```

**On the dashboard**, the two tiers are displayed differently:

**Quick summary** — shown inline on every failed test in the job detail table:
```
❌ [It] Creating a ipv6 cluster With ipv6 worker node         4584s
   🤖 Transient Azure infrastructure issue — 3rd control plane VM took
      too long to provision due to regional capacity constraints.
      ℹ️ Likely flake, not a code bug
```

**Deep analysis** — shown as an expandable panel on persistent failures:
```
❌ [It] Creating a HA cluster With 3 control-plane nodes...   4600s
   🤖 Control plane VMs consistently fail to reach 3 replicas. [persistent × 5]

   ▼ 🤖 AI Analysis (Claude Opus 4.6) — Generated 2h ago

   Root Cause: Control plane machines are timing out because only 2 of 3 VMs
   are being provisioned. The Azure activity log shows the 3rd VM creation is
   failing with a SkuNotAvailable error for Standard_D2s_v3.

   Severity: 🔴 High

   Suggested Fix: Check Azure VM SKU availability in the test region. Update
   the CAPZ e2e test config to use a different VM size or add a fallback SKU.

   Files to check:
     • templates/cluster-template.yaml
     • test/e2e/config/azure.yaml
```

### Cost & Rate Limiting Considerations

- **GitHub Models API** has generous free-tier limits for GitHub users with Copilot
- **Quick summaries** run for every failed test — typically ~20-40 failed tests per cron cycle across all jobs. Uses a smaller prompt (~500 tokens in, ~100 tokens out). Can also batch multiple failures into a single API call to reduce round-trips.
- **Deep analysis** only runs for persistent failures (typically 5-15 tests). Uses a larger prompt (~2-4K tokens in, ~500 tokens out) with build log + Azure activity log context.
- All analysis is **cached** — quick summaries keyed on `(test_name, error_message_hash)`, deep analysis keyed on `(test_name, error_pattern_hash, consecutive_count)`. Only re-analyzed when the inputs change.
- Estimated: ~25-50 API calls per cron run total, well within GitHub Models limits
- **Fallback**: If the API is unavailable or rate-limited, the dashboard still works — AI panels simply show "Analysis pending" and the raw error messages are always visible

---

## Notifications for Persistent Failures

### As Implemented

#### Channel: Slack (via Incoming Webhook)
- **Original plan**: Email + Microsoft Teams. **Actual**: Slack only — Teams webhooks were retired by Microsoft (Dec 2025), and Power Automate workflows were blocked by corporate policy. Email was deferred.
- Webhook URL stored as `SLACK_WEBHOOK_URL` repo secret (and env var for local testing)
- Uses Slack Block Kit format with header, fields, code blocks for errors, and action buttons

#### When to Notify
1. **New persistent failure** — test crosses 3+ consecutive failures threshold for the first time
2. **Error hash change** — same test still failing but with a different error (new failure mode)
3. **Recovery** — previously persistent failure starts passing again

#### De-duplication
- `notification_state.json` persisted in the CI data cache
- Keyed on `{jobName}::{testName}` with error hash
- Same error hash still failing → skip (already notified)

#### Notification Content
- Test name, job name, consecutive failure count
- Error message (truncated to 200 chars) in code block
- AI analysis root cause (truncated to 500 chars)
- Clickable buttons: "View on Dashboard" and "View in Prow"

---

## Implementation Phases

### Phase 1 — Data Pipeline & Landing Page ✅
- Go data fetcher: discovers jobs from test-infra YAML via GitHub API, discovers builds via GCS JSON API, parses JUnit XML, outputs pre-processed JSON
- Auto-discovers new release branches (no hardcoded config file list)
- GitHub Actions cron workflow (every 30 min) + GitHub Pages deploy
- Landing page with job status grid, summary bar, sparkline dots, status/branch filters
- **Deviation**: Also built the Job Detail page in Phase 1 (originally Phase 2)

### Phase 2 — Job Detail & Error Investigation ✅
- Job detail page with run history timeline (clickable rectangles)
- Test case table with failures sorted to top, expandable error messages
- Stack trace display with Go file:line highlighting
- Source location links to GitHub repos
- Direct links to Prow Spyglass and GCS build logs

### Phase 3 — Artifact Deep Links & Cluster Debug ✅
- Artifact discovery: scrapes GCSweb to find per-cluster debug dirs
- Cluster-to-test mapping using CAPZ CI template flavor names as source of truth
- Build log parsing to map test names to management cluster namespaces (handles ANSI escape codes + Unicode curly quotes)
- Bootstrap/cluster resources links for tests where workload cluster wasn't created
- Machine log links filtered to only non-empty files; boot.log prioritized
- Pod log directory links to GCSweb listings
- Test detail/investigation page with per-test failure history, pattern grouping

### Phase 4 — Flakiness Analytics ✅
- Flakiness computation: flip rate, fail rate, consecutive failures, error pattern grouping, duration history
- Three categories: most flaky (alternating pass/fail), persistent failures (3+ consecutive), recently broken (within 48h)
- Flakiness report page with tabs, expandable rows, error patterns
- Duration trend charts (Recharts) on test detail and flakiness pages
- Skipped tests excluded from failure classification

### Phase 5 — AI-Powered Failure Analysis ✅
- Evidence-based analysis: fetches Machine/AzureMachine YAML status, build log errors, cloud-init/boot/kubelet logs, Azure activity logs
- Two-tier: Sonnet 4.5 quick summaries + Opus 4.6 comprehensive analysis
- Full CAPZ domain knowledge system prompt (debug-capz-k8s skill)
- Local transient detection skips AI API calls
- Multi-layer caching: AI file cache (30-day TTL) + job data cache + CI data cache
- **Deviation**: Uses GitHub Copilot API (not Models API), comprehensive analysis for ALL non-transient failures (not just persistent)

### Phase 6 — Notifications for Persistent Failures ✅
- Slack incoming webhooks (Block Kit format with action buttons)
- New failure alerts, error hash change alerts, recovery notifications
- De-duplication via notification_state.json
- **Deviation**: Slack instead of Teams (webhooks retired) and email (deferred)

### Phase 7 — Polish & Advanced Features ❌ Not Started
- Search/filter across all tests
- Comparison view (compare two runs side-by-side)
- Mobile-responsive design

---

## Room for Improvement

### Data Pipeline
- **Real-time backend**: Currently static JSON rebuilt every 30 min. A lightweight backend (Cloudflare Workers, Vercel Functions) could serve live data on demand.
- **Incremental fetching in CI**: The data cache works well but initial cold-start fetches are slow (~5 min for 8 builds × 21 jobs with AI). Could parallelize AI analysis across jobs.
- **GCS API pagination for large jobs**: Jobs with 1000+ builds require multiple API pages. Could optimize with `endOffset` based on time-based estimation instead of build ID arithmetic.

### AI Analysis
- **Multi-turn investigation**: Currently sends all evidence in one prompt. Could do iterative investigation — first pass identifies what to look for, second pass reads the specific artifact.
- **AI can't fetch URLs**: The AI receives artifact content pre-fetched by Go. If it could fetch URLs itself (e.g., via function calling), it could follow its own triage path like a human would.
- **Cost tracking**: No visibility into how many API calls or tokens are used per run. Should log token usage.
- **Model selection**: Currently hardcoded to Sonnet 4.5 / Opus 4.6. Could make configurable or auto-select based on failure complexity.
- **Structured output**: The AI sometimes returns prose instead of clean JSON. Could use function calling or stricter output schemas.

### Frontend
- **Mobile responsiveness**: The dashboard works on desktop but hasn't been optimized for mobile. The TestGrid-style grid especially needs responsive handling.
- **Search**: No way to search across all tests/jobs. A search bar with fuzzy matching would help find specific tests quickly.
- **Comparison view**: Comparing two runs side-by-side would help identify what changed between a passing and failing run.
- **Dark/light theme toggle**: Currently dark theme only.
- **Loading states for AI**: When AI analysis hasn't been generated yet, the UI just shows nothing. Could show "Analysis pending" or a skeleton loader.
- **Bundle size**: At 628KB JS (187KB gzipped), the bundle is large due to Recharts and react-icons. Code splitting would help.

### Notifications
- **Email notifications**: Not implemented yet. Would be useful for users who don't use Slack.
- **Teams support**: Power Automate workflows could work if corporate policy allows. The Adaptive Card code exists but was replaced with Slack.
- **Notification preferences**: Currently all-or-nothing. Users might want to subscribe to specific jobs or branches.
- **Digest mode**: Instead of individual alerts, a daily/weekly summary of all persistent failures.

### Testing
- **Integration tests**: No end-to-end tests that verify the full pipeline (fetch → parse → aggregate → AI → notify → output).
- **Frontend tests**: No React component tests or E2E tests (Playwright/Cypress).
- **CI for the dashboard itself**: The Go tests and TypeScript type checking run locally but aren't in a CI workflow for PRs.

### Infrastructure
- **Monitoring**: No alerting if the dashboard itself fails to deploy or the cron stops running.
- **Data retention**: No mechanism to limit how much data accumulates in the cache. Over months, the cache will grow.
- **Secrets rotation**: AI_TOKEN and SLACK_WEBHOOK_URL are static. Should document rotation procedures.

---

## Key URLs Reference

| Resource | URL Pattern |
|----------|-------------|
| **Prow job configs (source of truth)** | `https://github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/cluster-api-provider-azure` |
| Prow config raw content | `https://raw.githubusercontent.com/kubernetes/test-infra/master/config/jobs/kubernetes-sigs/cluster-api-provider-azure/{file}.yaml` |
| TestGrid dashboard | `https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api-provider-azure` |
| TestGrid tab | `https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api-provider-azure#{tab-name}` |
| Prow job view | `https://prow.k8s.io/view/gs/kubernetes-ci-logs/logs/{job-name}/{build-id}` |
| GCS file | `https://storage.googleapis.com/kubernetes-ci-logs/logs/{job-name}/{build-id}/{file}` |
| GCSweb listing | `https://gcsweb.k8s.io/gcs/kubernetes-ci-logs/logs/{job-name}/` |
| CAPZ E2E tests | `https://github.com/kubernetes-sigs/cluster-api-provider-azure/tree/main/test/e2e` |
| GitHub Models API (AI analysis) | `https://models.github.ai/inference/chat/completions` |

---

## Open Questions

1. **How many days of history to retain?** Suggest 30 days (covers ~15-360 runs per job depending on interval).
2. **Should we include presubmit (PR) jobs?** Initially no — focus on periodic jobs that represent `main` and release branch health.
3. **Auth?** None needed — all data sources are public GCS. The dashboard itself would be public.
4. **Hosting preference?** GitHub Pages is free and simple. Vercel/Netlify are alternatives if we want SSR.
