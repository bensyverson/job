// Package render holds view-agnostic rendering helpers shared across
// handlers: actor color hashing, status-pill data shaping, relative
// time formatting, auto-linkify, and similar. Anything a single
// handler uses alone should stay with that handler; this package is
// for what's used by two or more.
package render
