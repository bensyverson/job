package render_test

import (
	"regexp"
	"testing"

	"github.com/bensyverson/jobs/internal/web/render"
)

func TestActorColor_Deterministic(t *testing.T) {
	a := render.ActorColor("alice")
	b := render.ActorColor("alice")
	if a != b {
		t.Errorf("ActorColor not deterministic: %q vs %q", a, b)
	}
}

func TestActorColor_HSLShape(t *testing.T) {
	// Per prototype/js/colors.js: hsl(<h> <s>% 48%) with H in 0-359,
	// S in 50-99, L fixed 48.
	got := render.ActorColor("alice")
	re := regexp.MustCompile(`^hsl\((\d{1,3}) (\d{2,3})% 48%\)$`)
	if !re.MatchString(got) {
		t.Errorf("ActorColor(\"alice\") = %q, want hsl(<h> <s>%% 48%%)", got)
	}
}

func TestActorColor_MatchesJSReference(t *testing.T) {
	// FNV-1a 32-bit is canonical; we lock the output for three seed
	// strings so drift between JS and Go is caught. Reference values
	// are produced by running internal/web/assets/js/colors.js's
	// hueFor/satFor in a browser console (or `node -e` against the
	// same hash + seed strings).
	// If you change the hash or the seed strings, update these.
	cases := map[string]string{
		"alice":  "hsl(30 93% 48%)",
		"bob":    "hsl(323 68% 48%)",
		"claude": "hsl(6 55% 48%)",
	}
	for name, want := range cases {
		if got := render.ActorColor(name); got != want {
			t.Errorf("ActorColor(%q) = %q, want %q (JS-reference lock)", name, got, want)
		}
	}
}

func TestLabelColor_MatchesJSReference(t *testing.T) {
	// Same hash + seed for hue as ActorColor, fixed S 40 / L 50.
	cases := map[string]string{
		"alice":  "hsl(30 40% 50%)",
		"bob":    "hsl(323 40% 50%)",
		"claude": "hsl(6 40% 50%)",
	}
	for name, want := range cases {
		if got := render.LabelColor(name); got != want {
			t.Errorf("LabelColor(%q) = %q, want %q (JS-reference lock)", name, got, want)
		}
	}
}

func TestLabelColor_HSLShape(t *testing.T) {
	got := render.LabelColor("migration")
	re := regexp.MustCompile(`^hsl\((\d{1,3}) 40% 50%\)$`)
	if !re.MatchString(got) {
		t.Errorf("LabelColor = %q, want hsl(<h> 40%% 50%%)", got)
	}
}

func TestInitialOf(t *testing.T) {
	cases := map[string]string{
		"alice":    "A",
		"Bob":      "B",
		"  carla ": "C",
		"":         "",
	}
	for in, want := range cases {
		if got := render.InitialOf(in); got != want {
			t.Errorf("InitialOf(%q) = %q, want %q", in, got, want)
		}
	}
}
