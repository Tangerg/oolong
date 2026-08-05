// Package kit is a default appearance for the behaviour in
// [github.com/Tangerg/oolong/components/headless].
//
// It is a default and not a destination. Everything here is an answer to a question
// headless deliberately refuses — what a border is made of, what a spinner looks
// like, which grey is muted text — and every one of those answers is a matter of
// taste that a real product eventually disagrees with. When that happens, the way
// out is to stop importing this package and keep the one below it, not to fight
// this one. Nothing in headless imports kit, so that door is always open.
//
// # What that buys
//
// An interface can be assembled in a few lines and look deliberate, which is what
// makes the library possible to evaluate. A library whose shortest working example
// is two hundred lines of style assignment is one nobody gets far enough into to
// judge.
//
// # The palette
//
// [Theme] names roles rather than colours: a widget asks for the style of a border
// or of muted text, never for a particular grey. That is what lets a whole interface
// change palette in one place, and what stops the same grey from being chosen twice
// with two slightly different values.
//
// A theme is a value that widgets are given, not a global they reach for. A global
// palette cannot be varied per pane, and a test cannot pin one.
//
// # How a widget is dressed
//
// There is one way, and it is a field:
//
//	kit.Transcript{Content: session, Theme: theme, Glyphs: glyphs}
//
// A widget takes a [Theme], and a [Glyphs] as well if it draws furniture — a border,
// a marker, a rule — because which characters a terminal can draw is a fact about the
// terminal and not a matter of taste. Neither has a default: a widget given no theme
// draws in the terminal's own colours, and one given no glyphs draws no furniture,
// because guessing either would be guessing about a terminal nobody has asked.
//
// No widget carries styles of its own. Every part of a widget has a fixed role in a
// look — a frame is a border, a pinned header sits on a surface, the match being
// stepped to is the accent — so a field for each would be a field with one sensible
// value and a hundred ways to be inconsistent. A caller who wants one of them
// different passes a theme with that role changed:
//
//	quiet := theme
//	quiet.Selection = quiet.Sunken
//
// The exception is text. [Label] and [Paragraph] take a [github.com/Tangerg/oolong/core/grid.Style],
// because the same label is a heading in one place and a warning in another, and
// which it is here is the caller's to say and nothing a theme can work out.
package kit
