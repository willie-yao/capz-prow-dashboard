// Package artifacts discovers per-cluster debug artifact directories for
// failed CAPZ E2E test runs stored in GCS.
package artifacts

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/net/html"

	"github.com/willie-yao/capz-prow-dashboard/backend/internal/models"
)

const (
	// GCSWebBaseURL is the base URL for GCSweb HTML listing pages.
	GCSWebBaseURL = "https://gcsweb.k8s.io/gcs/kubernetes-ci-logs/logs/"
	// GCSBaseURL is the base URL for direct GCS object access.
	GCSBaseURL = "https://storage.googleapis.com/kubernetes-ci-logs/logs/"
)

// knownMachineLogs lists the log file names we look for inside each machine directory.
var knownMachineLogs = []string{
	"cloud-init-output.log",
	"cloud-init.log",
	"kubelet.log",
	"kube-apiserver.log",
	"journal.log",
	"kern.log",
	"containerd.log",
}

// DiscoverClusters fetches the GCSweb listing at .../artifacts/clusters/ for
// the given build, then inspects each cluster subdirectory to build a list of
// ClusterArtifacts (machines, activity logs, pod log dirs).
func DiscoverClusters(ctx context.Context, client *http.Client, jobName, buildID string) ([]models.ClusterArtifacts, error) {
	base := GCSWebBaseURL + jobName + "/" + buildID + "/artifacts/clusters/"
	gcsBase := GCSBaseURL + jobName + "/" + buildID + "/artifacts/clusters/"

	return discoverClustersFromURL(ctx, client, base, gcsBase)
}

// discoverClustersFromURL is the testable core of DiscoverClusters,
// accepting arbitrary base URLs so tests can substitute an httptest server.
func discoverClustersFromURL(ctx context.Context, client *http.Client, listingBaseURL, gcsBaseURL string) ([]models.ClusterArtifacts, error) {
	dirs, err := fetchDirs(ctx, client, listingBaseURL)
	if err != nil {
		return nil, fmt.Errorf("listing clusters: %w", err)
	}

	var clusters []models.ClusterArtifacts
	for _, dir := range dirs {
		if strings.EqualFold(dir, "bootstrap") {
			continue
		}

		ca := models.ClusterArtifacts{ClusterName: dir}

		// Azure activity log: direct GCS URL (not a listing).
		ca.AzureActivityLog = gcsBaseURL + dir + "/azure-activity-logs/" + dir + ".log"

		// Discover machines.
		machinesURL := listingBaseURL + dir + "/machines/"
		machineNames, err := fetchDirs(ctx, client, machinesURL)
		if err == nil {
			for _, mn := range machineNames {
				ma := models.MachineArtifacts{
					Name: mn,
					Logs: make(map[string]string),
				}
				for _, logFile := range knownMachineLogs {
					ma.Logs[logFile] = gcsBaseURL + dir + "/machines/" + mn + "/" + logFile
				}
				ca.Machines = append(ca.Machines, ma)
			}
		}
		// Ignore errors listing machines — the dir may not exist.

		// Discover pod log directories (top-level dirs other than azure-activity-logs and machines).
		topDirs, err := fetchDirs(ctx, client, listingBaseURL+dir+"/")
		if err == nil {
			for _, td := range topDirs {
				lower := strings.ToLower(td)
				if lower == "machines" || lower == "azure-activity-logs" {
					continue
				}
				ca.PodLogDirs = append(ca.PodLogDirs, td)
			}
		}

		clusters = append(clusters, ca)
	}

	return clusters, nil
}

// fetchDirs fetches a GCSweb HTML listing page and returns the directory names found.
func fetchDirs(ctx context.Context, client *http.Client, url string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}

	return parseGCSWebDirs(resp.Body)
}

// parseGCSWebDirs reads GCSweb HTML and extracts directory names from <a> hrefs.
// It accepts any non-empty directory name (href ending with "/") and skips the
// ".." back link by checking both the href and the anchor text content.
func parseGCSWebDirs(r io.Reader) ([]string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	var dirs []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			// Skip if the visible text of this link is ".." (back link).
			if anchorTextIsBackLink(n) {
				goto children
			}
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					if name, ok := extractDirName(attr.Val); ok {
						dirs = append(dirs, name)
					}
				}
			}
		}
	children:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return dirs, nil
}

// anchorTextIsBackLink returns true if the visible text content of an <a>
// element is ".." (the GCSweb parent-directory link).
func anchorTextIsBackLink(n *html.Node) bool {
	var buf strings.Builder
	var collect func(*html.Node)
	collect = func(n *html.Node) {
		if n.Type == html.TextNode {
			buf.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(n)
	return strings.TrimSpace(buf.String()) == ".."
}

// extractDirName extracts a directory name from a GCSweb href. It returns the
// directory name (without trailing slash) and true if the href refers to a
// directory, or ("", false) for back links, files, or empty segments.
func extractDirName(href string) (string, bool) {
	if !strings.HasSuffix(href, "/") {
		return "", false
	}
	href = strings.TrimSuffix(href, "/")
	idx := strings.LastIndex(href, "/")
	segment := href
	if idx >= 0 {
		segment = href[idx+1:]
	}
	if segment == "" || segment == ".." {
		return "", false
	}
	return segment, true
}

// MapTestToCluster attempts to match a test name to a discovered cluster.
// If only one cluster was discovered, it always matches. Otherwise heuristics
// based on keywords in the test name are used.
func MapTestToCluster(testName string, clusters []models.ClusterArtifacts) *models.ClusterArtifacts {
	if len(clusters) == 0 {
		return nil
	}
	if len(clusters) == 1 {
		return &clusters[0]
	}

	lower := strings.ToLower(testName)

	type rule struct {
		testKeywords   []string // any of these in the test name triggers the rule
		clusterKeywords []string // cluster dir must contain any of these
	}

	rules := []rule{
		{testKeywords: []string{"ha"}, clusterKeywords: []string{"ha"}},
		{testKeywords: []string{"ipv6"}, clusterKeywords: []string{"ipv6"}},
		{testKeywords: []string{"dual-stack", "dualstack"}, clusterKeywords: []string{"dual"}},
		{testKeywords: []string{"windows"}, clusterKeywords: []string{"windows", "win"}},
		{testKeywords: []string{"vmss"}, clusterKeywords: []string{"vmss"}},
		{testKeywords: []string{"aks", "managed kubernetes"}, clusterKeywords: []string{"aks"}},
		{testKeywords: []string{"azurelinux", "azure linux"}, clusterKeywords: []string{"azurelinux", "flatcar", "mariner"}},
	}

	for _, r := range rules {
		testMatch := false
		for _, kw := range r.testKeywords {
			if strings.Contains(lower, kw) {
				testMatch = true
				break
			}
		}
		if !testMatch {
			continue
		}
		for i := range clusters {
			clusterLower := strings.ToLower(clusters[i].ClusterName)
			for _, ckw := range r.clusterKeywords {
				if strings.Contains(clusterLower, ckw) {
					return &clusters[i]
				}
			}
		}
	}

	return nil
}
