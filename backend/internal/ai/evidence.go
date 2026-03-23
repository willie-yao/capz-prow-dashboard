package ai

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/willie-yao/capz-prow-dashboard/backend/internal/gcs"
	"github.com/willie-yao/capz-prow-dashboard/backend/internal/models"
)

// Evidence holds all the artifact content gathered for a single test failure.
type Evidence struct {
	TestName         string
	FailureMessage   string
	FailureBody      string
	ClusterFlavor    string
	ConsecutiveCount int

	// Artifact content (fetched from GCS)
	BuildLogErrors    string // Filtered error/failure lines from build log
	MachineYAMLs      string // Machine objects with status fields
	AzureMachineYAMLs string // AzureMachine objects with provisioning status
	KCPStatus         string // KubeadmControlPlane status
	CloudInitLog      string // cloud-init-output.log content
	BootLog           string // boot.log content
	KubeletLog        string // kubelet.log excerpt
	AzureActivityLog  string // Azure activity log excerpt
}

// EvidenceParams provides the URLs and metadata needed to collect evidence.
type EvidenceParams struct {
	TestName         string
	FailureMessage   string
	FailureBody      string
	ClusterFlavor    string
	ConsecutiveCount int

	BuildLogURL           string                  // URL to build-log.txt
	BootstrapResourcesURL string                  // URL to bootstrap/resources/{namespace}/
	ClusterArtifacts      *models.ClusterArtifacts // machine logs, activity log, etc.
}

