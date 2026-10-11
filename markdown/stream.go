package markdown

import (
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
)

// Stream publishes complete blocks once and retains an open tail so new chunks
// do not rerender the entire prefix. Parser-confirmed boundaries keep lists and
// containers together regardless of transport chunking. Definitions must precede
// published reference links; later definitions cannot revise transferred blocks.
// The zero value is ready. A Stream belongs to one source goroutine and must not
// be copied after first use. Renderer callbacks may read Look; reentering Feed,
// Open, Flush, Reset or SetLook panics while source and cache remain unpublished.
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
	open      []Block
	openErr   error
	fresh     bool
	rendering bool
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
	s.requireIdle()
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

// Feed returns newly completed blocks and retains the rest until later input or
// [Stream.Flush]. Feed belongs to the stream owner, not a background io.Writer.
func (s *Stream) Feed(chunk string) ([]Block, error) {
	s.requireIdle()
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
	if cut <= 0 || s.splitsBlock(cut) {
		return nil, nil
	}
	source := s.held.String()
	done, err := s.render(source[:cut])
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
	s.requireIdle()
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
		s.open, s.openErr = s.render(source)
		s.fresh = true
	}
	return slices.Clone(s.open), s.openErr
}

// Flush publishes whatever is left, which is what the end of an answer is.
func (s *Stream) Flush() ([]Block, error) {
	s.requireIdle()
	done, err := s.render(s.held.String())
	s.Reset()
	return done, err
}

// Reset forgets everything, for a stream about to be given a different answer.
func (s *Stream) Reset() {
	s.requireIdle()
	s.held.Reset()
	s.scanned, s.searched, s.blank = 0, 0, 0
	s.afterCR = false
	s.open, s.fresh = nil, false
	s.openErr = nil
}

func (s *Stream) requireIdle() {
	if s.rendering {
		panic("markdown: reentrant stream operation during rendering")
	}
}

func (s *Stream) render(source string) ([]Block, error) {
	s.rendering = true
	defer func() { s.rendering = false }()
	return Render(source, s.look)
}

// Only complete lines can propose boundaries; partial lines may still change their
// meaning. The parser validates candidates so approximate fence tracking cannot
// prevent a valid cut from ever being offered.
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

// Only the parser knows whether fences, HTML or container indentation cross a cut.
// The scanner proposes candidates without creating a second set of block rules.
func (s *Stream) splitsBlock(cut int) bool {
	source := []byte(s.held.String())
	whole := parse(source)
	split := false
	_ = ast.Walk(whole, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if rawBlockSplit(node, cut) {
			split = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if split {
		return true
	}
	prefix := parse(source[:cut])
	last := whole.FirstChild()
	for i := 1; i < prefix.ChildCount() && last != nil; i++ {
		last = last.NextSibling()
	}
	return !completeBlock(prefix.LastChild(), last, cut)
}

// Empty list items have no source lines. A prefix is complete only when its final
// block has the full parser's block structure and none of its descendants cross cut.
// Inline reference resolution retains Stream's documented forward-reference limit.
func completeBlock(prefix, whole ast.Node, cut int) bool {
	if prefix == nil || whole == nil || prefix.Kind() != whole.Kind() {
		return false
	}
	lines := whole.Lines()
	if lines != nil && lines.Len() > 0 && lines.At(lines.Len()-1).Stop > cut {
		return false
	}
	for a, b := nextBlock(prefix.FirstChild()), nextBlock(whole.FirstChild()); a != nil || b != nil; a, b = nextBlock(a.NextSibling()), nextBlock(b.NextSibling()) {
		if a == nil || b == nil || !completeBlock(a, b, cut) {
			return false
		}
	}
	return true
}

func nextBlock(node ast.Node) ast.Node {
	for node != nil && node.Type() != ast.TypeBlock {
		node = node.NextSibling()
	}
	return node
}

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
