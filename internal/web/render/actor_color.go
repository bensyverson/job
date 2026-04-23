package render

import (
	"fmt"
	"hash/fnv"
	"strings"
	"unicode"
)

// ActorColor returns a deterministic HSL color string for an actor
// name. Mirrors prototype/js/colors.js so server-rendered avatars
// match client-painted ones byte-for-byte: FNV-1a 32-bit hash, hue =
// hash(name) % 360, saturation = hash(name+" sat") % 50 + 50, and a
// fixed lightness of 48%.
//
// Output shape: hsl(<h> <s>% 48%) — suitable for a CSS custom
// property on the avatar element.
func ActorColor(name string) string {
	hue := fnv32a(name) % 360
	sat := fnv32a(name+" sat")%50 + 50
	return fmt.Sprintf("hsl(%d %d%% 48%%)", hue, sat)
}

// LabelColor returns a deterministic HSL color string for a label
// name. Mirrors prototype/js/colors.js: same hue axis as actors but
// desaturated (S 40%, L 50%) so labels read as supporting metadata
// rather than identity.
func LabelColor(name string) string {
	return fmt.Sprintf("hsl(%d 40%% 50%%)", fnv32a(name)%360)
}

// InitialOf returns the uppercase first character of name, ignoring
// surrounding whitespace. Empty or whitespace-only names return "".
// Used for lettered actor avatars at 20px+.
func InitialOf(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	r := []rune(trimmed)[0]
	return string(unicode.ToUpper(r))
}

func fnv32a(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}
