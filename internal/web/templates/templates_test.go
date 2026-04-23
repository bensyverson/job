package templates_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bensyverson/jobs/internal/web/assets"
	"github.com/bensyverson/jobs/internal/web/templates"
)

func newEngine(t *testing.T) *templates.Engine {
	t.Helper()
	m, err := assets.BuildManifest()
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	e, err := templates.New(m)
	if err != nil {
		t.Fatalf("templates.New: %v", err)
	}
	return e
}

func renderHome(t *testing.T, e *templates.Engine) string {
	t.Helper()
	var buf bytes.Buffer
	if err := e.Render(&buf, "home", templates.Chrome{ActiveTab: "home"}); err != nil {
		t.Fatalf("Render home: %v", err)
	}
	return buf.String()
}

func TestEngine_RenderHome_EmitsHTMLSkeleton(t *testing.T) {
	out := renderHome(t, newEngine(t))
	mustContain(t, out, "<!doctype html>")
	mustContain(t, out, "<html")
	mustContain(t, out, "</html>")
	mustContain(t, out, "<main")
	mustContain(t, out, "</main>")
	mustContain(t, out, `<div class="page">`)
}

func TestEngine_Render_IncludesHeaderAndFooterPartials(t *testing.T) {
	out := renderHome(t, newEngine(t))
	mustContain(t, out, `class="c-header"`)
	mustContain(t, out, `class="c-footer"`)
	mustContain(t, out, `class="c-tabs"`)
}

func TestEngine_Render_HeaderHasAllFourTabs(t *testing.T) {
	out := renderHome(t, newEngine(t))
	// Tabs point at real routes (vision §6.4), not the prototype's
	// filesystem hrefs.
	for _, href := range []string{"/", "/plan", "/actors", "/log"} {
		needle := `href="` + href + `"`
		if !strings.Contains(out, needle) {
			t.Errorf("header missing tab link %q\n---\n%s", needle, out)
		}
	}
}

func TestEngine_Render_ActiveTabReceivesActiveClass(t *testing.T) {
	out := renderHome(t, newEngine(t))
	// With ActiveTab="home", the Home tab gets c-tab--active.
	if !strings.Contains(out, `href="/" class="c-tab c-tab--active"`) {
		t.Errorf("home tab missing c-tab--active modifier\n---\n%s", out)
	}
	// Other tabs must not be marked active.
	if strings.Contains(out, `href="/plan" class="c-tab c-tab--active"`) {
		t.Errorf("plan tab unexpectedly active on home page")
	}
}

func TestEngine_Render_FooterHasMetricsAndScrubberPill(t *testing.T) {
	out := renderHome(t, newEngine(t))
	mustContain(t, out, `class="c-footer__metric"`)
	mustContain(t, out, `class="c-scrubber-pill"`)
	mustContain(t, out, `class="c-footer__heartbeat"`)
}

func TestEngine_Render_UsesFingerprintedCSSURL(t *testing.T) {
	m, err := assets.BuildManifest()
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	hashed := m.URL("css/components.css")
	if hashed == "" {
		t.Fatal("manifest missing components.css entry")
	}
	e, err := templates.New(m)
	if err != nil {
		t.Fatalf("templates.New: %v", err)
	}
	var buf bytes.Buffer
	if err := e.Render(&buf, "home", templates.Chrome{ActiveTab: "home"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, hashed) {
		t.Errorf("rendered output missing fingerprinted components.css URL %q", hashed)
	}
	// Fingerprint discipline: an unhashed stylesheet link would bypass
	// the immutable cache.
	if strings.Contains(out, `href="/static/css/components.css"`) {
		t.Errorf("rendered output contains unhashed CSS link — must go through the manifest")
	}
}

func TestEngine_Render_UnknownPageErrors(t *testing.T) {
	e := newEngine(t)
	err := e.Render(&bytes.Buffer{}, "no-such-page", templates.Chrome{})
	if err == nil {
		t.Error("Render(unknown): got nil error, want an error naming the page")
	}
}

func mustContain(t *testing.T, out, needle string) {
	t.Helper()
	if !strings.Contains(out, needle) {
		t.Errorf("rendered output missing %q\n---\n%s", needle, out)
	}
}
