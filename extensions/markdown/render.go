package markdown

import (
	"bytes"
	"errors"
	"html"
	"strconv"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	gparser "github.com/yuin/goldmark/parser"
	gtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
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
func parse(source []byte, context gparser.Context) ast.Node {
	return parser().Parse(gtext.NewReader(source), gparser.WithContext(context))
}

var parser = sync.OnceValue(func() gparser.Parser {
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(gparser.WithBlockParsers(
			util.Prioritized(&mathBlockParser{}, 650),
		)),
	).Parser()
})

// Render turns a whole markdown document into blocks.
// Invalid UTF-8 runs and NUL become replacement text before parsing. Extension failures
// return readable blocks alongside errors; callers must observe both results.
//
// Live previews and final documents use this same parser. Source revisions and
// finalization belong to content.StreamDocument, not to transport chunk boundaries.
func Render(source string, look Look) ([]Block, error) {
	return render(source, look, nil)
}

func render(source string, look Look, context gparser.Context) ([]Block, error) {
	if source == "" {
		return nil, nil
	}
	look = cloneLook(look)
	r := &renderer{look: look, source: []byte(normalizeSource(source))}
	root := parse(r.source, context)
	r.render(root, frame{body: look.Text})
	return r.blocks, errors.Join(r.errors...)
}

type frame struct {
	indent  int
	rails   *prefixMark
	pending *pendingMark
	body    grid.Style
	// tight says no blank row goes before this block: the first block of a document,
	// and the first block of a list item.
	tight bool
	// dense says nothing inside this block is spaced out either, which is what a list
	// written without blank lines in it means. Without it the text of an item and the
	// list nested under it would be two blocks with a blank row between them, and a
	// tight list would come out loose the moment anything was nested in it.
	dense bool
}

// A list item owns its marker until its first emitted block claims it. An item
// that begins with a nested list may transfer several still-pending markers.
type pendingMark struct {
	mark  *prefixMark
	outer *pendingMark
}

func (p *pendingMark) take() *prefixMark {
	var first *prefixMark
	for item := p; item != nil && item.mark != nil; item = item.outer {
		mark := item.mark
		mark.next = first
		first = mark
		item.mark = nil
	}
	return first
}

func (f *frame) place(span text.Span) *prefixMark {
	mark := &prefixMark{column: f.indent, span: span}
	f.indent = layout.Sum(f.indent, text.Width(span.Text))
	return mark
}

type renderer struct {
	look   Look
	source []byte
	blocks []Block
	errors []error
}

func (r *renderer) push(b Block, in frame) {
	b.prefix = blockPrefix{indent: in.indent, rails: in.rails, markers: in.pending.take()}
	b.blankBefore = !in.tight
	r.blocks = append(r.blocks, b)
}

type renderAction struct {
	node  ast.Node
	frame frame
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
		r.block(action.node, action.frame, &stack)
	}
}

// scheduleChildren pushes children in reverse so the action stack visits them in
// document order. Tightness belongs to the child's position among its siblings.
func (r *renderer) scheduleChildren(stack *[]renderAction, n ast.Node, in frame) {
	for child := n.LastChild(); child != nil; child = child.PreviousSibling() {
		at := in
		first := child.PreviousSibling() == nil
		at.tight = (in.tight && first) || (in.dense && !first)
		*stack = append(*stack, renderAction{node: child, frame: at})
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
		r.push(Block{lines: r.inline(node, r.look.heading(node.Level))}, in)
	case *ast.Paragraph, *ast.TextBlock:
		r.push(Block{lines: r.inline(node, in.body)}, in)
	case *ast.List:
		r.list(node, in, stack)
	case *ast.ListItem:
		if node.ChildCount() == 0 {
			// The first row carries the marker even for an empty item.
			r.push(Block{
				lines: []text.Line{nil},
			}, in)
			return
		}
		r.scheduleChildren(stack, node, in)
	case *ast.Blockquote:
		inner := in
		bar := r.look.Glyphs.Bar
		if bar == "" {
			bar = " "
		}
		rail := text.Span{Text: bar + " ", Style: r.look.Rail}
		inner.rails = inner.place(rail)
		inner.rails.next = in.rails
		inner.body = r.look.Quote
		r.scheduleChildren(stack, node, inner)
	case *ast.FencedCodeBlock:
		r.code(node, r.fenceLanguage(node), in)
	case *ast.CodeBlock:
		r.code(node, "", in)
	case *mathBlock:
		r.math(node, in)
	case *ast.ThematicBreak:
		r.push(Block{rule: &text.Span{Text: r.look.Glyphs.Divider, Style: r.look.Rule}}, in)
	case *east.Table:
		r.table(node, in)
	case *ast.HTMLBlock:
	default:
		r.scheduleChildren(stack, n, in)
	}
}

