package markdown

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	gparser "github.com/yuin/goldmark/parser"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Render normalizes invalid UTF-8 and NUL before parsing so every syntax uses the
// same characters and offsets. Extension failures retain readable blocks alongside
// errors. Live and final snapshots share parsing; source revision and finalization
// belong to the stream document. ErrBlockNestingLimit returns no blocks because
// the parse was stopped before rendering.
func Render(source string, look Look) ([]Block, error) {
	return render(source, look, nil)
}

func render(source string, look Look, context gparser.Context) ([]Block, error) {
	if source == "" {
		return nil, nil
	}
	look = cloneLook(look)
	r := &renderer{look: look, source: []byte(normalizeSource(source))}
	root, err := parse(r.source, context)
	if err != nil {
		return nil, err
	}
	r.render(root, frame{body: look.Text})
	return r.blocks, errors.Join(r.errors...)
}

type frame struct {
	indent int
	rails  *prefixMark
	marker *prefixMark
	body   grid.Style
	tight  bool
	blocks *[]Block
	// Nested extension failures identify their containing public result block.
	topBlock *int
	// Nested blocks inherit list tightness so a tight item cannot become loose
	// merely because it contains another list.
	dense bool
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
	b.prefix = blockPrefix{indent: in.indent, rails: in.rails}
	b.blankBefore = !in.tight
	*in.blocks = append(*in.blocks, b)
}

func (r *renderer) blockIndex(in frame) int {
	if in.topBlock != nil {
		return *in.topBlock
	}
	return len(r.blocks)
}

type renderAction struct {
	node  ast.Node
	frame frame
}

// Markdown nesting is untrusted input, so an explicit action stack bounds
// goroutine stack usage independently of document depth.
func (r *renderer) render(root ast.Node, in frame) {
	in.blocks = &r.blocks
	var stack []renderAction
	r.scheduleChildren(&stack, root, in)
	for len(stack) > 0 {
		last := len(stack) - 1
		action := stack[last]
		stack = stack[:last]
		r.block(action.node, action.frame, &stack)
	}
}

// Reverse scheduling preserves source order on the action stack. Tightness
// belongs to each child's position among its siblings.
func (r *renderer) scheduleChildren(stack *[]renderAction, n ast.Node, in frame) {
	for child := n.LastChild(); child != nil; child = child.PreviousSibling() {
		at := in
		first := child.PreviousSibling() == nil
		at.tight = (in.tight && first) || (in.dense && !first)
		*stack = append(*stack, renderAction{node: child, frame: at})
	}
}

// Raw HTML is omitted because interpreting tags would add a second markup engine
// and printing them verbatim would expose syntax as document content.
func (r *renderer) block(n ast.Node, in frame, stack *[]renderAction) {
	switch node := n.(type) {
	case *ast.Heading:
		r.push(Block{body: textBody(r.inline(node, r.look.heading(node.Level)))}, in)
	case *ast.Paragraph, *ast.TextBlock:
		r.push(Block{body: textBody(r.inline(node, in.body))}, in)
	case *ast.List:
		r.list(node, in, stack)
	case *ast.ListItem:
		r.listItem(node, in, stack)
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
		r.push(Block{body: ruleBody{glyph: text.Span{Text: r.look.Glyphs.Divider, Style: r.look.Rule}}}, in)
	case *east.Table:
		r.table(node, in)
	case *ast.HTMLBlock:
	default:
		r.scheduleChildren(stack, n, in)
	}
}

func (r *renderer) list(n *ast.List, in frame, stack *[]renderAction) {
	// Zero is a valid ordered-list start; only unordered lists omit numbering.
	number := max(n.Start, 0)
	index := n.ChildCount() - 1
	for item := n.LastChild(); item != nil; item = item.PreviousSibling() {
		marker := r.bullet(n, ordinal(number, index))
		inner := in
		inner.marker = inner.place(marker)
		// The first item inherits spacing from the enclosing frame; later items use the
		// list's own tightness. Otherwise a tight list would join the preceding paragraph.
		inner.tight = (index == 0 && in.tight) || (index > 0 && n.IsTight)
		inner.dense = n.IsTight
		*stack = append(*stack, renderAction{node: item, frame: inner})
		index--
	}
}

func (r *renderer) listItem(n *ast.ListItem, in frame, stack *[]renderAction) {
	item := &listItem{
		prefix: blockPrefix{indent: in.indent, rails: in.rails},
		marker: in.marker,
	}
	index := r.blockIndex(in)
	*in.blocks = append(*in.blocks, Block{body: item, blankBefore: !in.tight})
	in.blocks, in.topBlock, in.marker = &item.body.blocks, &index, nil
	r.scheduleChildren(stack, n, in)
}

func ordinal(start, index int) int {
	if index <= 0 {
		return start
	}
	return layout.Translate(start, index)
}

// unmarkedIndent is a bullet's width when the look draws none; unruledColumnGap
// separates table columns when it draws no bar.
const (
	unmarkedIndent   = "  "
	unruledColumnGap = "  "
)

func (r *renderer) bullet(n *ast.List, number int) text.Span {
	if n.IsOrdered() {
		return text.Span{Text: strconv.Itoa(number) + ". ", Style: r.look.Marker}
	}
	if r.look.Glyphs.Bullet == "" {
		// No bullet to draw, and the indent still has to happen or a list reads as a
		// paragraph with odd line breaks.
		return text.Span{Text: unmarkedIndent, Style: r.look.Marker}
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
		r.errors = append(r.errors, &ExtensionError{Block: r.blockIndex(in), Extension: kind, Info: info, Err: err})
	}
	if child == nil {
		var lines []text.Line
		for line := range strings.SplitSeq(source, "\n") {
			lines = append(lines, text.Of(line, r.look.Block))
		}
		child = text.NewBlock(text.BlockConfig{Lines: lines, Wrap: kind != DisplayMath})
	}
	r.push(Block{body: embeddedBody{child: child}}, in)
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

// Character references cannot introduce Markdown structure. Their whitespace must
// survive the single-line projection so terminal control filtering cannot join words.
var inlineWhitespace = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\f", " ")

const (
	maxEntityLength        = len("&CounterClockwiseContourIntegral;")
	maxDecimalEntityDigits = 7
	maxHexEntityDigits     = 6
)

// CommonMark normalization precedes parsing so code, references and extension
// bodies share the same characters and all parser offsets address that source.
func normalizeSource(source string) string {
	const replacement = string(utf8.RuneError)
	source = strings.ToValidUTF8(source, replacement)
	source = strings.ReplaceAll(source, "\x00", replacement)
	return text.NormalizeLineEndings(source)
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
		body: &table{
			rows: rows, aligns: columnAlignments(n.Alignments), header: header,
			separator: text.Of(r.column(), r.look.Rail).Pad(0, layout.Start),
			divider:   text.Of(r.look.Glyphs.Divider, r.look.Rule).Pad(0, layout.Start),
			rail:      r.look.Rail, rule: r.look.Rule,
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
		var row []text.Line
		for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
			// Markdown table syntax cannot express a line break inside a cell, and
			// inline content always yields at least one, possibly empty, line.
			row = append(row, r.inline(cell, style)[0].Pad(0, layout.Start))
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
		return unruledColumnGap
	}
	return " " + r.look.Glyphs.Bar + " "
}
