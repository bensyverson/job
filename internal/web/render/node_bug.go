package render

import "html/template"

// NodeBug is the small actor avatar overlaid on a claimed graph node.
// Color is typed as template.CSS because it's a known-safe HSL literal
// generated from the actor name; without that marker html/template's
// autoescape collapses it to "ZgotmplZ" when interpolated into a
// style attribute.
type NodeBug struct {
	Actor    string
	ActorURL string
	Letter   string
	Color    template.CSS
	Left     int
	Top      int
}
