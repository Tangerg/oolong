package base

import (
	"math/bits"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/text"
)

// Trigger is what opens a completion: the characters that begin a token, and where
// they count as beginning one.
type Trigger struct {
	Prefix  string
	AtStart bool
}

// Token is the run of text a completion is being offered for.
type Token struct {
	// Line distinguishes identical ranges in a multiline editor. TokenAt reads one line.
	Line int
	// Start and End are the byte range in the line that accepting a candidate
	// replaces: everything after the prefix, through the end of the token. The prefix
	// itself stays where it is — it is what the user typed to ask for the completion,
	// not part of the answer.
	Start, End int
	// Query is that range up to the cursor, which is what candidates are matched
	// against. What is after the cursor is replaced but not matched: a cursor put back
	// into the middle of a word is a request to reconsider its beginning.
	Query string
	// Trigger is what opened the token, so that a caller offering several can tell
	// which kind of thing is being asked for.
	Trigger Trigger
}

// TokenAt finds the token the cursor is inside, if any. The rightmost trigger before
// the cursor wins, so a file mentioned inside a command completes as a file.
//
// A token ends at the first space after it, which is the one rule general enough to
// be worth having: anything more — a path that may contain spaces, a quote that
// protects them — is the caller's grammar and not a terminal library's.
// Invalid UTF-8, out-of-range cursors and partial graphemes have no token. Both
// the query and replacement range must preserve the editor's text boundaries.
func TokenAt(line string, cursor int, triggers ...Trigger) (Token, bool) {
	if len(triggers) == 0 || cursor < 0 || cursor > len(line) || !utf8.ValidString(line) {
		return Token{}, false
	}
	boundaries := indexClusterBoundaries(line)
	if !boundaries.contains(cursor) {
		return Token{}, false
	}
	best, at := Token{}, -1
	for _, trigger := range triggers {
		if trigger.Prefix == "" || !utf8.ValidString(trigger.Prefix) {
			continue
		}
		opened, ok := trigger.openedBefore(line, cursor, at, boundaries)
		if !ok {
			continue
		}
		start := opened + len(trigger.Prefix)
		end := tokenEnd(line, start)
		if cursor < start || cursor > end || !boundaries.contains(end) {
			continue
		}
		best, at = Token{
			Start:   start,
			End:     end,
			Query:   line[start:cursor],
			Trigger: trigger,
		}, opened
	}
	return best, at >= 0
}

// Rejected prefixes must not reparse every preceding grapheme. This local index
// shares one segmentation pass and bounds scratch storage to one bit per byte offset.
type clusterBoundaries []uint

func indexClusterBoundaries(line string) clusterBoundaries {
	boundaries := make(clusterBoundaries, len(line)/bits.UintSize+1)
	boundaries[0] = 1
	for at, cluster := range text.Clusters(line) {
		end := at + len(cluster)
		boundaries[end/bits.UintSize] |= 1 << (end % bits.UintSize)
	}
	return boundaries
}

func (b clusterBoundaries) contains(at int) bool {
	return at >= 0 && at/bits.UintSize < len(b) && b[at/bits.UintSize]&(1<<(at%bits.UintSize)) != 0
}

func (t Trigger) openedBefore(line string, cursor, after int, boundaries clusterBoundaries) (int, bool) {
	for end := cursor; end >= len(t.Prefix); {
		candidate := strings.LastIndex(line[:end], t.Prefix)
		if candidate < 0 || candidate < after {
			return 0, false
		}
		if boundaries.contains(candidate) && boundaries.contains(candidate+len(t.Prefix)) && t.opensAt(line, candidate) {
			return candidate, true
		}
		end = candidate + len(t.Prefix) - 1
	}
	return 0, false
}

func (t Trigger) opensAt(line string, at int) bool {
	if t.AtStart {
		return at == 0
	}
	if at == 0 {
		return true
	}
	_, size := utf8.DecodeLastRuneInString(line[:at])
	return !text.ClassOf(line[at-size : at]).InWord()
}

func tokenEnd(line string, start int) int {
	for end := start; end < len(line); {
		r, size := utf8.DecodeRuneInString(line[end:])
		if unicode.IsSpace(r) {
			return end
		}
		end += size
	}
	return len(line)
}

// TokenAtCursor reads the token on the current line. Masked text cannot be offered to a completion provider.
func (e *Editor) TokenAtCursor(triggers ...Trigger) (Token, bool) {
	lines := e.document().lines
	if e.Mask() != "" || e.caret.Line < 0 || e.caret.Line >= len(lines) {
		return Token{}, false
	}
	token, ok := TokenAt(lines[e.caret.Line], e.caret.Byte, triggers...)
	if ok {
		token.Line = e.caret.Line
	}
	return token, ok
}

// clone owns every string a caller supplied, so an offer cannot retain a much
// larger buffer it was sliced from.
func (t Token) clone() Token {
	t.Query = strings.Clone(t.Query)
	t.Trigger.Prefix = strings.Clone(t.Trigger.Prefix)
	return t
}