func (r *renderer) list(n *ast.List, in frame, stack *[]renderAction) {
	// An ordered list carries its own first number, and zero is one a document may
	// legitimately write. Only a bullet list has no number at all, and it never asks
	// for one — see [renderer.bullet].
	number := max(n.Start, 0)
	index := n.ChildCount() - 1
	for item := n.LastChild(); item != nil; item = item.PreviousSibling() {
		marker := r.bullet(n, ordinal(number, index))
		inner := in
		inner.pending = &pendingMark{mark: inner.place(marker), outer: in.pending}
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
		*stack = append(*stack, renderAction{node: item, frame: inner})
		index--
	}
}

func ordinal(start, index int) int {
	if index <= 0 {
		return start
	}
	return layout.Translate(start, index)
}

func (r *renderer) bullet(n *ast.List, number int) text.Span {
	if n.IsOrdered() {
		return text.Span{Text: strconv.Itoa(number) + ". ", Style: r.look.Marker}
	}
	if r.look.Glyphs.Bullet == "" {
		// No bullet to draw, and the indent still has to happen or a list reads as a
		// paragraph with odd line breaks.
		return text.Span{Text: "  ", Style: r.look.Marker}
	}
	return text.Span{Text: r.look.Glyphs.Bullet + " ", Style: r.look.Marker}
}

func (r *renderer) fenceLanguage(node *ast.FencedCodeBlock) string {
	if node.Info != nil {
		for word := range bytes.FieldsSeq(node.Info.Segment.Value(r.source)) {
			return markdownText(string(word))
		}
	}
	return ""
}

func (r *renderer) code(n ast.Node, language string, in frame) {
	source := r.sourceLines(n)
	if strings.EqualFold(strings.TrimSpace(language), "math") {
		r.extension(DisplayMath, "", strings.Join(source, "\n"), in)
		return
	}
	r.extension(FencedCode, language, strings.Join(source, "\n"), in)
}

func (r *renderer) math(n ast.Node, in frame) {
	r.extension(DisplayMath, "", strings.Join(r.sourceLines(n), "\n"), in)
}

func (r *renderer) extension(kind Extension, info, source string, in frame) {
	var child grid.Drawable
	var err error
	if render := r.look.renderer(kind); render != nil {
		child, err = render(info, source)
		if err == ErrUnhandled { //nolint:errorlint // Only an explicit decline suppresses diagnostics; joined failures remain observable.
			child, err = nil, nil
		} else if grid.IsNilDrawable(child) {
			child = nil
			if err == nil {
				err = errors.New("renderer returned nil content without an error")
			}
		}
	}
	if err != nil {
		r.errors = append(r.errors, &ExtensionError{Block: len(r.blocks), Extension: kind, Info: info, Err: err})
	}
	if child == nil {
		var lines []text.Line
		for line := range strings.SplitSeq(source, "\n") {
			lines = append(lines, text.Of(line, r.look.Block))
		}
		child = text.NewBlock(text.BlockConfig{Lines: lines, Wrap: kind != DisplayMath})
	}
	r.push(Block{content: child}, in)
}

func (r *renderer) sourceLines(n ast.Node) []string {
	lines := n.Lines()
	source := make([]string, 0, lines.Len())
	for i := range lines.Len() {
		segment := lines.At(i)
		source = append(source, strings.TrimRight(string(segment.Value(r.source)), "\n"))
	}
	return source
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
	pushInlineChildren(&stack, n, style, nil)
	for len(stack) > 0 {
		last := len(stack) - 1
		action := stack[last]
		stack = stack[:last]
		r.writeInline(&out, &stack, action)
	}
	return out.finish()
}

