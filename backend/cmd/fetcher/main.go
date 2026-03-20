package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/willie-yao/capz-prow-dashboard/backend/internal/aggregator"
	"github.com/willie-yao/capz-prow-dashboard/backend/internal/artifacts"
	"github.com/willie-yao/capz-prow-dashboard/backend/internal/config"
	"github.com/willie-yao/capz-prow-dashboard/backend/internal/gcs"
	"github.com/willie-yao/capz-prow-dashboard/backend/internal/gcsweb"
	"github.com/willie-yao/capz-prow-dashboard/backend/internal/junit"
	"github.com/willie-yao/capz-prow-dashboard/backend/internal/models"
	"github.com/willie-yao/capz-prow-dashboard/backend/internal/output"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	outDir := flag.String("out", "data", "output directory for JSON files")
	buildsPerJob := flag.Int("builds", 10, "number of recent builds to fetch per job")
	workers := flag.Int("workers", 5, "number of concurrent job fetchers")
	timeout := flag.Duration("timeout", 10*time.Minute, "overall fetch timeout")
	periodicOnly := flag.Bool("periodic-only", true, "only fetch periodic jobs (skip presubmits)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client := &http.Client{Timeout: 30 * time.Second}

	// Step 1: Discover jobs from test-infra config YAMLs.
	log.Println("Fetching job configs from test-infra...")
	jobs, err := config.FetchJobConfigs(ctx, client)
	if err != nil {
		return fmt.Errorf("fetching job configs: %w", err)
	}

	if *periodicOnly {
		var periodic []models.ProwJob
		for _, j := range jobs {
			if j.MinimumInterval != "" {
				periodic = append(periodic, j)
			}
		}
		jobs = periodic
	}
	log.Printf("Discovered %d jobs", len(jobs))

	// Step 2: For each job, discover builds and fetch results.
	type jobResult struct {
		job  models.ProwJob
		runs []models.BuildResult
	}

	results := make([]jobResult, len(jobs))
	sem := make(chan struct{}, *workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var fetchErrors []error

	for i, job := range jobs {
		wg.Add(1)
		go func(idx int, j models.ProwJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			runs, err := fetchJobRuns(ctx, client, j.Name, *buildsPerJob)
			if err != nil {
				mu.Lock()
				fetchErrors = append(fetchErrors, fmt.Errorf("job %s: %w", j.Name, err))
				mu.Unlock()
				log.Printf("  ⚠ %s: %v", j.Name, err)
				return
			}

			results[idx] = jobResult{job: j, runs: runs}
			passed := 0
			for _, r := range runs {
				if r.Passed {
					passed++
				}
			}
			log.Printf("  ✓ %s: %d runs (%d passed)", j.Name, len(runs), passed)
		}(i, job)
	}
	wg.Wait()

	if len(fetchErrors) > 0 {
		log.Printf("Warning: %d jobs had fetch errors", len(fetchErrors))
	}

	// Step 3: Aggregate and write output.
	now := time.Now().UTC()
	dashboard := models.Dashboard{
		GeneratedAt: now,
	}
	var details []models.JobDetail

	for _, r := range results {
		if r.job.Name == "" {
			continue // skipped due to fetch error
		}

		summary := aggregator.ComputeJobSummary(r.job, r.runs, now)
		dashboard.Jobs = append(dashboard.Jobs, summary)

		detail := models.JobDetail{
			Name: r.job.Name,
			Runs: r.runs,
		}
		details = append(details, detail)
	}

	log.Printf("Writing output to %s/ (%d jobs)", *outDir, len(dashboard.Jobs))
	if err := output.WriteAll(*outDir, dashboard, details); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	log.Println("Done!")
	return nil
}

// fetchJobRuns discovers recent builds for a job and fetches their results.
func fetchJobRuns(ctx context.Context, client *http.Client, jobName string, count int) ([]models.BuildResult, error) {
	buildIDs, err := gcsweb.ListRecentBuildIDs(ctx, client, jobName, count)
	if err != nil {
		return nil, fmt.Errorf("listing builds: %w", err)
	}

	var runs []models.BuildResult
	for _, bid := range buildIDs {
		result, err := fetchBuildResult(ctx, client, jobName, bid)
		if err != nil {
			log.Printf("    ⚠ %s/%s: %v", jobName, bid, err)
			continue
		}
		runs = append(runs, *result)
	}

	return runs, nil
}

// fetchBuildResult fetches metadata and JUnit XML for a single build.
func fetchBuildResult(ctx context.Context, client *http.Client, jobName, buildID string) (*models.BuildResult, error) {
	info, err := gcs.FetchBuildInfo(ctx, client, jobName, buildID)
	if err != nil {
		return nil, fmt.Errorf("fetching build info: %w", err)
	}

	result := &models.BuildResult{
		BuildInfo: *info,
	}

	// Fetch JUnit XML (best-effort — some builds may not have it).
	junitData, err := gcs.FetchRaw(ctx, client, info.JUnitURL)
	if err != nil {
		// JUnit not available — return result with metadata only.
		return result, nil
	}

	testCases, err := junit.Parse(junitData)
	if err != nil {
		log.Printf("    ⚠ %s/%s: failed to parse JUnit: %v", jobName, buildID, err)
		return result, nil
	}

	result.TestCases = testCases
	for _, tc := range testCases {
		result.TestsTotal++
		switch tc.Status {
		case "passed":
			result.TestsPassed++
		case "failed":
			result.TestsFailed++
		case "skipped":
			result.TestsSkipped++
		}
	}

	// For failed builds, discover per-cluster debug artifacts.
	// Skip if the build is still pending (no finished.json yet).
	if result.Result != "PENDING" && !result.Passed && result.TestsFailed > 0 {
		clusters, err := artifacts.DiscoverClusters(ctx, client, jobName, buildID)
		if err != nil {
			// 404 is expected for jobs that don't produce cluster artifacts (e.g., AKS, conformance).
			// Only log non-404 errors.
			if !strings.Contains(err.Error(), "404") {
				log.Printf("    ⚠ %s/%s: artifact discovery failed: %v", jobName, buildID, err)
			}
		}

		// Fetch build log for namespace mapping (best-effort).
		var namespaceMap map[string]string
		buildLog, err := gcs.FetchRaw(ctx, client, info.BuildLogURL)
		if err != nil {
			log.Printf("    ⚠ %s/%s: failed to fetch build log for namespace mapping: %v", jobName, buildID, err)
		} else {
			namespaceMap = artifacts.ParseNamespaceMap(buildLog)
			log.Printf("    📋 %s/%s: build log %d bytes, %d namespace mappings", jobName, buildID, len(buildLog), len(namespaceMap))
		}

		nsPrefixRe := regexp.MustCompile(`capz-e2e-[a-z0-9]+`)

		for i := range result.TestCases {
			if result.TestCases[i].Status != "failed" {
				continue
			}

			ca := artifacts.MapTestToCluster(result.TestCases[i].Name, clusters)
			if ca != nil {
				// Add bootstrap resources URL by extracting namespace prefix.
				if prefix := nsPrefixRe.FindString(ca.ClusterName); prefix != "" {
					ca.BootstrapResourcesURL = fmt.Sprintf(
						"https://gcsweb.k8s.io/gcs/kubernetes-ci-logs/logs/%s/%s/artifacts/clusters/bootstrap/resources/%s/",
						jobName, buildID, prefix,
					)
				}
				result.TestCases[i].ClusterArtifacts = ca
			} else if namespaceMap != nil {
				// No workload cluster match — try namespace from build log.
				ns := artifacts.FindNamespaceForTest(result.TestCases[i].Name, namespaceMap)
				if ns != "" {
					result.TestCases[i].ClusterArtifacts = &models.ClusterArtifacts{
						ClusterName: ns,
						BootstrapResourcesURL: fmt.Sprintf(
							"https://gcsweb.k8s.io/gcs/kubernetes-ci-logs/logs/%s/%s/artifacts/clusters/bootstrap/resources/%s/",
							jobName, buildID, ns,
						),
					}
				}
			}
		}
	}

	return result, nil
}
