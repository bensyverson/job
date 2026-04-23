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
	// FNV-1a 32-bit is canonical; we lock the output for two seed strings
	// so drift between JS and Go is caught. Reference values are produced
	// by running the prototype's hueFor/satFor in a browser console.
	// If you change the hash or the S/L constants, update these.
	cases := map[string]string{
		"alice":  "hsl(239 67% 48%)",
		"bob":    "hsl(284 70% 48%)",
		"claude": "hsl(303 85% 48%)",
	}
	for name, want := range cases {
		if got := render.ActorColor(name); got != want {
			t.Errorf("ActorColor(%q) = %q, want %q (JS-reference lock)", name, got, want)
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
