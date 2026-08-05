// Package link finds the things in a piece of text that point somewhere.
//
// A terminal that speaks OSC 8 will make a range of cells clickable, and
// [grid.Cell] carries the target for exactly that. What it cannot do is work out
// which part of a line points somewhere in the first place — that is this package,
// and it is a surprising amount of care for what looks like two regular expressions.
//
// # Two kinds, because they are two things
//
// A URL and a file path are not one destination with two spellings. A URL is opened
// by a browser and can be handed to a terminal as an OSC 8 target; a file is opened by
// an editor, at a line, and handing the terminal a file:// for it is usually worse
// than saying nothing — see [Link.Hyperlink].
//
// In an agent's output the file is the commoner one. A model saying which file it
// changed, and where, is the thing a reader most wants to click.
//
// # What is not here
//
// Opening one. Which browser, which editor, whether the process is sandboxed, whether
// a path from tool output should be opened at all — none of that is a terminal
// library's to decide, and a library that decided would be a framework for one
// program. The answer is a byte range and a destination; what happens on a click is
// the caller's.
//
// Nor is the filesystem. This package reads text and nothing else, which is why the
// one rule that needs the filesystem takes it as an argument — see [DetectIn].
package link

import (
	"regexp"
	"strconv"
	"strings"
)

// Kind is what a link points at.
type Kind uint8

const (
	// URL is a web address. It is the zero value because it is the kind that needs no
	// confirmation from anywhere: a thing shaped like a URL is a URL.
	URL Kind = iota
	// File is a path on the machine the program is running on.
	File
)

// String names the kind the way a diagnostic would.
func (k Kind) String() string {
	if k == File {
		return "file"
	}
	return "url"
}

// Link is one thing in a piece of text that points somewhere.
type Link struct {
	// Start and End are the byte range of the text that was matched, which is what a
	// caller stamps onto cells with [text.StampLink].
	Start, End int
	// Kind says which sort of destination Target is.
	Kind Kind
	// Target is the destination. It is not always the matched text: a bare host is
	// given the scheme it was written without, a quoted path loses its quotes, and a
	// line and column suffix is taken off and reported separately.
	Target string
	// Line and Column are where in a file the reference points, counting from one, or
	// zero when it did not say. Both are always zero for a URL.
	//
	// "src/main.go:42" is the shape a compiler, a stack trace and a model all use, and
	// a link that opened the file at the top would throw away the useful half of what
	// was written.
	Line, Column int
}

// Text is the matched range of s, which is what was actually written.
func (l Link) Text(s string) string {
	if l.Start < 0 || l.End > len(s) || l.Start > l.End {
		return ""
	}
	return s[l.Start:l.End]
}

// Hyperlink is what to give a terminal as an OSC 8 target, and whether to give it one
// at all.
//
// A URL is given as it is. A relative path is not given at all, and that refusal is the
// interesting half: terminals find paths in their own output, know the directory the
// program is running in, and offer to open one in the editor the user actually uses.
// Wrapping a path in a link takes all of that away and replaces it with a destination
// the terminal will hand to a browser.
//
// An absolute path is the exception. There a file:// target says exactly what it means,
// and a terminal that does nothing with it is no worse off than before.
func (l Link) Hyperlink() (string, bool) {
	if l.Kind == URL {
		return l.Target, true
	}
	if strings.HasPrefix(l.Target, "/") {
		return "file://" + l.Target, true
	}
	return "", false
}

// urlPattern finds http(s) URLs and bare www. hosts in one pass.
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
var urlPattern = regexp.MustCompile(`(?:https?://|\bwww\.)[A-Za-z0-9\-._~:/?#\[\]@!$&()*+,;=%]+`)

