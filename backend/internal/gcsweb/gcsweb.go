// Package gcsweb scrapes GCSweb HTML listing pages to discover build IDs for Prow jobs.
package gcsweb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

const (
	// GCSWebBaseURL is the base URL for GCSweb HTML listing pages.
	GCSWebBaseURL = "https://gcsweb.k8s.io/gcs/kubernetes-ci-logs/logs/"
	// GCSBaseURL is the base URL for direct GCS object access.
	GCSBaseURL = "https://storage.googleapis.com/kubernetes-ci-logs/logs/"
)

// ListBuildIDs fetches the GCSweb listing page for the given job and returns
// all build IDs sorted descending (newest first).
func ListBuildIDs(ctx context.Context, client *http.Client, jobName string) ([]string, error) {
	url := GCSWebBaseURL + jobName + "/"

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

	return parseBuildIDs(resp.Body)
}

// ListRecentBuildIDs returns the most recent count build IDs for the given job,
// sorted descending (newest first).
func ListRecentBuildIDs(ctx context.Context, client *http.Client, jobName string, count int) ([]string, error) {
	ids, err := ListBuildIDs(ctx, client, jobName)
	if err != nil {
		return nil, err
	}
	if count > len(ids) {
		count = len(ids)
	}
	return ids[:count], nil
}

// parseBuildIDs reads HTML from r and extracts numeric build IDs from <a> hrefs.
// Returns IDs sorted descending (newest/largest first).
func parseBuildIDs(r io.Reader) ([]string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	var ids []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					if id, ok := extractBuildID(attr.Val); ok {
						ids = append(ids, id)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Sort descending (newest first). Build IDs are numeric strings of equal
	// length, so lexicographic descending order matches numeric descending.
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))

	return ids, nil
}

// extractBuildID checks whether href ends with a path segment that is purely
// numeric (a build ID). It returns the ID and true if found, or ("", false).
func extractBuildID(href string) (string, bool) {
	href = strings.TrimSuffix(href, "/")
	idx := strings.LastIndex(href, "/")
	segment := href
	if idx >= 0 {
		segment = href[idx+1:]
	}

	if segment == "" || segment == ".." {
		return "", false
	}
	for _, r := range segment {
		if !unicode.IsDigit(r) {
			return "", false
		}
	}
	return segment, true
}
