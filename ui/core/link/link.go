// Package link keeps detection independent of presentation, opening policy and
// the local filesystem. Ambiguous file references require the caller's evidence
// through Detect; text from another machine cannot authorize a local lookup.
package link

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type Kind uint8

const (
	URL Kind = iota
	File
)

func (k Kind) String() string {
	switch k {
	case URL:
		return "url"
	case File:
		return "file"
	default:
		return "Kind(" + strconv.Itoa(int(k)) + ")"
	}
}

type Link struct {
	// Start and End are the byte range of the text that was matched, so a caller can
	// attach the destination to its own representation of that range.
	Start, End int
	Kind       Kind
	// Target is the destination. It is not always the matched text: a bare host is
	// given the scheme it was written without, a quoted path loses its quotes, and a
	// line and column suffix is taken off and reported separately.
	Target string
	// Line and Column are where in a file the reference points, counting from one, or
	// zero when it did not say. Both are always zero for a URL.
	// Decimal positions may have leading zeros. A zero or overflowing token refuses
	// the complete position suffix; the path itself remains a link without a location.
	//
	// "src/main.go:42" is the shape a compiler, a stack trace and a model all use, and
	// a link that opened the file at the top would throw away the useful half of what
	// was written.
	Line, Column int
}

// Links is an ordered, non-overlapping set of links detected in one text.
type Links []Link

func (l Link) Text(s string) string {
	if l.Start < 0 || l.End > len(s) || l.Start > l.End {
		return ""
	}
	return s[l.Start:l.End]
}

// At uses a half-open byte interval so a following character cannot activate the
// preceding link. A linear scan suits the handful of links in an ordinary line.
func (links Links) At(offset int) (Link, bool) {
	for _, link := range links {
		if offset >= link.Start && offset < link.End {
			return link, true
		}
	}
	return Link{}, false
}

const (
	httpsPrefix    = "https://"
	httpPrefix     = "http://"
	wwwPrefix      = "www."
	uriPunctuation = "-.~:/?#[]@!$&'()*+,;=%"
)

var urlPrefixes = [...]string{httpsPrefix, httpPrefix, wwwPrefix}

// Searching only prefixes lets the scanner stop at a quotation boundary without
// rescanning a suffix already consumed by a greedy URI match.
var urlPrefixPattern = func() *regexp.Regexp {
	patterns := make([]string, 0, len(urlPrefixes))
	for _, prefix := range urlPrefixes {
		pattern := regexp.QuoteMeta(prefix)
		if prefix == wwwPrefix {
			pattern = `\b` + pattern
		}
		patterns = append(patterns, pattern)
	}
	return regexp.MustCompile("(?i)" + strings.Join(patterns, "|"))
}()

// trailing is trimmed from the end of a match: the punctuation that almost always
// belongs to the sentence around a link rather than to the link.
const (
	closingDelimiters = ")]}"
	trailing          = ".,;:!?" + closingDelimiters
)

// Detect delegates filesystem evidence to exists so remote text does not trigger
// implicit local I/O. The callback receives relative paths exactly as written;
// URLs, rooted paths and home-relative paths never trigger it. A nil callback
// leaves ambiguous bare filenames out. File names retain their Unicode spelling
// and original byte ranges; quotes admit spaces and punctuation within a name.
// Drive and backslash paths retain their source grammar regardless of the host OS.
func Detect(s string, exists func(path string) bool) Links {
	found := detectURLs(s)
	found = append(found, detectPaths(s, exists != nil)...)
	return inOrder(found, exists)
}

func detectURLs(s string) []Link {
	var found []Link
	scanner := urlScanner{source: s}
	for {
		from, to, delimited, ok := scanner.next()
		if !ok {
			return found
		}
		text := trimTrailing(s[from:to], delimited)
		scheme, ok := prefixOf(text)
		if !ok || len(text) == len(scheme) {
			continue
		}
		found = append(found, Link{Start: from, End: from + len(text), Kind: URL, Target: urlTarget(text, scheme)})
	}
}

// Quotation context must advance with candidate consumption. Finding every
// match first would hide a later URL swallowed by an earlier quoted candidate.
type urlScanner struct {
	source string
	offset int
	quote  byte
}

