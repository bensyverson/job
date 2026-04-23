package assets_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/bensyverson/jobs/internal/web/assets"
)

// knownAsset is a file we know the scaffold ships; swap it if the
// underlying filename changes.
const knownAsset = "css/components.css"

func TestBuildManifest_CoversKnownAsset(t *testing.T) {
	m, err := assets.BuildManifest()
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	url := m.URL(knownAsset)
	if url == "" {
		t.Fatalf("Manifest.URL(%q): empty — asset not in manifest", knownAsset)
	}
}

func TestManifestURL_ContainsFingerprint(t *testing.T) {
	m, err := assets.BuildManifest()
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	url := m.URL(knownAsset)
	// Shape: /static/css/components.<hex>.css — hex segment before ext.
	re := regexp.MustCompile(`^/static/css/components\.[0-9a-f]{6,}\.css$`)
	if !re.MatchString(url) {
		t.Errorf("URL(%q) = %q, want /static/css/components.<hex>.css", knownAsset, url)
	}
}

func TestManifestURL_DeterministicAcrossBuilds(t *testing.T) {
	m1, err := assets.BuildManifest()
	if err != nil {
		t.Fatal(err)
	}
	m2, err := assets.BuildManifest()
	if err != nil {
		t.Fatal(err)
	}
	if m1.URL(knownAsset) != m2.URL(knownAsset) {
		t.Errorf("fingerprint drift between builds: %q vs %q", m1.URL(knownAsset), m2.URL(knownAsset))
	}
}

func TestManifestURL_UnknownAssetReturnsEmpty(t *testing.T) {
	m, err := assets.BuildManifest()
	if err != nil {
		t.Fatal(err)
	}
	if got := m.URL("does/not/exist.css"); got != "" {
		t.Errorf("URL(unknown) = %q, want empty string", got)
	}
}

func TestHandler_ServesHashedURL_WithImmutableCache(t *testing.T) {
	m, err := assets.BuildManifest()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.StripPrefix("/static/", m.Handler()))
	defer ts.Close()

	resp, err := http.Get(ts.URL + m.URL(knownAsset))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET %s: status %d, want 200", m.URL(knownAsset), resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Errorf("GET %s: empty body", m.URL(knownAsset))
	}
	cc := resp.Header.Get("Cache-Control")
	// Long-lived cache with the immutable directive, per common fingerprint-cache practice.
	if !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=") {
		t.Errorf("Cache-Control = %q, want immutable + max-age=<long>", cc)
	}
}

func TestHandler_UnhashedPath_NotFound(t *testing.T) {
	m, err := assets.BuildManifest()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.StripPrefix("/static/", m.Handler()))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/static/" + knownAsset)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET unhashed /static/%s: status %d, want 404 (only fingerprinted URLs are served)", knownAsset, resp.StatusCode)
	}
}

func TestHandler_WrongHash_NotFound(t *testing.T) {
	m, err := assets.BuildManifest()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.StripPrefix("/static/", m.Handler()))
	defer ts.Close()

	bogus := "/static/css/components.deadbeef.css"
	resp, err := http.Get(ts.URL + bogus)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET %s: status %d, want 404 for stale/wrong fingerprint", bogus, resp.StatusCode)
	}
}
