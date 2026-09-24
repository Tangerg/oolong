// Package kit is a default appearance for the behaviour in
// [github.com/Tangerg/oolong/components/headless].
//
// It is a default and not a destination: every answer here is a matter of taste a
// product eventually disagrees with, and the way out is to stop importing this
// package and keep the one below it. Nothing in headless imports kit.
//
// A widget is dressed by a field, never by a style of its own:
//
//	kit.Transcript{Content: session, Theme: theme, Glyphs: glyphs}
//
// [Theme] names roles rather than colours, so a palette changes in one place and the
// same grey cannot be chosen twice with two values. [Glyphs] is separate because
// which characters a terminal can draw is a fact about the terminal rather than
// taste. Neither has a default — a widget given no theme draws in the terminal's own
// colours and one given no glyphs draws no furniture — because guessing either means
// guessing about a terminal nobody asked.
//
// [Label], [Paragraph] and the label on [Entry] are the exception and take a
// [github.com/Tangerg/oolong/core/grid.Style]: the same words are a heading in one
// place and a warning in another, which no theme can work out.
package kit
