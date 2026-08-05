// Package link finds the URLs in a piece of text.
//
// A terminal that speaks OSC 8 will make a range of cells clickable, and
// [grid.Cell] carries the target for exactly that. What it cannot do is work out
// which part of a line is a URL in the first place — that is this package, and it
// is a surprising amount of care for what looks like one regular expression.
//
// # What is not here
//
// Opening one. Which browser, whether the process is sandboxed, whether a URL
// from tool output should be opened at all — none of that is a terminal library's
// to decide, and a library that decided would be a framework for one program. The
// answer is a byte range and a target; what happens on a click is the caller's.
package link

import (
	"regexp"
	"strings"
)

// Link is one URL found in a piece of text.
type Link struct {
	// Start and End are the byte range of the text that was matched, which is what
	// a caller stamps the target onto with [grid.View.Link].
	Start, End int
	// URL is the target. It is not always the matched text: a bare host is given
	// the scheme it was written without.
	URL string
}

// Text is the matched range of s, which is what was actually written.
func (l Link) Text(s string) string {
	if l.Start < 0 || l.End > len(s) || l.Start > l.End {
		return ""
	}
	return s[l.Start:l.End]
}

// pattern finds http(s) URLs and bare www. hosts in one pass.
//
// One alternation rather than two passes, so "https://www.example.com" is taken by
// the scheme branch and never reported twice; and \b so that a "www." inside a
// longer word — "awww.example" — is not a host.
//
// The body is the RFC 3986 URI character set rather than "anything that is not a
// space". That is the part worth keeping: it stops the match cleanly where non-URL
// text is butted straight against the URL, which is the normal case in Chinese and
// Japanese prose — 见 https://example.com 页面 must not swallow 页面. Brackets and
// parentheses are admitted so that the trimming below can apply a heuristic to
// them, rather than the pattern silently cutting a path short at the first one.
var pattern = regexp.MustCompile(`(?:https?://|\bwww\.)[A-Za-z0-9\-._~:/?#\[\]@!$&()*+,;=%]+`)

// trailing is trimmed from the end of a match: the punctuation that almost always
// belongs to the sentence around a URL rather than to the URL.
const trailing = ".,;:!?)]}"

// Detect finds every URL in s, in the order they appear, as byte ranges into s.
func Detect(s string) []Link {
	var found []Link
	for _, m := range pattern.FindAllStringIndex(s, -1) {
		text := trimTrailing(s[m[0]:m[1]])
		scheme, ok := prefixOf(text)
		if !ok || len(text) == len(scheme) {
			// Nothing left after the scheme once the punctuation went: a bare
			// "https://" at the end of a sentence is not a link.
			continue
		}
		url := text
		if scheme == "www." {
			url = "https://" + url
		}
		found = append(found, Link{Start: m[0], End: m[0] + len(text), URL: url})
	}
	return found
}

// trimTrailing strips the sentence punctuation off the end of a match.
//
// A closing parenthesis is the exception, and it needs a heuristic rather than a
// rule: it is kept while the match still holds an unclosed one, so the path in
// en.wikipedia.org/wiki/Go_(language) survives, and dropped otherwise, so prose
// that wrapped a URL in brackets does not leak one into it.
func trimTrailing(text string) string {
	for len(text) > 0 {
		last := text[len(text)-1]
		if !strings.ContainsRune(trailing, rune(last)) {
			break
		}
		if last == ')' && strings.Count(text, "(") >= strings.Count(text, ")") {
			break
		}
		text = text[:len(text)-1]
	}
	return text
}

// prefixOf is the scheme or host marker the text begins with.
func prefixOf(text string) (string, bool) {
	for _, p := range []string{"https://", "http://", "www."} {
		if strings.HasPrefix(text, p) {
			return p, true
		}
	}
	return "", false
}

// At is the link covering a byte offset, and whether there is one.
//
// Start is inclusive and End exclusive, so a click on the character after a URL is
// not a click on the URL. The scan is linear because a line holds a handful of these
// and an index over them would cost more to build than to skip.
func At(links []Link, offset int) (Link, bool) {
	for _, l := range links {
		if offset >= l.Start && offset < l.End {
			return l, true
		}
	}
	return Link{}, false
}

// Map records where links were drawn, so that a click can be answered.
//
// It is filled while drawing and read when a click arrives. That is the only
// arrangement of these two that cannot fall out of step: the record is produced by
// the pass that drew the cells, so there is nothing to invalidate, no second
// detection over text that may have changed since, and no cache to be wrong.
//
// A [Map] holds screen positions, so it belongs to a frame. Reset it at the start of
// each one — see [Map.Reset] for why that is cheap.
type Map struct{ regions []region }

// region is one run of columns on one row that carries a target.
type region struct {
	y, x, w int
	url     string
}

// Reset empties the map, keeping the space it had. A frame draws roughly what the
// last one did, so the allocation from the first frame serves every frame after.
func (m *Map) Reset() { m.regions = m.regions[:0] }

// Add records that w columns from (x, y) carry url. A run of no width records
// nothing, which is what a link scrolled off the edge comes to.
func (m *Map) Add(x, y, w int, url string) {
	if w <= 0 || url == "" {
		return
	}
	m.regions = append(m.regions, region{y: y, x: x, w: w, url: url})
}

// At is the target at a screen position, and whether there is one.
//
// Later records win, which is what overlapping draws mean: something drawn over a
// link covers it, and a click lands on what is in front.
func (m *Map) At(x, y int) (string, bool) {
	for i := len(m.regions) - 1; i >= 0; i-- {
		if r := m.regions[i]; r.y == y && x >= r.x && x < r.x+r.w {
			return r.url, true
		}
	}
	return "", false
}

// Len is how many runs the map holds, which is what a test asserts on and what a
// caller checks before bothering to hit-test a click at all.
func (m *Map) Len() int { return len(m.regions) }