// Build log error patterns (from CAPZ debugging knowledge).
var buildLogPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)FAIL|FAILED|\[FAIL\]`),
	regexp.MustCompile(`(?i)timed?\s*out|timeout`),
	regexp.MustCompile(`(?i)ImagePullBackOff|ErrImagePull`),
	regexp.MustCompile(`(?i)CrashLoopBackOff`),
	regexp.MustCompile(`(?i)unknown flag`),
	regexp.MustCompile(`(?i)quota|OperationNotAllowed`),
	regexp.MustCompile(`(?i)SkuNotAvailable`),
	regexp.MustCompile(`(?i)not found|no image`),
	regexp.MustCompile(`(?i)FailureMessage|FailureReason`),
}

// errorRe matches "error" but we skip lines containing "no error" in the matching logic.
var errorRe = regexp.MustCompile(`(?i)error`)

var connectionRefusedRe = regexp.MustCompile(`(?i)connection refused`)

// CollectEvidence gathers all available artifact content for a test failure.
// Errors fetching individual artifacts are logged but do not fail the overall collection.
func CollectEvidence(ctx context.Context, client *http.Client, params EvidenceParams) Evidence {
	ev := Evidence{
		TestName:         params.TestName,
		FailureMessage:   params.FailureMessage,
		FailureBody:      params.FailureBody,
		ClusterFlavor:    params.ClusterFlavor,
		ConsecutiveCount: params.ConsecutiveCount,
	}

	// 1. Build log errors
	if params.BuildLogURL != "" {
		ev.BuildLogErrors = collectBuildLogErrors(ctx, client, params.BuildLogURL)
	}

	// 2. Bootstrap resource YAMLs
	if params.BootstrapResourcesURL != "" {
		ev.MachineYAMLs = collectResourceStatus(ctx, client, params.BootstrapResourcesURL+"Machine/", 2000)
		ev.AzureMachineYAMLs = collectResourceStatus(ctx, client, params.BootstrapResourcesURL+"AzureMachine/", 2000)
		ev.KCPStatus = collectResourceStatus(ctx, client, params.BootstrapResourcesURL+"KubeadmControlPlane/", 1500)
	}

	// 3. Machine logs
	if params.ClusterArtifacts != nil && len(params.ClusterArtifacts.Machines) > 0 {
		ev.BootLog, ev.CloudInitLog = collectBootLogs(ctx, client, params.ClusterArtifacts.Machines[0])
		ev.KubeletLog = collectKubeletLog(ctx, client, params.ClusterArtifacts.Machines[0])
	}

	// 4. Azure activity log
	if params.ClusterArtifacts != nil && params.ClusterArtifacts.AzureActivityLog != "" {
		ev.AzureActivityLog = collectActivityLog(ctx, client, params.ClusterArtifacts.AzureActivityLog)
	}

	return ev
}

// collectBuildLogErrors fetches the build log and extracts lines matching error patterns
// with 2 lines of context around each match.
func collectBuildLogErrors(ctx context.Context, client *http.Client, url string) string {
	data, err := gcs.FetchRaw(ctx, client, url)
	if err != nil {
		log.Printf("  ⚠ Evidence: failed to fetch build log: %v", err)
		return ""
	}

	lines := strings.Split(string(data), "\n")

	// Count connection refused occurrences to detect persistent issues.
	connRefusedCount := 0
	for _, line := range lines {
		if connectionRefusedRe.MatchString(line) {
			connRefusedCount++
		}
	}
	includeConnRefused := connRefusedCount >= 5

	// Find matching line indices.
	matchSet := make(map[int]bool)
	noErrorRe := regexp.MustCompile(`(?i)no error`)
	for i, line := range lines {
		for _, pat := range buildLogPatterns {
			if pat.MatchString(line) {
				matchSet[i] = true
				break
			}
		}
		// Match "error" lines but exclude "no error" lines.
		if !matchSet[i] && errorRe.MatchString(line) && !noErrorRe.MatchString(line) {
			matchSet[i] = true
		}
		if !matchSet[i] && includeConnRefused && connectionRefusedRe.MatchString(line) {
			matchSet[i] = true
		}
	}

	// Expand with 2 lines of context and collect unique lines.
	contextSet := make(map[int]bool)
	for idx := range matchSet {
		for c := idx - 2; c <= idx+2; c++ {
			if c >= 0 && c < len(lines) {
				contextSet[c] = true
			}
		}
	}

	// Build output in order, inserting separators between non-contiguous blocks.
	var sb strings.Builder
	prevIdx := -10
	for i := 0; i < len(lines); i++ {
		if !contextSet[i] {
			continue
		}
		if i > prevIdx+1 && sb.Len() > 0 {
			sb.WriteString("---\n")
		}
		sb.WriteString(lines[i])
		sb.WriteByte('\n')
		prevIdx = i

		if sb.Len() > 3000 {
			break
		}
	}

	result := sb.String()
	if len(result) > 3000 {
		result = result[:3000] + "..."
	}
	return result
}

// collectResourceStatus fetches a GCSweb resource directory listing, downloads each
// YAML file, and extracts the status: section from each.
func collectResourceStatus(ctx context.Context, client *http.Client, listingURL string, maxChars int) string {
	yamlURLs, err := fetchYAMLFileLinks(ctx, client, listingURL)
	if err != nil {
		log.Printf("  ⚠ Evidence: failed to list resources at %s: %v", listingURL, err)
		return ""
	}

	var sb strings.Builder
	for _, url := range yamlURLs {
		data, err := gcs.FetchRaw(ctx, client, url)
		if err != nil {
			log.Printf("  ⚠ Evidence: failed to fetch resource YAML %s: %v", url, err)
			continue
		}

		status := extractYAMLStatus(string(data))
		if status == "" {
			continue
		}

		// Extract resource name from URL for labeling.
		name := url
		if idx := strings.LastIndex(url, "/"); idx >= 0 {
			name = url[idx+1:]
		}
		fmt.Fprintf(&sb, "--- %s ---\n%s\n", name, status)

		if sb.Len() > maxChars {
			break
		}
	}

	result := sb.String()
	if len(result) > maxChars {
		result = result[:maxChars] + "..."
	}
	return result
}

// fetchYAMLFileLinks fetches a GCSweb listing page and returns URLs to .yaml files.
func fetchYAMLFileLinks(ctx context.Context, client *http.Client, listingURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listingURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", listingURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, listingURL)
	}

	return parseYAMLLinks(resp.Body)
}

// parseYAMLLinks parses GCSweb HTML and extracts href values ending in .yaml.
func parseYAMLLinks(r io.Reader) ([]string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	var urls []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" && strings.HasSuffix(attr.Val, ".yaml") {
					urls = append(urls, attr.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return urls, nil
}

// extractYAMLStatus extracts the status: section from a Kubernetes resource YAML.
// It returns everything from the first top-level "status:" line to the end of the
// indented block (or end of file).
func extractYAMLStatus(yamlContent string) string {
	lines := strings.Split(yamlContent, "\n")
	var sb strings.Builder
	inStatus := false

	for _, line := range lines {
		if !inStatus {
			if strings.HasPrefix(line, "status:") {
				inStatus = true
				sb.WriteString(line)
				sb.WriteByte('\n')
			}
			continue
		}
		// Still in status block: any line starting with a space/tab is indented under status.
		// A non-empty line that doesn't start with whitespace ends the block.
		if line == "" {
			sb.WriteByte('\n')
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			sb.WriteString(line)
			sb.WriteByte('\n')
		} else {
			break
		}
	}

	return strings.TrimSpace(sb.String())
}

// collectBootLogs fetches boot.log and/or cloud-init-output.log from the first machine.
// Returns (bootLog, cloudInitLog). Takes last 150 lines, capped at 3000 chars.
func collectBootLogs(ctx context.Context, client *http.Client, machine models.MachineArtifacts) (string, string) {
	var bootLog, cloudInitLog string

	if url, ok := machine.Logs["boot.log"]; ok && url != "" {
		bootLog = fetchLogTail(ctx, client, url, 150, 3000)
	}

	if url, ok := machine.Logs["cloud-init-output.log"]; ok && url != "" {
		cloudInitLog = fetchLogTail(ctx, client, url, 150, 3000)
	}

	return bootLog, cloudInitLog
}

// collectKubeletLog fetches kubelet.log from a machine. Takes last 100 lines, capped at 2000 chars.
func collectKubeletLog(ctx context.Context, client *http.Client, machine models.MachineArtifacts) string {
	url, ok := machine.Logs["kubelet.log"]
	if !ok || url == "" {
		return ""
	}
	return fetchLogTail(ctx, client, url, 100, 2000)
}

// collectActivityLog fetches the Azure activity log. Takes last 100 lines, capped at 2000 chars.
func collectActivityLog(ctx context.Context, client *http.Client, url string) string {
	return fetchLogTail(ctx, client, url, 100, 2000)
}

// fetchLogTail fetches a log file and returns the last N lines, capped at maxChars.
func fetchLogTail(ctx context.Context, client *http.Client, url string, lastN int, maxChars int) string {
	data, err := gcs.FetchRaw(ctx, client, url)
	if err != nil {
		log.Printf("  ⚠ Evidence: failed to fetch %s: %v", url, err)
		return ""
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > lastN {
		lines = lines[len(lines)-lastN:]
	}

	result := strings.Join(lines, "\n")
	if len(result) > maxChars {
		result = result[:maxChars] + "..."
	}
	return result
}
