package templates_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bensyverson/jobs/internal/web/assets"
	"github.com/bensyverson/jobs/internal/web/render"
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

// homeTemplateData mirrors the shape of handlers.HomePageData just
// closely enough for the home template to render without missing-field
// errors. Duplicated here rather than imported from handlers because
// handlers already depends on templates; reversing that would cycle.
type homeTemplateData struct {
	templates.Chrome
	Activity          homeActivity
	NewlyBlocked      homeNewlyBlocked
	LongestClaim      homeLongestClaim
	OldestTodo        homeOldestTodo
	ActiveClaims      homeActiveClaims
	RecentCompletions homeRecentCompletions
	Blocked           homeBlocked
	Graph             render.MiniGraphView
}

type homeBlocked struct {
	Count int
	Rows  []homeBlockedRow
}

type homeBlockedRow struct {
	TaskShortID, TaskURL, TaskTitle string
	Blockers                        []homeBlockerLink
}

type homeBlockerLink struct {
	ShortID, URL string
}

type homeRecentCompletions struct {
	Count int
	Rows  []homeRecentCompletionRow
}

type homeRecentCompletionRow struct {
	Actor, ActorURL, TaskShortID, TaskURL, TaskTitle, AgeText string
	CompletedAtUnix                                           int64
}

type homeActiveClaims struct {
	Count int
	Rows  []homeActiveClaimRow
}

type homeActiveClaimRow struct {
	Actor, ActorURL, TaskShortID, TaskURL, TaskTitle, DurationText string
	ClaimedAtUnix                                                  int64
}

type homeActivity struct {
	Bars                                                        []homeBar
	TotalDone, TotalClaim, TotalCreate, TotalBlock, TotalEvents int
}

type homeBar struct {
	Empty                      bool
	HeightPercent              int
	Done, Claim, Create, Block int
}

type homeNewlyBlocked struct {
	Count       int
	ProgressPct int
	Items       []homeBlockRef
}

type homeBlockRef struct {
	BlockedShortID, BlockedURL, WaitingOnShortID, WaitingOnURL string
}

type homeLongestClaim struct {
	Present                                          bool
	Actor, ActorURL, TaskShortID, TaskURL, TaskTitle string
	DurationText                                     string
	ProgressPct                                      int
}

type homeOldestTodo struct {
	Present                              bool
	TaskShortID, TaskURL, Title, AgeText string
	ProgressPct                          int
}

func renderHome(t *testing.T, e *templates.Engine) string {
	t.Helper()
	data := homeTemplateData{Chrome: templates.Chrome{ActiveTab: "home"}}
	var buf bytes.Buffer
	if err := e.Render(&buf, "home", data); err != nil {
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
	data := homeTemplateData{Chrome: templates.Chrome{ActiveTab: "home"}}
	if err := e.Render(&buf, "home", data); err != nil {
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

func TestEngine_Render_MountsLiveRegionAndScript(t *testing.T) {
	m, err := assets.BuildManifest()
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	liveURL := m.URL("js/live.js")
	if liveURL == "" {
		t.Fatal("manifest missing js/live.js entry")
	}
	out := renderHome(t, newEngine(t))

	// Element is present so the live-region WebComponent attaches
	// once live.js registers it.
	if !strings.Contains(out, `<live-region src="/events">`) {
		t.Errorf("layout missing <live-region>\n---\n%s", out)
	}
	// Script is loaded with a fingerprinted URL (immutable cache).
	if !strings.Contains(out, liveURL) {
		t.Errorf("layout missing fingerprinted live.js script tag (%s)", liveURL)
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
