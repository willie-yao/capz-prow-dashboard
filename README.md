# CAPZ Prow Dashboard

A modern dashboard for [Cluster API Provider Azure (CAPZ)](https://github.com/kubernetes-sigs/cluster-api-provider-azure) prow E2E test results. Provides an alternative to the [TestGrid UI](https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api-provider-azure) with a purpose-built frontend that makes it easy to triage test health, investigate failures, and track flakiness.

## Features

- **Test Health Overview** — All periodic jobs at a glance, grouped by category (CAPZ E2E, AKS E2E, Upgrade, CAPI E2E, Conformance) and filterable by status and branch
- **Job Detail Page** — Run history timeline with clickable squares, test case table with failures sorted to top and expandable error messages
- **Failure Investigation** — Failure messages, stack traces, and direct links to GitHub source locations
- **Auto-Discovery** — Job configs are fetched from the [kubernetes/test-infra](https://github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/cluster-api-provider-azure) repo via GitHub API, so new release branches are picked up automatically
- **Links to Prow & GCS** — Every run links to Prow artifacts and raw build logs

## Architecture

```
┌─────────────────────────────────┐
│   React + TypeScript Frontend   │  ← GitHub Pages
│   (Vite, Tailwind CSS, Recharts)│
└──────────────┬──────────────────┘
               │ fetches static JSON
               ▼
┌─────────────────────────────────┐
│     Go Data Fetcher (cron)      │  ← GitHub Actions (every 30 min)
│  Discovers jobs → scrapes GCS   │
│  → parses JUnit XML → writes   │
│  pre-processed JSON             │
└──────────────┬──────────────────┘
               │ reads
               ▼
┌─────────────────────────────────┐
│    GCS: kubernetes-ci-logs      │  ← Prow build artifacts
│  started.json, finished.json,   │
│  junit.e2e_suite.1.xml          │
└─────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.22+
- Node.js 20+
- npm

### Local Development

```bash
# Fetch fresh test data from GCS
make fetch-data

# Start the dev server
make dev
```

Then open http://localhost:5173/capz-prow-dashboard/

### Other Make Targets

```
make build            Build Go data fetcher binary
make test             Run Go tests
make test-v           Run Go tests (verbose)
make fmt              Format Go code
make lint             Run golangci-lint

make fetch-data       Fetch data from GCS (15 builds/job)
make fetch-data-quick Fetch minimal data (3 builds/job)

make fe-install       Install frontend npm dependencies
make fe-build         Production build of frontend
make fe-check         TypeScript type check

make dist             Full pipeline: build + fetch + frontend
make clean            Remove all build artifacts
make deploy           Trigger GitHub Actions deploy
```

## Project Structure

```
├── backend/
│   ├── cmd/fetcher/          # Main data fetcher entrypoint
│   └── internal/
│       ├── config/           # Prow job YAML config parser
│       ├── gcsweb/           # GCSweb HTML scraper for build discovery
│       ├── gcs/              # GCS artifact fetcher (started/finished JSON)
│       ├── junit/            # JUnit XML parser with failure extraction
│       ├── aggregator/       # Pass rates, status classification
│       ├── output/           # JSON file writer
│       └── models/           # Shared data types
├── frontend/
│   └── src/
│       ├── components/       # Layout, JobCard, Sparkline, RunTimeline, etc.
│       ├── pages/            # DashboardPage, JobDetailPage
│       ├── hooks/            # Data fetching hooks
│       ├── lib/              # Utility functions
│       └── types/            # TypeScript type definitions
├── .github/workflows/
│   └── deploy.yml            # Cron + deploy to GitHub Pages
└── Makefile
```

## Data Sources

| Source | What it provides |
|---|---|
| [Prow job configs](https://github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/cluster-api-provider-azure) | Job names, branches, descriptions, run intervals |
| [GCS artifacts](https://storage.googleapis.com/kubernetes-ci-logs/logs/) | Build metadata (started/finished JSON), JUnit XML test results |
| [GCSweb](https://gcsweb.k8s.io/gcs/kubernetes-ci-logs/logs/) | Build ID discovery via HTML listing pages |

## Roadmap

- [ ] **Phase 2** — Artifact deep links (Azure activity logs, machine logs, pod logs)
- [ ] **Phase 3** — Flakiness analytics and trend charts
- [ ] **Phase 4** — AI-powered failure analysis via Claude Opus 4.6
- [ ] **Phase 5** — Notifications for persistent failures (email + Teams)
- [ ] **Phase 6** — Search, comparison view, mobile polish

## License

[Apache 2.0](LICENSE)