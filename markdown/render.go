package markdown

import (
	"strconv"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	gparser "github.com/yuin/goldmark/parser"
	gtext "github.com/yuin/goldmark/text"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// parse reads a document into the tree the renderer walks.
//
// One parser rather than one per call: building it walks a list of extensions and
// their priorities, and doing that for every chunk of a streaming answer would be
// the most expensive thing in the module. Parsing itself keeps no state on it —
// every parse gets a context of its own — so the one instance is shared.
//
// GFM is on. Tables, task lists, strikethrough and bare URLs are what is actually
// written, whatever the specification says is core; a renderer that showed a table
// as the pipes it was typed with would be one nobody could use for the thing this
// module exists for.
func parse(source []byte) ast.Node { return parser().Parse(gtext.NewReader(source)) }

var parser = sync.OnceValue(func() gparser.Parser {
	return goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()
})

// Render turns a whole markdown document into blocks.
//
// It is the form for text that has finished arriving. Anything still being written
// wants a [Stream], which is this applied to the part that is certainly finished and
// again to the part that is not.
func Render(source string, look Look) []Block {
	if source == "" {
		return nil
	}
	r := &renderer{look: look, source: []byte(source)}
	root := parse(r.source)
	r.render(root, frame{body: look.Text})
	return r.blocks
}

// frame is where a block sits: how far in its text starts, what is drawn beside it,
// what its text is drawn in, and whether it follows a blank row.
//
// It is one value carried down the tree rather than four arguments, because every
// one of them is inherited by default and changed by exactly one kind of node: a
// list changes the indent, a quotation changes the rail and the body, and a list
// item decides whether what is inside it is spaced out.
type frame struct {
	indent int
	rail   *railChain
	body   grid.Style
	// tight says no blank row goes before this block: the first block of a document,
	// and the first block of a list item.
	tight bool
	// dense says nothing inside this block is spaced out either, which is what a list
	// written without blank lines in it means. Without it the text of an item and the
	// list nested under it would be two blocks with a blank row between them, and a
	// tight list would come out loose the moment anything was nested in it.
	dense bool
}

// railChain is a quotation prefix as persistent segments. Nested quotations share
// their prefix and add one segment, so depth costs one allocation per level rather
// than copying the complete rail at every level. A Block materializes the line once
// when ownership crosses into the renderer result.
type railChain struct {
	previous *railChain
	span     text.Span
	depth    int
}

func (r *railChain) line() text.Line {
	if r == nil {
		return nil
	}
	line := make(text.Line, r.depth)
	for current := r; current != nil; current = current.previous {
		line[current.depth-1] = current.span
	}
	return line
}

// renderer walks the tree and leaves blocks behind it.
type renderer struct {
	look   Look
	source []byte
	blocks []Block
	// marker waits for the next block pushed, which is how a list item's bullet
	// reaches the first line of whatever the item begins with — a paragraph, a nested
	// list, a block of code.
	marker text.Line
}

// push adds a block, giving it whatever marker is waiting.
func (r *renderer) push(b Block) {
	b.Marker, r.marker = r.marker, nil
	r.blocks = append(r.blocks, b)
}

type renderActionKind uint8

const (
	renderNode renderActionKind = iota
	setMarker
	clearMarker
)

type renderAction struct {
	kind   renderActionKind
	node   ast.Node
	frame  frame
	marker text.Line
}

// render walks the block tree with explicit actions. Markdown nesting is input, not
// trusted program structure; it therefore consumes heap storage proportional to the
// active depth and never the goroutine call stack.
func (r *renderer) render(root ast.Node, in frame) {
	stack := make([]renderAction, 0, 16)
	r.scheduleChildren(&stack, root, in)
	for len(stack) > 0 {
		last := len(stack) - 1
		action := stack[last]
		stack = stack[:last]
		switch action.kind {
		case setMarker:
			r.marker = action.marker
		case clearMarker:
			r.marker = nil
		case renderNode:
			r.block(action.node, action.frame, &stack)
		}
	}
}

// scheduleChildren pushes children in reverse so the action stack visits them in
// document order. Tightness belongs to the child's position among its siblings.
func (r *renderer) scheduleChildren(stack *[]renderAction, n ast.Node, in frame) {
	for child := n.LastChild(); child != nil; child = child.PreviousSibling() {
		at := in
		first := child.PreviousSibling() == nil
		at.tight = (in.tight && first) || (in.dense && !first)
		*stack = append(*stack, renderAction{kind: renderNode, node: child, frame: at})
	}
}

// block renders one node of the document.
//
// What is not here is as deliberate as what is. Raw HTML is dropped: a terminal
// cannot show a tag, printing it as text is noise, and a renderer that guessed at
// what a tag meant would be a browser.
func (r *renderer) block(n ast.Node, in frame, stack *[]renderAction) {
	switch node := n.(type) {
	case *ast.Heading:
		r.push(Block{
			Indent: in.indent, Rail: in.rail.line(), Gap: !in.tight,
			Lines: r.inline(node, r.look.heading(node.Level)),
		})
	case *ast.Paragraph:
		r.push(Block{
			Indent: in.indent, Rail: in.rail.line(), Gap: !in.tight,
			Lines: r.inline(node, in.body),
		})
	case *ast.TextBlock:
		// What a list item's own text is, when the list is written without blank lines
		// in it. It is a paragraph in everything but name.
		r.push(Block{
			Indent: in.indent, Rail: in.rail.line(), Gap: !in.tight,
			Lines: r.inline(node, in.body),
		})
	case *ast.List:
		r.list(node, in, stack)
	case *ast.ListItem:
		r.scheduleChildren(stack, node, in)
	case *ast.Blockquote:
		inner := in
		inner.rail, inner.indent = r.quoted(in.rail, in.indent)
		inner.body = r.look.Quote
		r.scheduleChildren(stack, node, inner)
	case *ast.FencedCodeBlock:
		r.code(node, string(node.Language(r.source)), in)
	case *ast.CodeBlock:
		r.code(node, "", in)
	case *ast.ThematicBreak:
		r.push(Block{
			Indent: in.indent, Rail: in.rail.line(), Gap: !in.tight, Rule: true,
			Lines: r.rule(),
		})
	case *east.Table:
		r.table(node, in)
	case *ast.HTMLBlock:
		// Dropped, and see above.
	default:
		r.scheduleChildren(stack, n, in)
	}
}

// rule is what a thematic break draws: one divider, repeated to the width when it is
// drawn — see [Block.Rule].
//
// A look with no divider still gets a row, because that is what a break is for: it
// separates, and a separator that vanished would join the two things it was between.
func (r *renderer) rule() []text.Line {
	if r.look.Glyphs.Divider == "" {
		return []text.Line{nil}
	}
	return []text.Line{text.Of(r.look.Glyphs.Divider, r.look.Rule)}
}

// list renders a list, one item at a time, with the item's mark waiting for the
// first block inside it.
func (r *renderer) list(n *ast.List, in frame, stack *[]renderAction) {
	number := n.Start
	if number <= 0 {
		number = 1
	}
	index := n.ChildCount() - 1
	for item := n.LastChild(); item != nil; item = item.PreviousSibling() {
		marker := r.bullet(n, ordinal(number, index))
		inner := in
		inner.indent = addExtent(in.indent, marker.Width())
		// An item is against the one before it in a tight list and spaced from it in a
		// loose one, which is the distinction markdown draws by whether there are blank
		// lines between them — and the one thing about a list a reader notices.
		//
		// The first item is not spaced from the item before it, because there is none:
		// what it is spaced from is whatever came before the list, which is the frame's
		// to say. A list that took its own tightness for that would run into the
		// paragraph above it.
		inner.tight = (index == 0 && in.tight) || (index > 0 && n.IsTight)
		inner.dense = n.IsTight
		*stack = append(*stack,
			renderAction{kind: clearMarker},
			renderAction{kind: renderNode, node: item, frame: inner},
			renderAction{kind: setMarker, marker: marker},
		)
		index--
	}
}

func ordinal(start, index int) int {
	if index <= 0 {
		return start
	}
	const maxInt = int(^uint(0) >> 1)
	if start > maxInt-index {
		return maxInt
	}
	return start + index
}

// bullet is what marks an item: a number for an ordered list, the look's bullet for
// the rest, and the room after it that the item's text is indented past.
func (r *renderer) bullet(n *ast.List, number int) text.Line {
	if n.IsOrdered() {
		return text.Of(strconv.Itoa(number)+". ", r.look.Marker)
	}
	if r.look.Glyphs.Bullet == "" {
		// No bullet to draw, and the indent still has to happen or a list reads as a
		// paragraph with odd line breaks.
		return text.Of("  ", r.look.Marker)
	}
	return text.Of(r.look.Glyphs.Bullet+" ", r.look.Marker)
}

// quoted adds one persistent rail segment and advances the content indent by this
// segment only. Adding the complete accumulated rail at every level would move a
// depth-n quotation by the triangular number of columns.
func (r *renderer) quoted(previous *railChain, indent int) (*railChain, int) {
	bar := r.look.Glyphs.Bar
	if bar == "" {
		bar = " "
	}
	span := text.Span{Text: bar + " ", Style: r.look.Rail}
	depth := 1
	if previous != nil {
		depth = previous.depth + 1
	}
	return &railChain{previous: previous, span: span, depth: depth}, addExtent(indent, text.Width(span.Text))
}

func addExtent(a, b int) int {
	a, b = max(a, 0), max(b, 0)
	const maxInt = int(^uint(0) >> 1)
	if b > maxInt-a {
		return maxInt
	}
	return a + b
}

// code renders a block of code, through the look's highlighter when it has one.
func (r *renderer) code(n ast.Node, language string, in frame) {
	lines := n.Lines()
	source := make([]string, 0, lines.Len())
	for i := range lines.Len() {
		segment := lines.At(i)
		source = append(source, strings.TrimRight(string(segment.Value(r.source)), "\n"))
	}

	var out []text.Line
	if r.look.Highlight != nil {
		out = r.look.Highlight(language, strings.Join(source, "\n"))
	}
	if out == nil {
		out = make([]text.Line, 0, len(source))
		for _, line := range source {
			out = append(out, text.Of(line, r.look.Block))
		}
	}
	r.push(Block{Indent: in.indent, Rail: in.rail.line(), Gap: !in.tight, Lines: out})
}

// inline turns the inline nodes under n into lines, splitting where the text said to
// and nowhere else.
//
// A soft break — the line break somebody typed inside a paragraph — becomes a space,
// because the width the text will be laid out in is not the width it was written in.
// A hard break is a line break, because it was asked for.
func (r *renderer) inline(n ast.Node, style grid.Style) []text.Line {
	var out inlineWriter

	stack := make([]inlineAction, 0, 16)
	pushInlineChildren(&stack, n, style, "")
	for len(stack) > 0 {
		last := len(stack) - 1
		action := stack[last]
		stack = stack[:last]
		if action.target {
			r.target(&out, action.destination, action.shown, action.style)
			continue
		}
		switch node := action.node.(type) {
		case *ast.Text:
			out.add(string(node.Segment.Value(r.source)), action.style, action.link)
			switch {
			case node.HardLineBreak():
				out.breakLine()
			case node.SoftLineBreak():
				out.add(" ", action.style, action.link)
			}
		case *ast.String:
			out.add(string(node.Value), action.style, action.link)
		case *ast.CodeSpan:
			out.add(r.plain(node), action.style.Merge(r.look.Code), action.link)
		case *ast.Emphasis:
			if node.Level >= 2 {
				pushInlineChildren(&stack, node, action.style.Merge(r.look.Strong), action.link)
			} else {
				pushInlineChildren(&stack, node, action.style.Merge(r.look.Emphasis), action.link)
			}
		case *east.Strikethrough:
			pushInlineChildren(&stack, node, action.style.Merge(r.look.Struck), action.link)
		case *ast.Link:
			// Push the target before the children: the stack visits the words first and
			// then writes the optional address, preserving document order.
			target := string(node.Destination)
			stack = append(stack, inlineAction{
				target: true, destination: target, shown: r.plain(node), style: action.style,
			})
			pushInlineChildren(&stack, node, action.style.Merge(r.look.Link), target)
		case *ast.AutoLink:
			url := string(node.URL(r.source))
			out.add(url, action.style.Merge(r.look.Link), url)
		case *ast.Image:
			// A picture is not something a row of cells can hold — see the graphics
			// package for what a terminal will take — so what is left is what it was
			// called and where it is.
			target := string(node.Destination)
			out.add("["+r.plain(node)+"]", action.style.Merge(r.look.Link), target)
			r.target(&out, target, "", action.style)
		case *east.TaskCheckBox:
			out.add(r.box(node.IsChecked), action.style.Merge(r.look.Marker), action.link)
		case *ast.RawHTML:
			// Dropped, like a block of it.
		default:
			pushInlineChildren(&stack, action.node, action.style, action.link)
		}
	}
	return out.finish()
}

// inlineWriter is the mutable construction form of styled lines. Markdown parsers
// are free to split one visual run into any number of adjacent AST nodes; owning a
// Builder here keeps that parser detail from turning the rendering cost quadratic
// or leaking extra spans into the public result.
type inlineWriter struct {
	lines []text.Line
	line  text.Line
	run   strings.Builder
	style grid.Style
	link  string
	open  bool
}

func (w *inlineWriter) add(s string, style grid.Style, link string) {
	if s == "" {
		return
	}
	if !w.open || w.style != style || w.link != link {
		w.flush()
		w.style, w.link, w.open = style, link, true
	}
	w.run.WriteString(s)
}

func (w *inlineWriter) breakLine() {
	w.flush()
	w.lines = append(w.lines, w.line)
	w.line = nil
}

func (w *inlineWriter) finish() []text.Line {
	w.breakLine()
	return w.lines
}

func (w *inlineWriter) flush() {
	if !w.open {
		return
	}
	w.line = append(w.line, text.Span{Text: w.run.String(), Style: w.style, Link: w.link})
	w.run.Reset()
	w.open = false
}

type inlineAction struct {
	node  ast.Node
	style grid.Style
	link  string

	target      bool
	destination string
	shown       string
}

func pushInlineChildren(stack *[]inlineAction, n ast.Node, style grid.Style, link string) {
	for child := n.LastChild(); child != nil; child = child.PreviousSibling() {
		*stack = append(*stack, inlineAction{node: child, style: style, link: link})
	}
}

// target writes where a link points, after the words that point there.
//
// Only when the look has a style for it. The words themselves carry the address
// now, so a terminal that shows hyperlinks needs nothing written out — and a
// document full of parenthesised URLs is what that costs. A look that has no style
// for an address says it wants the shorter reading, which is the same rule the
// glyphs keep: what a look does not describe, it does not draw.
//
// It is left out either way when it says nothing the text did not: a bare URL as its
// own link text is the commonest link there is.
func (r *renderer) target(out *inlineWriter, destination, shown string, style grid.Style) {
	if destination == "" || destination == shown || r.look.Target == (grid.Style{}) {
		return
	}
	out.add(" ("+destination+")", style.Merge(r.look.Target), destination)
}

// box is what marks a task, and nothing when the look has no marks for one.
func (r *renderer) box(checked bool) string {
	mark := r.look.Glyphs.Unchecked
	if checked {
		mark = r.look.Glyphs.Checked
	}
	if mark == "" {
		return ""
	}
	return mark + " "
}

// plain is the text under a node with the styling left out, which is what a code
// span, a link's own words and an image's description all come to.
func (r *renderer) plain(n ast.Node) string {
	var b strings.Builder
	stack := make([]ast.Node, 0, 16)
	pushNodes := func(n ast.Node) {
		for child := n.LastChild(); child != nil; child = child.PreviousSibling() {
			stack = append(stack, child)
		}
	}
	pushNodes(n)
	for len(stack) > 0 {
		last := len(stack) - 1
		node := stack[last]
		stack = stack[:last]
		switch node := node.(type) {
		case *ast.Text:
			b.Write(node.Segment.Value(r.source))
		case *ast.String:
			b.Write(node.Value)
		default:
			pushNodes(node)
		}
	}
	return b.String()
}
