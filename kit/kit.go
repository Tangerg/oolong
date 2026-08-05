// Package kit is a default appearance for the behaviour in
// [github.com/Tangerg/oolong/headless].
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
package kit