func (r *renderer) writeInline(out *inlineWriter, stack *[]inlineAction, action inlineAction) {
	if action.target {
		r.target(out, action.destination, action.shown, action.style)
		return
	}
	if !r.writeLeaf(out, action) {
		r.pushInline(stack, action)
	}
}

func (r *renderer) writeLeaf(out *inlineWriter, action inlineAction) bool {
	link := ""
	if action.link != nil {
		link = *action.link
	}
	switch node := action.node.(type) {
	case *ast.Text:
		value := string(node.Segment.Value(r.source))
		if !node.IsRaw() {
			value = markdownText(value)
		}
		out.add(value, action.style, link)
		switch {
		case node.HardLineBreak():
			out.breakLine()
		case node.SoftLineBreak():
			out.add(" ", action.style, link)
		}
	case *ast.String:
		out.add(stringValue(node), action.style, link)
	case *ast.CodeSpan:
		out.add(r.codeSpan(node), action.style.Merge(r.look.Code), link)
	case *ast.AutoLink:
		// What the document showed and where it goes are two things, and they differ
		// whenever the protocol was inferred rather than written: "www.example.com"
		// is what the reader wrote and "http://www.example.com" is what opens.
		label := string(node.Label(r.source))
		target := string(node.URL(r.source))
		if node.AutoLinkType == ast.AutoLinkEmail {
			// An address is not a destination. Without the scheme a terminal has
			// nothing to hand to a mail client; with the one goldmark attaches it has
			// "mailto://", which is not the scheme either. The address is the whole of
			// what an email autolink says, so the destination is built from it.
			target = "mailto:" + label
		}
		out.add(label, action.style.Merge(r.look.Link), target)
	case *ast.Image:
		// A picture is not something a row of cells can hold — see the graphics
		// package for what a terminal will take — so what is left is what it was
		// called and where it is.
		target := markdownText(string(node.Destination))
		if action.link == nil {
			link = target
		}
		out.add("["+r.plain(node)+"]", action.style.Merge(r.look.Link), link)
		r.target(out, target, "", action.style)
	case *east.TaskCheckBox:
		out.add(r.box(node.IsChecked), action.style.Merge(r.look.Marker), link)
	case *ast.RawHTML:
	default:
		return false
	}
	return true
}

func (r *renderer) pushInline(stack *[]inlineAction, action inlineAction) {
	switch node := action.node.(type) {
	case *ast.Emphasis:
		style := r.look.Emphasis
		if node.Level >= 2 {
			style = r.look.Strong
		}
		pushInlineChildren(stack, node, action.style.Merge(style), action.link)
	case *east.Strikethrough:
		pushInlineChildren(stack, node, action.style.Merge(r.look.Struck), action.link)
	case *ast.Link:
		// Push the target before the children: the stack visits the words first and
		// then writes the optional address, preserving document order.
		target := markdownText(string(node.Destination))
		*stack = append(*stack, inlineAction{
			target: true, destination: target, shown: r.plain(node), style: action.style,
		})
		pushInlineChildren(stack, node, action.style.Merge(r.look.Link), &target)
	default:
		pushInlineChildren(stack, action.node, action.style, action.link)
	}
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
	link  *string

	target      bool
	destination string
	shown       string
}

func pushInlineChildren(stack *[]inlineAction, n ast.Node, style grid.Style, link *string) {
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
			value := string(node.Segment.Value(r.source))
			if !node.IsRaw() {
				value = markdownText(value)
			}
			b.WriteString(value)
			if node.SoftLineBreak() || node.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.CodeSpan:
			b.WriteString(r.codeSpan(node))
		case *ast.String:
			b.WriteString(stringValue(node))
		case *ast.AutoLink:
			b.Write(node.Label(r.source))
		default:
			pushNodes(node)
		}
	}
	return b.String()
}

