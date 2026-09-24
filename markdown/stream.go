package markdown

import (
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
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
//	    blocks, feedErr := stream.Feed(chunk)
//	    doc.Append(blocks...)
//	    live, openErr = stream.Open()
//	    report(errors.Join(feedErr, openErr))
//	}
//	blocks, err := stream.Flush()
//	doc.Append(blocks...)
//	report(err)
//
// # Where it cuts
//
// At a blank line, once a line has arrived after it that does not begin with a
// space — and never inside fenced code or display mathematics. That is what
// "certainly finished" can be made of without a parser that can be asked what it is
// in the middle of: a blank line ends every block markdown has, except that a list or
// an indented block of code carries on across one when what follows it is indented.
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
	noCopy noCopy

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
	afterCR           bool
	// blank is the offset just past the most recent run of blank lines, which is a
	// cut waiting to be confirmed by whatever comes next. Zero is none of them: a cut
	// at the very start of the held text would be a cut with nothing before it, so the
	// one value that cannot mean a cut is the one that means there is not one.
	blank int

	// open is the last rendering of what is still arriving, kept so that asking for it
	// twice in one frame — to measure and to draw — parses once.
	open    []Block
	openErr error
	fresh   bool
}

// noCopy makes the stream's single-owner contract visible to go vet. Its methods
// are never called.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// SetLook changes how unpublished text is rendered. Blocks already returned by
// Feed cannot change; the open tail and everything published afterwards use look.
// Stream owns the heading styles so later caller mutation cannot silently change
// its rendering without invalidating the open cache.
func (s *Stream) SetLook(look Look) {
	s.look = cloneLook(look)
	clear(s.open)
	s.open = nil
	s.openErr = nil
	s.fresh = false
}

// Look returns a snapshot of the stream's current appearance.
func (s *Stream) Look() Look { return cloneLook(s.look) }

func cloneLook(look Look) Look {
	look.Headings = slices.Clone(look.Headings)
	look.extensions = maps.Clone(look.extensions)
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
func (s *Stream) Feed(chunk string) ([]Block, error) {
	if chunk == "" {
		return nil, nil
	}
	afterCR := strings.HasSuffix(chunk, "\r")
	if s.afterCR {
		chunk = strings.TrimPrefix(chunk, "\n")
	}
	s.afterCR = afterCR
	_, _ = s.held.WriteString(normalizeNewlines(chunk))
	clear(s.open)
	s.open = nil
	s.openErr = nil
	s.fresh = false

	cut := s.scan()
	if cut <= 0 || s.splitsARawBlock(cut) {
		return nil, nil
	}
	source := s.held.String()
	done, err := Render(source[:cut], s.look)
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
	return done, err
}

// Open is what is still being written, rendered.
//
// It is a rendering of a prefix, so it says what the text says so far: a heading
// halfway through its own words is a heading, and a fenced block of code with no
// closing fence yet is a block of code. That is what a reader sees while an answer
// is written. A trailing incomplete UTF-8 rune waits for the next Feed; Flush
// settles it as replacement text if the source ends there.
func (s *Stream) Open() ([]Block, error) {
	if !s.fresh {
		source := s.held.String()
		if len(source) > 0 {
			start := len(source) - 1
			for start > 0 && !utf8.RuneStart(source[start]) {
				start--
			}
			if !utf8.FullRuneInString(source[start:]) {
				source = source[:start]
			}
		}
		s.open, s.openErr = Render(source, s.look)
		s.fresh = true
	}
	return slices.Clone(s.open), s.openErr
}

// Flush publishes whatever is left, which is what the end of an answer is.
func (s *Stream) Flush() ([]Block, error) {
	done, err := Render(s.held.String(), s.look)
	s.Reset()
	return done, err
}

// Reset forgets everything, for a stream about to be given a different answer.
func (s *Stream) Reset() {
	s.held.Reset()
	s.scanned, s.searched, s.blank = 0, 0, 0
	s.afterCR = false
	s.open, s.fresh = nil, false
	s.openErr = nil
}

// scan reads the complete lines that have arrived since the last call and returns
// where the held text could be cut, or zero for nowhere.
//
// Could, not may. This reads lines and nothing else: a blank line, and then a line
// at the left margin that begins something new. Whether a cut there would fall
// inside text the parser is not allowed to interpret is [Stream.splitsARawBlock]'s
// answer, and the scan does not try to have an opinion about it.
//
// It used to track fences itself, to save the parser the trouble. Tracking them
// approximately is the trouble: three lines that only look like fences — an info
// string with a backtick in it, a run of backticks inside an HTML comment, a closing
// fence indented under a list item — left the scan believing it was inside a block
// of code with nothing in the document able to close it. From there it proposed no
// cuts at all, and a guard that can only refuse a candidate cannot recover a
// candidate that is never offered. Everything after such a line waited for the end
// of the answer.
//
// What that tracking bought was measured before it was given up. It saves a parse on
// the feeds where the scan would otherwise offer a candidate from inside a block of
// code — which needs unindented lines with blank lines between them, inside a fence,
// to happen at all. In the loop this type documents, where every feed is followed by
// [Stream.Open], that parse is 2.7% of what the feed already costs: Open renders the
// whole unpublished tail, and the tail is exactly what is long in the case this was
// protecting.
//
// Only whole lines are looked at. A line that has not ended cannot be told apart
// from the beginning of a different one, so the scan stops at the last newline and
// takes up there when more arrives.
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
		line := strings.TrimSuffix(source[s.scanned:end], "\r")
		s.scanned = end + 1
		s.searched = s.scanned

		if strings.TrimRight(line, " \t") == "" {
			// A cut, if what follows says so. The whole run of blank lines goes with
			// what came before it: they are what ended it.
			s.blank = s.scanned
			continue
		}
		if s.blank > 0 && !indented(line) {
			// A line at the left margin after a blank one begins something new, and
			// nothing before it can still be added to.
			cut = s.blank
		}
		s.blank = 0
	}
}