// next reports whether the candidate is delimited: quoted on both sides, so its
// final punctuation is part of it. A URL inside a quoted sentence is not; the
// sentence's closing punctuation still belongs to the sentence.
func (s *urlScanner) next() (from, to int, delimited, ok bool) {
	match := urlPrefixPattern.FindStringIndex(s.source[s.offset:])
	if match == nil {
		return 0, 0, false, false
	}
	from, to = s.offset+match[0], s.offset+match[1]
	s.quotationBefore(from)
	opened := s.quote != 0 && from > 0 && s.source[from-1] == s.quote
	for to < len(s.source) && uriByte(s.source[to]) {
		if s.quote == '\'' && s.source[to] == '\'' && !wordApostrophe(s.source, to) {
			s.offset, s.quote = to+1, 0
			return from, to, opened, true
		}
		to++
	}
	s.offset = to
	if s.quote == '"' && to < len(s.source) && s.source[to] == '"' {
		s.offset, s.quote = to+1, 0
		return from, to, opened, true
	}
	return from, to, false, true
}

func (s *urlScanner) quotationBefore(until int) {
	for s.offset < until {
		at := s.offset
		s.offset++
		quote := s.source[at]
		if quote != '\'' && quote != '"' || quote == '\'' && wordApostrophe(s.source, at) {
			continue
		}
		if s.quote != 0 {
			if s.quote == quote {
				s.quote = 0
			}
			continue
		}
		// A possessive apostrophe or inch mark cannot open a quotation after a word.
		if at == 0 || !wordByte(s.source[at-1]) {
			s.quote = quote
		}
	}
}

// Word-internal apostrophes do not end a quotation. Within a single-quoted URI,
// other apostrophes delimit it; percent-encoding disambiguates URI data there.
// Without a surrounding quotation every URI apostrophe remains data.
func wordApostrophe(source string, at int) bool {
	return at > 0 && at+1 < len(source) && wordByte(source[at-1]) && wordByte(source[at+1])
}

func wordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_'
}

// ASCII URI syntax keeps adjacent non-Latin prose outside the destination.
func uriByte(b byte) bool {
	return wordByte(b) || strings.IndexByte(uriPunctuation, b) >= 0
}

// urlTarget completes a scheme-less www address; every other URL is its own target.
func urlTarget(text, scheme string) string {
	if scheme == wwwPrefix {
		return httpsPrefix + text
	}
	return text
}

// Overlap is possible because the shapes are looked for separately, and two links over
// the same cells would stamp one target on top of the other — leaving whichever ran
// last, which is not a rule anybody could predict from the text.
func inOrder(found []Link, exists func(string) bool) Links {
	slices.SortStableFunc(found, func(a, b Link) int { return cmp.Compare(a.Start, b.Start) })
	out := found[:0]
	end := 0
	for _, l := range found {
		if l.Start < end {
			continue
		}
		// A filesystem lookup can do I/O. Only a relative file that can
		// actually win its range has anything for the caller to confirm.
		if l.Kind == File && exists != nil && pathAnchorLength(l.Target) == 0 && !exists(l.Target) {
			continue
		}
		// Detection slices targets out of s. The result is the ownership boundary: a
		// caller retaining one short link must not retain the complete document it was
		// found in.
		l.Target = strings.Clone(l.Target)
		out = append(out, l)
		end = l.End
	}
	clear(found[len(out):])
	return Links(out)
}

// Keep a closing delimiter that belongs to the URL, including an IPv6 host's
// final bracket. Count surplus closers once so removing a run of punctuation
// does not repeatedly rescan the entire candidate.
func trimTrailing(text string, delimited bool) string {
	punctuation := trailing
	if delimited {
		punctuation = closingDelimiters
	}
	if text == "" || !strings.ContainsRune(punctuation, rune(text[len(text)-1])) {
		return text
	}
	round := strings.Count(text, ")") - strings.Count(text, "(")
	square := strings.Count(text, "]") - strings.Count(text, "[")
	for len(text) > 0 {
		last := text[len(text)-1]
		if !strings.ContainsRune(punctuation, rune(last)) {
			break
		}
		switch last {
		case ')':
			if round <= 0 {
				return text
			}
			round--
		case ']':
			if square <= 0 {
				return text
			}
			square--
		}
		text = text[:len(text)-1]
	}
	return text
}

func prefixOf(text string) (string, bool) {
	for _, p := range urlPrefixes {
		if len(text) >= len(p) && strings.EqualFold(text[:len(p)], p) {
			return p, true
		}
	}
	return "", false
}
