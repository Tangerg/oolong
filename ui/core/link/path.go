package link

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const unquotedPathPunctuation = "/-._~%@+"

var linePattern = regexp.MustCompile(`^:([0-9]{1,9})(?::([0-9]{1,9}))?`)

// Consuming a complete token before classifying it prevents a rejected relative
// path from donating a suffix that looks like a different rooted path or bare name.
type pathScanner struct {
	source string
	offset int
}

func (s *pathScanner) next() (from, to int, quoted, ok bool) {
	for s.offset < len(s.source) {
		from = s.offset
		r, size := utf8.DecodeRuneInString(s.source[from:])
		s.offset += size
		if (r == '\'' || r == '"') && s.quoteStarts(from) {
			if end := strings.IndexRune(s.source[s.offset:], r); end >= 0 {
				s.offset += end + 1
				return from, s.offset, true, true
			}
		}
		if !unquotedPathRune(r) {
			continue
		}
		for s.offset < len(s.source) {
			r, size = utf8.DecodeRuneInString(s.source[s.offset:])
			if !unquotedPathRune(r) {
				break
			}
			s.offset += size
		}
		return from, s.offset, false, true
	}
	return 0, 0, false, false
}

func (s *pathScanner) quoteStarts(at int) bool {
	if at == 0 {
		return true
	}
	previous, _ := utf8.DecodeLastRuneInString(s.source[:at])
	return !unquotedPathRune(previous)
}

func unquotedPathRune(r rune) bool {
	if 0 <= r && r < utf8.RuneSelf {
		return wordByte(byte(r)) || strings.ContainsRune(unquotedPathPunctuation, r)
	}
	return !unicode.IsSpace(r) && !unicode.IsControl(r) &&
		!unicode.In(r, unicode.Ps, unicode.Pe, unicode.Pi, unicode.Pf, unicode.Terminal_Punctuation)
}

func detectPaths(source string, allowBare bool) []Link {
	var found []Link
	scanner := pathScanner{source: source}
	for {
		from, to, quoted, ok := scanner.next()
		if !ok {
			return found
		}
		if candidate, ok := pathAt(source, from, to, quoted, allowBare); ok {
			found = append(found, candidate)
			scanner.offset = max(scanner.offset, candidate.End)
		}
	}
}

func pathAt(source string, from, to int, quoted, allowBare bool) (Link, bool) {
	target := source[from:to]
	if quoted {
		target = target[1 : len(target)-1]
	} else {
		target = trimTrailing(target, false)
		to = from + len(target)
	}
	if _, url := prefixOf(target); url || !pathEvidence(target, quoted, allowBare) {
		return Link{}, false
	}
	result := Link{Start: from, End: to, Kind: File, Target: target}
	if suffix := linePattern.FindStringSubmatch(source[to:]); suffix != nil {
		result.Line, _ = strconv.Atoi(suffix[1])
		if suffix[2] != "" {
			result.Column, _ = strconv.Atoi(suffix[2])
		}
		result.End += len(suffix[0])
	}
	return result, true
}

// Rooted paths supply their own evidence. Relative tokens need an extension to
// exclude slashed prose; an unquoted bare name additionally requires caller evidence.
func pathEvidence(target string, quoted, allowBare bool) bool {
	if rest, rooted := strings.CutPrefix(target, "/"); rooted {
		return strings.Trim(rest, "/") != ""
	}
	if rest, homeRelative := strings.CutPrefix(target, "~/"); homeRelative {
		return strings.Trim(rest, "/") != ""
	}
	slash, dot := strings.LastIndexByte(target, '/'), strings.LastIndexByte(target, '.')
	return dot > slash && dot+1 < len(target) && (quoted || slash >= 0 || allowBare)
}