// indented reports whether a line begins with room for it, which is how a list item
// and a block of code by indent carry on across a blank line.
func indented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

// splitsARawBlock reports whether a proposed cut would cut through text the parser
// is not allowed to interpret.
//
// The scan that proposes cuts reads lines and nothing else, which is enough to find
// where one could go and not enough to be sure one may. Fenced code, display
// mathematics and the seven HTML forms all have boundaries only the parser knows: how
// deep a container indents its contents, whether a run of backticks inside a block
// closes it or is part of it, whether a raw block is still open. Every one of those
// is a rule the parser already has, and a second reading of them agrees with it right
// up until the document where it does not — so the parser answers, and the scan only
// proposes.
func (s *Stream) splitsARawBlock(cut int) bool {
	source := []byte(s.held.String())
	split := false
	_ = ast.Walk(parse(source), func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if rawBlockSplit(node, cut) {
			split = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return split
}

// rawBlockSplit reports whether cut falls inside one block of uninterpreted text.
func rawBlockSplit(node ast.Node, cut int) bool {
	if node.Type() != ast.TypeBlock {
		return false
	}
	lines := node.Lines()
	if lines == nil || lines.Len() == 0 {
		return false
	}
	first, last := lines.At(0), lines.At(lines.Len()-1)
	switch block := node.(type) {
	case *ast.HTMLBlock:
		// An HTML block's lines include the tag it opens with, so its first line is a
		// place a cut may go: what is before it belongs to something else.
		stop := last.Stop
		if block.HasClosure() {
			stop = max(stop, block.ClosureLine.Stop)
		}
		return first.Start < cut && stop > cut
	case *ast.FencedCodeBlock:
		return delimitedSplit(first.Start, last.Stop, cut)
	default:
		return node.Kind() == kindMathBlock && delimitedSplit(first.Start, last.Stop, cut)
	}
}

// delimitedSplit answers for a block whose delimiters are lines of their own and are
// therefore not among its lines: a cut at either end of the content still puts that
// content on the far side of the fence that owns it.
func delimitedSplit(start, stop, cut int) bool { return start <= cut && cut <= stop }
