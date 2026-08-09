package markdown

import (
	"slices"
	"strings"
)

// Stream renders markdown that is still arriving.
//
// It is the whole reason this module exists rather than a call to somebody's
// renderer. A markdown parser takes a document; a program showing a model's answer
// has a prefix of one, growing a few words at a time, and re-rendering the whole of
// it on every chunk is quadratic in the length of the answer — which is exactly the
// case where answers are long.
//
// So a stream splits what has arrived into the part that is certainly finished and
// the part that is not. [Stream.Feed] hands back blocks for the first, once, and
// never looks at that text again; [Stream.Open] renders the second, which is short
// by construction and is re-rendered as often as anybody likes.
//
//	for chunk := range answer {
//	    doc.Append(stream.Feed(chunk)...)
//	    live = stream.Open()
//	}
//	doc.Append(stream.Flush()...)
//
// # Where it cuts
//
// At a blank line, once a line has arrived after it that does not begin with a
// space — and never inside a fenced block of code. That is what "certainly finished"
// can be made of without a parser that can be asked what it is in the middle of: a
// blank line ends every block markdown has, except that a list or an indented block
// of code carries on across one when what follows it is indented.
//
// The cost of the rule is stated rather than hidden. A list with blank lines between
// its items is published in pieces, and reads the same. A link written as a
// reference — the address on a line of its own further down — is published before
// its address arrives, and comes out as the words without the link. Both are the
// price of showing an answer as it is written instead of after it is finished.
//
// A Stream must not be copied after first use, like [strings.Builder]. It is owned
// by the goroutine feeding and rendering one source and is not safe for concurrent
// use. Its zero value is ready.
type Stream struct {
	// look is how the blocks are drawn. Changing it affects what is published after
	// the change and not what was published before it, which is the one thing a
	// stream cannot go back on.
	look Look

	// held is the source that has not been published. A builder makes many small
	// chunks an amortized append rather than copying the complete open tail for every
	// byte that arrives.
	held strings.Builder
	// scanned is where the current incomplete line starts; searched is how far that
	// line has already been checked for a newline. Keeping both means a long line
	// delivered one byte at a time searches each byte once without pretending an
	// incomplete line can already be interpreted.
	scanned, searched int
	// fenced says the scan is inside a block of code, and fence is what would close
	// it.
	fenced bool
	fence  string
	// blank is the offset just past the most recent run of blank lines, which is a
	// cut waiting to be confirmed by whatever comes next. Zero is none of them: a cut
	// at the very start of the held text would be a cut with nothing before it, so the
	// one value that cannot mean a cut is the one that means there is not one.
	blank int

	// open is the last rendering of what is still arriving, kept so that asking for it
	// twice in one frame — to measure and to draw — parses once.
	open  []Block
	fresh bool
}

// SetLook changes how unpublished text is rendered. Blocks already returned by
// Feed cannot change; the open tail and everything published afterwards use look.
// Stream owns the heading styles so later caller mutation cannot silently change
// its rendering without invalidating the open cache.
func (s *Stream) SetLook(look Look) {
	s.look = cloneLook(look)
	clear(s.open)
	s.open = nil
	s.fresh = false
}

// Look returns a snapshot of the stream's current appearance.
func (s *Stream) Look() Look { return cloneLook(s.look) }

func cloneLook(look Look) Look {
	look.Headings = slices.Clone(look.Headings)
	look.Glyphs.Bullet = strings.Clone(look.Glyphs.Bullet)
	look.Glyphs.Bar = strings.Clone(look.Glyphs.Bar)
	look.Glyphs.Divider = strings.Clone(look.Glyphs.Divider)
	look.Glyphs.Checked = strings.Clone(look.Glyphs.Checked)
	look.Glyphs.Unchecked = strings.Clone(look.Glyphs.Unchecked)
	return look
}

