package link

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	pathSeparators          = `/\`
	unquotedPathPunctuation = pathSeparators + "-._~%@+"
	homePathPrefix          = "~/"
	extendedPathPrefix      = `\\?\`
	devicePathPrefix        = `\\.\`
)

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
		s.offset = from + max(size, pathPrefixLength(s.source[from:]))
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

// Volume and namespace prefixes belong to the token even though their punctuation
// also separates ordinary prose and line references. Source paths need not use the
// host operating system's grammar.
func pathPrefixLength(target string) int {
	for _, prefix := range [...]string{extendedPathPrefix, devicePathPrefix} {
		if strings.HasPrefix(target, prefix) {
			return len(prefix) + drivePrefixLength(target[len(prefix):])
		}
	}
	return drivePrefixLength(target)
}

func drivePrefixLength(target string) int {
	if len(target) < 2 || target[1] != ':' {
		return 0
	}
	letter := target[0]
	if letter >= 'a' && letter <= 'z' || letter >= 'A' && letter <= 'Z' {
		return 2
	}
	return 0
}

func pathAnchorLength(target string) int {
	if strings.HasPrefix(target, homePathPrefix) {
		return len(homePathPrefix)
	}
	volume := drivePrefixLength(target)
	if len(target) > volume && strings.ContainsRune(pathSeparators, rune(target[volume])) {
		return volume + 1
	}
	return 0
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
	if anchor := pathAnchorLength(target); anchor > 0 {
		return strings.Trim(target[anchor:], pathSeparators) != ""
	}
	slash, dot := strings.LastIndexAny(target, pathSeparators), strings.LastIndexByte(target, '.')
	return dot > slash && dot+1 < len(target) && (quoted || slash >= 0 || allowBare)
}
