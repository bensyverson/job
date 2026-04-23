// Package assets owns the dashboard's static CSS, JS, and fonts. All
// assets are embedded at compile time and served from /static/ by the
// web server. No build step, no CDN — the binary is self-contained
// (vision §2, principle 8).
//
// URLs are content-fingerprinted so browsers can cache them
// indefinitely: the [Manifest] computes an 8-hex-character prefix of
// each file's SHA-256 and emits URLs of the form
// /static/<dir>/<name>.<hash>.<ext>. The [Manifest.Handler] refuses
// unhashed paths and paths whose hash does not match the current
// content, so a stale bookmark can never serve a fresh file under a
// cache-forever header.
package assets

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:css all:js all:fonts
var files embed.FS

// FS returns the embedded asset filesystem rooted at the package
// directory, so paths look like "css/dashboard.css". Exposed for tools
// and tests; production serving goes through [Manifest.Handler].
func FS() fs.FS {
	return files
}

// fingerprintLen is the number of hex characters kept from each file's
// SHA-256 hash. 8 is plenty — the birthday-collision surface across the
// few-hundred files a dashboard ships is vanishing small, and shorter
// URLs read cleaner in templates and logs.
const fingerprintLen = 8

// immutableCacheControl is the header served with every fingerprinted
// asset: long max-age plus the immutable directive so browsers skip
// revalidation entirely. Safe because a content change flips the
// fingerprint and produces a different URL.
const immutableCacheControl = "public, max-age=31536000, immutable"

// Manifest maps logical asset paths (e.g. "css/dashboard.css") to their
// fingerprinted URL paths (e.g. "/static/css/dashboard.abc12345.css").
// It also serves those URLs via [Manifest.Handler].
//
// Build once at server startup — the underlying files are embedded and
// never change at runtime, so a fresh hash per request would be waste.
type Manifest struct {
	// byLogical maps "css/dashboard.css" → "/static/css/dashboard.<hash>.css"
	byLogical map[string]string
	// byHashedPath maps "css/dashboard.<hash>.css" → "css/dashboard.css"
	// (i.e. the URL component after /static/ back to the real embedded file).
	byHashedPath map[string]string
}

// BuildManifest walks the embedded asset tree and fingerprints every
// file. Returns an error only if the embed FS is unreadable — which
// would be a build-time bug rather than a runtime failure.
func BuildManifest() (*Manifest, error) {
	m := &Manifest{
		byLogical:    make(map[string]string),
		byHashedPath: make(map[string]string),
	}
	err := fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		content, err := fs.ReadFile(files, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		fingerprint := hex.EncodeToString(sum[:])[:fingerprintLen]

		ext := path.Ext(p)
		stem := strings.TrimSuffix(p, ext)
		hashed := stem + "." + fingerprint + ext

		m.byLogical[p] = "/static/" + hashed
		m.byHashedPath[hashed] = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// URL returns the fingerprinted /static/… URL for a logical asset
// path, or "" if the path is unknown. Callers should treat an empty
// return as a bug (typo in a template, missing file) rather than
// papering over it with a plain path.
func (m *Manifest) URL(logicalPath string) string {
	return m.byLogical[logicalPath]
}

// Handler serves fingerprinted asset URLs with an immutable cache
// header. It is intended to be mounted with http.StripPrefix("/static/", …).
// Unhashed paths and paths whose fingerprint does not match the current
// content both 404 — a stale cache entry can never win against a fresh
// build, and a typo in a template can't silently serve the raw file
// under cache-forever headers.
func (m *Manifest) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// r.URL.Path has the /static/ prefix stripped by the outer mux;
		// what remains is the hashed path, e.g. "css/dashboard.<hash>.css".
		hashedPath := strings.TrimPrefix(r.URL.Path, "/")
		logical, ok := m.byHashedPath[hashedPath]
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(files, logical)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "asset read error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", immutableCacheControl)
		// Zero modtime so clients never see a revalidation-sensitive
		// Last-Modified; cache freshness comes entirely from the URL
		// fingerprint.
		http.ServeContent(w, r, logical, time.Time{}, bytes.NewReader(data))
	})
}