func stringValue(node *ast.String) string {
	value := string(node.Value)
	if node.IsRaw() || node.IsCode() {
		return value
	}
	return markdownText(value)
}

func markdownText(source string) string {
	if !strings.ContainsAny(source, "\\&") {
		return source
	}
	var out strings.Builder
	for i := 0; i < len(source); {
		if source[i] == '\\' && i+1 < len(source) && util.IsPunct(source[i+1]) {
			out.WriteByte(source[i+1])
			i += 2
			continue
		}
		if source[i] == '&' {
			if end := strings.IndexByte(source[i:min(i+33, len(source))], ';'); end >= 0 && end <= 32 {
				entity := source[i : i+end+1]
				resolved, valid := markdownEntity(entity)
				if valid {
					out.WriteString(resolved)
					i += end + 1
					continue
				}
			}
		}
		out.WriteByte(source[i])
		i++
	}
	return out.String()
}

func markdownEntity(entity string) (string, bool) {
	name := entity[1 : len(entity)-1]
	if !strings.HasPrefix(name, "#") {
		value, ok := util.LookUpHTML5EntityByName(name)
		if !ok {
			return "", false
		}
		return string(value.Characters), true
	}
	digits, base, limit := name[1:], 10, 7
	if strings.HasPrefix(digits, "x") || strings.HasPrefix(digits, "X") {
		digits, base, limit = digits[1:], 16, 6
	}
	if len(digits) == 0 || len(digits) > limit {
		return "", false
	}
	if _, err := strconv.ParseUint(digits, base, 32); err != nil {
		return "", false
	}
	return html.UnescapeString(entity), true
}

func (r *renderer) codeSpan(node *ast.CodeSpan) string {
	var out strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		textNode, ok := child.(*ast.Text)
		if !ok {
			continue
		}
		segment := textNode.Segment
		value := string(segment.Value(r.source))
		value = strings.ReplaceAll(value, "\r\n", " ")
		value = strings.ReplaceAll(value, "\n", " ")
		value = strings.ReplaceAll(value, "\r", " ")
		out.WriteString(value)
	}
	return out.String()
}

// CommonMark normalization precedes parsing so code, references and extension
// bodies share the same characters and all parser offsets address that source.
func normalizeSource(source string) string {
	const replacement = "�"
	source = strings.ToValidUTF8(source, replacement)
	source = strings.ReplaceAll(source, "\x00", replacement)
	return strings.ReplaceAll(strings.ReplaceAll(source, "\r\n", "\n"), "\r", "\n")
}

// table retains cells until layout. A table is the one markdown block whose shape
// depends on its contents and on the final region at the same time; flattening it
// during parsing loses the information needed to make a narrow table readable.
func (r *renderer) table(n *east.Table, in frame) {
	rows, header := r.cells(n)
	if len(rows) == 0 {
		return
	}
	r.push(Block{
		table: &table{
			rows: rows, aligns: columnAlignments(n.Alignments), header: header,
			separator: r.column(), divider: r.look.Glyphs.Divider,
			rail: r.look.Rail, rule: r.look.Rule,
		},
	}, in)
}

// cells reads the table into styled cells and says whether the first row is its
// heading. Cells remain logical lines; wrapping belongs to table layout.
func (r *renderer) cells(n *east.Table) (rows [][]text.Line, header bool) {
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		style := r.look.Text
		if _, is := child.(*east.TableHeader); is {
			style = r.look.Strong
			header = header || len(rows) == 0
		}
		row := make([]text.Line, 0, 8)
		for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
			// Markdown table syntax cannot express a line break inside a cell. An
			// empty cell can still produce no line, so make that case an empty one.
			lines := r.inline(cell, style)
			if len(lines) == 0 {
				row = append(row, nil)
				continue
			}
			row = append(row, lines[0])
		}
		rows = append(rows, row)
	}
	return rows, header
}

// column is what goes between two cells: the look's bar where it has one, and room
// where it does not — because two columns run together is worse than a table with no
// lines in it.
func (r *renderer) column() string {
	if r.look.Glyphs.Bar == "" {
		return "  "
	}
	return " " + r.look.Glyphs.Bar + " "
}