// Feed takes another piece of the answer and returns the blocks it finished.
//
// Nothing is lost by a chunk that finishes nothing: what it added is held, and the
// blocks it eventually becomes are returned by a later call or by [Stream.Flush].
//
// It follows the ordinary streaming-decoder shape: hand over the next piece, take
// back what is now decidable, and let Flush settle what only the end can. It is
// deliberately not Write — this is not an io.Writer,
// and something wired to a command's output is written to from a goroutine that may
// not touch what is on screen.
func (s *Stream) Feed(chunk string) []Block {
	if chunk == "" {
		return nil
	}
	_, _ = s.held.WriteString(chunk)
	clear(s.open)
	s.open = nil
	s.fresh = false

	cut := s.scan()
	if cut <= 0 {
		return nil
	}
	source := s.held.String()
	done := Render(source[:cut], s.look)
	// A substring shares the complete builder allocation. Copy the short tail into a
	// fresh builder at the ownership cut so it cannot retain the published prefix.
	// Reset releases the old buffer; tail keeps it alive only until WriteString has
	// copied from it.
	tail := source[cut:]
	s.held.Reset()
	s.held.Grow(len(tail))
	_, _ = s.held.WriteString(tail)
	s.scanned -= cut
	s.searched -= cut
	if s.blank > cut {
		s.blank -= cut
	} else {
		s.blank = 0
	}
	return done
}

// Open is what is still being written, rendered.
//
// It is a rendering of a prefix, so it says what the text says so far: a heading
// halfway through its own words is a heading, and a fenced block of code with no
// closing fence yet is a block of code. That is what a reader sees while an answer
// is written, and it is what they would see if it stopped there.
func (s *Stream) Open() []Block {
	if !s.fresh {
		s.open, s.fresh = Render(s.held.String(), s.look), true
	}
	return slices.Clone(s.open)
}

// Flush publishes whatever is left, which is what the end of an answer is.
func (s *Stream) Flush() []Block {
	done := Render(s.held.String(), s.look)
	s.Reset()
	return done
}

// Reset forgets everything, for a stream about to be given a different answer.
func (s *Stream) Reset() {
	s.held.Reset()
	s.scanned, s.searched, s.blank = 0, 0, 0
	s.fenced, s.fence = false, ""
	s.open, s.fresh = nil, false
}

// scan reads the complete lines that have arrived since the last call and returns
// how much of the held text is certainly finished, or zero for none of it.
//
// Only whole lines are looked at. A line that has not ended cannot be told apart
// from the beginning of a different one — "```" is a fence and "```go" is a fence
// with a language, and "“" is neither yet — so the scan stops at the last newline
// and takes up there when more arrives.
func (s *Stream) scan() int {
	cut := 0
	source := s.held.String()
	for {
		rest := source[s.searched:]
		nl := strings.IndexByte(rest, '\n')
		if nl < 0 {
			s.searched = len(source)
			return cut
		}
		end := s.searched + nl
		line := source[s.scanned:end]
		s.scanned = end + 1
		s.searched = s.scanned
		trimmed := strings.TrimRight(line, " \t")

		switch {
		case s.fenced:
			if closes(trimmed, s.fence) {
				s.fenced, s.fence = false, ""
			}
		case fenceOf(trimmed) != "":
			s.fenced, s.fence = true, fenceOf(trimmed)
			s.blank = 0
		case trimmed == "":
			// A cut, if what follows says so. The whole run of blank lines goes with
			// what came before it: they are what ended it.
			s.blank = s.scanned
		default:
			if s.blank > 0 && !indented(line) {
				// A line at the left margin after a blank one begins something new, and
				// nothing before it can still be added to.
				cut = s.blank
			}
			s.blank = 0
		}
	}
}

// fenceOf is the run of backticks or tildes that opens a block of code, or nothing
// when the line does not open one.
//
// Up to three spaces of indent, because that is what the syntax allows and what a
// list item's contents arrive with. Four would be a block of code by indent, which
// needs no fence and ends by itself.
func fenceOf(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return ""
	}
	for _, mark := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, mark) {
			run := 0
			for run < len(trimmed) && trimmed[run] == mark[0] {
				run++
			}
			return trimmed[:run]
		}
	}
	return ""
}

// closes reports whether a line ends the fence that opened.
//
// A closing fence is the same character, at least as long, and has nothing after it
// — which is what lets a line of backticks inside a block of shell script not end it.
func closes(line, fence string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || fence == "" {
		return false
	}
	run := fenceOf(trimmed)
	return run != "" && run[0] == fence[0] && len(run) >= len(fence) && strings.TrimRight(trimmed[len(run):], " \t") == ""
}

// indented reports whether a line begins with room for it, which is how a list item
// and a block of code by indent carry on across a blank line.
func indented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}