// pathPattern finds the shapes a file path is written in, in the order they must be
// tried.
//
//   - Quoted, in single or double quotes, which is how anything with a space in it is
//     written and the only form where a space may appear anywhere. It is what makes
//     "Demo App.app" one path rather than two words.
//   - Rooted, beginning with "/" or "~/", where the leading character is evidence
//     enough on its own. "24/7" and "and/or" have no leading slash.
//   - Relative with a directory part, "a/b.go", where the slash says it is a path and
//     the extension keeps slashed prose out: "TCP/IP" and "and/or" have none.
//
// A bare name with neither — "main.go" — is deliberately absent here, and is found
// only when a caller can confirm it. See [DetectIn].
var pathPattern = regexp.MustCompile(
	`'[^']*\.[A-Za-z0-9]+'` +
		`|"[^"]*\.[A-Za-z0-9]+"` +
		`|~?/[A-Za-z0-9\-._~%@+]+(?:/[A-Za-z0-9\-._~%@+]+)*/?` +
		`|[A-Za-z0-9\-._~%@+]+(?:/[A-Za-z0-9\-._~%@+]+)*/[A-Za-z0-9\-._~%@+]*\.[A-Za-z0-9]{1,8}`,
)

// barePattern finds a name with an extension and no directory part.
//
// It is the shape that cannot be told from prose by looking at it: "node.js" is a file
// in one sentence and a runtime in the next, and "1.2.3" and "U.S.A" are neither. So
// it is only ever reported when something that can see the filesystem says it is there.
var barePattern = regexp.MustCompile(`[A-Za-z0-9\-._~%@+]+\.[A-Za-z0-9]{1,8}`)

// linePattern is a line, and optionally a column, written after a path.
var linePattern = regexp.MustCompile(`^:([0-9]{1,9})(?::([0-9]{1,9}))?`)

// trailing is trimmed from the end of a match: the punctuation that almost always
// belongs to the sentence around a link rather than to the link.
const trailing = ".,;:!?)]}"

// Detect finds every link in s, in the order they appear, as byte ranges into s.
//
// Only what can be recognised from the text alone. A bare filename cannot be — see
// [DetectIn], which is this with the one question this package cannot answer handed in.
func Detect(s string) []Link { return DetectIn(s, nil) }

// DetectIn finds every link in s, asking exists about the shapes that cannot be told
// from prose by looking at them.
//
// exists is given a path exactly as it was written, relative and unexpanded, and
// answers whether there is a file there. A nil exists asks nothing and leaves the
// ambiguous shapes out, which is [Detect] and which is the right answer for text whose
// paths belong to somebody else's machine.
//
// It is an argument rather than a call into the operating system because this package
// reads text. A library that quietly stat'd every word of a model's output would be
// doing something no reader of its documentation had reason to expect.
func DetectIn(s string, exists func(path string) bool) []Link {
	found := detectURLs(s)
	found = append(found, detectPaths(s, exists)...)
	if exists != nil {
		found = append(found, detectBare(s, exists)...)
	}
	return inOrder(found)
}

// detectURLs finds the web addresses.
func detectURLs(s string) []Link {
	var found []Link
	for _, m := range urlPattern.FindAllStringIndex(s, -1) {
		text := trimTrailing(s[m[0]:m[1]])
		scheme, ok := prefixOf(text)
		if !ok || len(text) == len(scheme) {
			// Nothing left after the scheme once the punctuation went: a bare
			// "https://" at the end of a sentence is not a link.
			continue
		}
		target := text
		if scheme == "www." {
			target = "https://" + target
		}
		found = append(found, Link{Start: m[0], End: m[0] + len(text), Target: target})
	}
	return found
}

// detectPaths finds the file paths whose shape is evidence enough on its own.
func detectPaths(s string, exists func(path string) bool) []Link {
	var found []Link
	for _, m := range pathPattern.FindAllStringIndex(s, -1) {
		if within(s, m[0]) || !startsWord(s, m[0]) {
			continue
		}
		l, ok := pathAt(s, m[0], m[1])
		if !ok {
			continue
		}
		// A relative path is confirmed when there is anything to confirm it with. A
		// rooted one is not asked about: it says where it is, and a reference to a
		// file that has not been written yet is still a reference to that file.
		rooted := strings.HasPrefix(l.Target, "/") || strings.HasPrefix(l.Target, "~/")
		if exists != nil && !rooted && !exists(l.Target) {
			continue
		}
		found = append(found, l)
	}
	return found
}

