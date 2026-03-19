package config

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/willie-yao/capz-prow-dashboard/backend/internal/models"
)

const baseURL = "https://raw.githubusercontent.com/kubernetes/test-infra/master/config/jobs/kubernetes-sigs/cluster-api-provider-azure/"

// configFiles is the list of CAPZ Prow config YAML files to fetch.
var configFiles = []string{
	"cluster-api-provider-azure-periodics-main.yaml",
	"cluster-api-provider-azure-periodics-main-upgrades.yaml",
	"cluster-api-provider-azure-periodics-v1beta1-release-1.21.yaml",
	"cluster-api-provider-azure-periodics-v1beta1-release-1.22.yaml",
	"cluster-api-provider-azure-presubmits-main.yaml",
	"cluster-api-provider-azure-presubmits-release-v1beta1.yaml",
}

// FetchJobConfigs downloads all known CAPZ config YAMLs from the
// kubernetes/test-infra repository on GitHub and returns the parsed jobs.
func FetchJobConfigs(ctx context.Context, client *http.Client) ([]models.ProwJob, error) {
	var allJobs []models.ProwJob

	for _, file := range configFiles {
		url := baseURL + file

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request for %s: %w", file, err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", file, err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", file, err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetching %s: HTTP %d", file, resp.StatusCode)
		}

		jobs, err := ParseJobConfig(body, file)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", file, err)
		}
		allJobs = append(allJobs, jobs...)
	}

	return allJobs, nil
}
