package gcsweb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return data
}

func TestListBuildIDs(t *testing.T) {
	fixture := loadFixture(t, "gcsweb_listing.html")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(fixture)
	}))
	defer srv.Close()

	// Override base URL by calling the server directly.
	origBase := GCSWebBaseURL
	ids, err := listBuildIDsFromURL(context.Background(), srv.Client(), srv.URL+"/")
	_ = origBase
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{
		"2035720955698790400",
		"2035358567883448320",
		"2034996180198326272",
		"2034633792370569216",
		"2034271404769153024",
	}

	if len(ids) != len(expected) {
		t.Fatalf("got %d IDs, want %d", len(ids), len(expected))
	}

	for i, id := range ids {
		if id != expected[i] {
			t.Errorf("ids[%d] = %s, want %s", i, id, expected[i])
		}
	}
}

func TestBackLinkExcluded(t *testing.T) {
	fixture := loadFixture(t, "gcsweb_listing.html")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	}))
	defer srv.Close()

	ids, err := listBuildIDsFromURL(context.Background(), srv.Client(), srv.URL+"/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, id := range ids {
		if id == ".." || id == "" {
			t.Errorf("back link should be excluded, got %q", id)
		}
	}
}

func TestSortingNewestFirst(t *testing.T) {
	fixture := loadFixture(t, "gcsweb_listing.html")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	}))
	defer srv.Close()

	ids, err := listBuildIDsFromURL(context.Background(), srv.Client(), srv.URL+"/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 1; i < len(ids); i++ {
		if ids[i] > ids[i-1] {
			t.Errorf("not sorted descending: ids[%d]=%s > ids[%d]=%s", i, ids[i], i-1, ids[i-1])
		}
	}
}

func TestListRecentBuildIDs(t *testing.T) {
	fixture := loadFixture(t, "gcsweb_listing.html")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	}))
	defer srv.Close()

	ids, err := listRecentBuildIDsFromURL(context.Background(), srv.Client(), srv.URL+"/", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ids) != 3 {
		t.Fatalf("got %d IDs, want 3", len(ids))
	}

	// Should be the 3 newest.
	expected := []string{
		"2035720955698790400",
		"2035358567883448320",
		"2034996180198326272",
	}
	for i, id := range ids {
		if id != expected[i] {
			t.Errorf("ids[%d] = %s, want %s", i, id, expected[i])
		}
	}
}

func TestListRecentBuildIDsCountExceedsAvailable(t *testing.T) {
	fixture := loadFixture(t, "gcsweb_listing.html")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture)
	}))
	defer srv.Close()

	ids, err := listRecentBuildIDsFromURL(context.Background(), srv.Client(), srv.URL+"/", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ids) != 5 {
		t.Fatalf("got %d IDs, want 5 (all available)", len(ids))
	}
}

func TestEmptyListing(t *testing.T) {
	emptyHTML := `<!DOCTYPE html><html><body><ul></ul></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(emptyHTML))
	}))
	defer srv.Close()

	ids, err := listBuildIDsFromURL(context.Background(), srv.Client(), srv.URL+"/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ids) != 0 {
		t.Errorf("expected 0 IDs from empty listing, got %d", len(ids))
	}
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := listBuildIDsFromURL(context.Background(), srv.Client(), srv.URL+"/")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

// listBuildIDsFromURL is a test helper that fetches from an arbitrary URL
// (e.g., an httptest server) instead of the hardcoded GCSWebBaseURL.
func listBuildIDsFromURL(ctx context.Context, client *http.Client, url string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{StatusCode: resp.StatusCode, URL: url}
	}

	return parseBuildIDs(resp.Body)
}

// listRecentBuildIDsFromURL is like listBuildIDsFromURL but returns only count results.
func listRecentBuildIDsFromURL(ctx context.Context, client *http.Client, url string, count int) ([]string, error) {
	ids, err := listBuildIDsFromURL(ctx, client, url)
	if err != nil {
		return nil, err
	}
	if count > len(ids) {
		count = len(ids)
	}
	return ids[:count], nil
}

type httpError struct {
	StatusCode int
	URL        string
}

func (e *httpError) Error() string {
	return "unexpected status " + http.StatusText(e.StatusCode) + " for " + e.URL
}
