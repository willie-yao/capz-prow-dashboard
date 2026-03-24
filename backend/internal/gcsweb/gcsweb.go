// Package gcsweb uses the GCS JSON API to discover build IDs for Prow jobs.
package gcsweb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode"
)

const (
	// GCSWebBaseURL is the base URL for GCSweb HTML listing pages.
	GCSWebBaseURL = "https://gcsweb.k8s.io/gcs/kubernetes-ci-logs/logs/"
	// GCSBaseURL is the base URL for direct GCS object access.
	GCSBaseURL = "https://storage.googleapis.com/kubernetes-ci-logs/logs/"
	// GCSListAPIURL is the GCS JSON API endpoint for listing objects in the bucket.
	GCSListAPIURL = "https://storage.googleapis.com/storage/v1/b/kubernetes-ci-logs/o"

	gcsBucket = "kubernetes-ci-logs"
	gcsPrefix = "logs/"
)

// gcsListResponse represents the JSON response from the GCS list objects API.
type gcsListResponse struct {
	Prefixes      []string `json:"prefixes"`
	NextPageToken string   `json:"nextPageToken"`
}

// ListBuildIDs uses the GCS JSON API to list all build IDs for the given job,
// sorted descending (newest first). For jobs with many builds, prefer
// ListRecentBuildIDs which is much faster.
func ListBuildIDs(ctx context.Context, client *http.Client, jobName string) ([]string, error) {
	return listAllBuildIDs(ctx, client, GCSListAPIURL, jobName)
}

// ListRecentBuildIDs returns the most recent count build IDs for the given job,
// sorted descending (newest first). It uses latest-build.txt to find the newest
// build and then fetches only a small window of recent builds.
func ListRecentBuildIDs(ctx context.Context, client *http.Client, jobName string, count int) ([]string, error) {
	return listRecentBuildIDs(ctx, client, GCSBaseURL, GCSListAPIURL, jobName, count)
}

func listRecentBuildIDs(ctx context.Context, client *http.Client, gcsBase, apiURL, jobName string, count int) ([]string, error) {
	// First try: get latest-build.txt to find the newest build ID.
	latestURL := gcsBase + jobName + "/latest-build.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		latestID := strings.TrimSpace(string(body))

		if isNumeric(latestID) {
			// List builds using startOffset to skip to recent ones.
			// Subtract enough from the latest ID to get a window.
			// Build IDs are ~19 digits. We request more than needed to be safe.
			ids, err := listBuildIDsWithOffset(ctx, client, apiURL, jobName, latestID, count*3)
			if err == nil && len(ids) > 0 {
				if count > len(ids) {
					count = len(ids)
				}
				return ids[:count], nil
			}
		}
	} else if resp != nil {
		resp.Body.Close()
	}

	// Fallback: list all (slow for jobs with many builds).
	ids, err := listAllBuildIDs(ctx, client, apiURL, jobName)
	if err != nil {
		return nil, err
	}
	if count > len(ids) {
		count = len(ids)
	}
	return ids[:count], nil
}

// listBuildIDsWithOffset fetches build IDs near the latest one by using
// endOffset to limit the listing from above and fetching the last page.
func listBuildIDsWithOffset(ctx context.Context, client *http.Client, apiURL, jobName, latestID string, maxResults int) ([]string, error) {
	prefix := gcsPrefix + jobName + "/"

	// To get the LATEST builds efficiently, we list with a high startOffset
	// that's just before the oldest build we want. Since build IDs are
	// monotonically increasing snowflake IDs, we can estimate.
	// We'll just ask for maxResults and rely on GCS returning from the end.
	// Actually GCS always returns from the start, so we need a different approach:
	// List ALL prefixes but only the last page. Use a high startOffset.

	// Strategy: subtract a large amount from the latest ID to create a startOffset
	// that skips most old builds. Build IDs increase by ~70-80 billion per day.
	// For 30 days of builds, offset by ~30 * 80B = 2.4T.
	latestNum := int64(0)
	fmt.Sscanf(latestID, "%d", &latestNum)
	startOffset := latestNum - 3_000_000_000_000_000 // ~40 days window
	if startOffset < 0 {
		startOffset = 0
	}

	startOffsetPrefix := fmt.Sprintf("%s%d", prefix, startOffset)

	params := url.Values{
		"prefix":      {prefix},
		"delimiter":   {"/"},
		"maxResults":  {fmt.Sprintf("%d", maxResults)},
		"startOffset": {startOffsetPrefix},
	}

	var allIDs []string
	pageToken := ""

	for {
		u := apiURL + "?" + params.Encode()
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GCS API HTTP %d", resp.StatusCode)
		}
		var result gcsListResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		for _, p := range result.Prefixes {
			if id := extractBuildID(p); id != "" {
				allIDs = append(allIDs, id)
			}
		}

		if result.NextPageToken == "" {
			break
		}
		pageToken = result.NextPageToken
	}

	sort.Sort(sort.Reverse(sort.StringSlice(allIDs)))
	return allIDs, nil
}

// listAllBuildIDs paginates through all build IDs (slow for large jobs).
func listAllBuildIDs(ctx context.Context, client *http.Client, apiURL, jobName string) ([]string, error) {
	prefix := gcsPrefix + jobName + "/"
	var allIDs []string
	pageToken := ""

	for {
		params := url.Values{
			"prefix":    {prefix},
			"delimiter": {"/"},
			"maxResults": {"1000"},
		}
		u := apiURL + "?" + params.Encode()
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", u, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, u)
		}
		var result gcsListResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding GCS response: %w", err)
		}
		resp.Body.Close()

		for _, p := range result.Prefixes {
			if id := extractBuildID(p); id != "" {
				allIDs = append(allIDs, id)
			}
		}

		if result.NextPageToken == "" {
			break
		}
		pageToken = result.NextPageToken
	}

	sort.Sort(sort.Reverse(sort.StringSlice(allIDs)))
	return allIDs, nil
}

func extractBuildID(prefix string) string {
	prefix = strings.TrimSuffix(prefix, "/")
	idx := strings.LastIndex(prefix, "/")
	segment := prefix
	if idx >= 0 {
		segment = prefix[idx+1:]
	}
	if isNumeric(segment) {
		return segment
	}
	return ""
}

// isNumeric returns true if s is non-empty and contains only digits.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