// detectBare finds names with no directory part, which only the filesystem can tell
// from prose.
func detectBare(s string, exists func(path string) bool) []Link {
	var found []Link
	for _, m := range barePattern.FindAllStringIndex(s, -1) {
		if within(s, m[0]) || !startsWord(s, m[0]) {
			continue
		}
		l, ok := pathAt(s, m[0], m[1])
		if !ok || !exists(l.Target) {
			continue
		}
		found = append(found, l)
	}
	return found
}

// pathAt reads one path match: the quotes come off, the sentence punctuation comes
// off, and the line and column suffix comes off and is kept.
func pathAt(s string, from, to int) (Link, bool) {
	text := s[from:to]
	quoted := len(text) >= 2 && (text[0] == '\'' || text[0] == '"') && text[len(text)-1] == text[0]
	if quoted {
		text = text[1 : len(text)-1]
	} else {
		text = trimTrailing(text)
	}
	if text == "" || text == "/" || text == "~/" || text == "~" {
		return Link{}, false
	}

	end := from + len(text)
	if quoted {
		end += 2
	}
	l := Link{Start: from, End: end, Kind: File, Target: text}

	// A line and column written after a path belong to the link and not to the
	// sentence: "src/main.go:42" points at a line, and a reader clicking it means to
	// go there.
	if m := linePattern.FindStringSubmatch(s[end:]); m != nil {
		l.Line, _ = strconv.Atoi(m[1])
		if m[2] != "" {
			l.Column, _ = strconv.Atoi(m[2])
		}
		l.End += len(m[0])
	}
	return l, true
}

// startsWord reports whether a match at this offset begins where a path could begin.
//
// The rooted branch of the pattern matches from a slash, and a slash appears in the
// middle of ordinary prose: "and/or" holds "/or", "24/7" holds "/7". A regular
// expression cannot look backwards, so the check that the match is not the tail of a
// longer word is made here — and it is the same check the bare branch needs, for the
// same reason.
func startsWord(s string, at int) bool {
	if at == 0 {
		return true
	}
	switch b := s[at-1]; {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return false
	case b == '.' || b == '-' || b == '_' || b == '~' || b == '%' || b == '@' || b == '+' || b == '/':
		return false
	default:
		return true
	}
}

// within reports whether an offset falls inside a web address, which is what keeps the
// path of a URL from being reported a second time as a file.
func within(s string, at int) bool {
	for _, m := range urlPattern.FindAllStringIndex(s, -1) {
		if at >= m[0] && at < m[1] {
			return true
		}
	}
	return false
}

// inOrder sorts links by where they start and drops any that overlap one already kept.
//
// Overlap is possible because the shapes are looked for separately, and two links over
// the same cells would stamp one target on top of the other — leaving whichever ran
// last, which is not a rule anybody could predict from the text.
func inOrder(found []Link) []Link {
	for i := 1; i < len(found); i++ {
		for j := i; j > 0 && found[j].Start < found[j-1].Start; j-- {
			found[j-1], found[j] = found[j], found[j-1]
		}
	}
	out := found[:0]
	end := 0
	for _, l := range found {
		if l.Start < end {
			continue
		}
		out = append(out, l)
		end = l.End
	}
	return out
}

// At is the link covering a byte offset, and whether there is one.
//
// Start is inclusive and End exclusive, so a click on the character after a link is
// not a click on the link. The scan is linear because a line holds a handful of these
// and an index over them would cost more to build than to skip.
func At(links []Link, offset int) (Link, bool) {
	for _, l := range links {
		if offset >= l.Start && offset < l.End {
			return l, true
		}
	}
	return Link{}, false
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
