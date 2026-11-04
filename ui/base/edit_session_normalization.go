package base

import (
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/text"
)

func maskable(mask string) bool {
	return text.Printable(mask) == mask &&
		!strings.Contains(mask, "\t") &&
		text.Width(mask) > 0 &&
		clusters(mask+mask) == 2*clusters(mask)
}

func clusters(s string) int {
	n := 0
	for range text.Clusters(s) {
		n++
	}
	return n
}

func commonRunePrefix(a, b string) int {
	at := 0
	for at < len(a) && at < len(b) {
		ra, sa := utf8.DecodeRuneInString(a[at:])
		rb, sb := utf8.DecodeRuneInString(b[at:])
		if ra != rb || sa != sb {
			break
		}
		at += sa
	}
	return at
}

func commonRuneSuffixStarts(a, b string, prefix int) (int, int) {
	aEnd, bEnd := len(a), len(b)
	for aEnd > prefix && bEnd > prefix {
		ra, sa := utf8.DecodeLastRuneInString(a[:aEnd])
		rb, sb := utf8.DecodeLastRuneInString(b[:bEnd])
		if ra != rb || sa != sb || aEnd-sa < prefix || bEnd-sb < prefix {
			break
		}
		aEnd, bEnd = aEnd-sa, bEnd-sb
	}
	return aEnd, bEnd
}

func completeClusters(line string, start, end int) (int, int) {
	empty := start == end
	for at, cluster := range text.Clusters(line) {
		after := at + len(cluster)
		if start > at && start < after {
			start = at
		}
		if end > at && end < after {
			end = after
		}
	}
	if empty {
		end = start
	}
	return start, end
}

// Pure projections share one-line normalization without constructing or mutating an Editor.
func oneLineText(s string) string { return text.Printable(flattenLines(s)) }

func flattenLines(s string) string {
	if !strings.ContainsAny(s, "\n\r") {
		return s
	}
	return flatLineBreaks.Replace(s)
}

// Immutable replacers are shared safely without allocating one for each edit.
var (
	logicalLineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n")
	flatLineBreaks    = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")
)

func (s *EditSession) canonicalText(value string) string {
	if s.oneLine() {
		return oneLineText(value)
	}
	if strings.Contains(value, "\r") {
		value = logicalLineBreaks.Replace(value)
	}
	if !strings.Contains(value, "\n") {
		return text.Printable(value)
	}
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = text.Printable(lines[i])
	}
	return strings.Join(lines, "\n")
}

// Retaining a deleted line's backing storage would keep its removed bytes alive.
// Surviving prefixes and suffixes therefore receive owned storage.
func ownedEditLine(parts ...string) string {
	length := 0
	for _, part := range parts {
		length += len(part)
	}
	var line strings.Builder
	line.Grow(length)
	for _, part := range parts {
		line.WriteString(part)
	}
	return line.String()
}
