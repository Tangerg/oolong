package markdown

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/util"
)

// Soft breaks become spaces because output width differs from source width.
// Hard breaks remain structural because the author explicitly requested them.
func (r *renderer) inline(n ast.Node, style grid.Style) []text.Line {
	var out inlineWriter

	var stack []inlineAction
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
	case *ast.CodeSpan:
		out.add(r.codeSpan(node), action.style.Merge(r.look.Code), link)
	case *ast.AutoLink:
		// What the document showed and where it goes are two things, and they differ
		// whenever the protocol was inferred rather than written: "www.example.com"
		// is what the reader wrote and "http://www.example.com" is what opens.
		label := string(node.Label(r.source))
		target := string(node.URL(r.source))
		if node.AutoLinkType == ast.AutoLinkEmail {
			// An address is not a destination: goldmark returns the bare address for
			// an email autolink, and a terminal has nothing to hand a mail client
			// without a scheme. The address is the whole of what an email autolink
			// says, so the destination is built from it.
			target = schemeMailto + ":" + label
		}
		if action.link == nil {
			link = newHyperlink(target, label).labelLink()
		}
		out.add(label, action.style.Merge(r.look.Link), link)
	case *ast.Image:
		// A picture is not something a row of cells can hold — see the graphics
		// package for what a terminal will take — so what is left is what it was
		// called and where it is.
		alt := r.plain(node)
		var destination hyperlink
		if action.link == nil {
			destination = newHyperlink(markdownText(string(node.Destination)), alt)
			link = destination.labelLink()
		}
		out.add("["+alt+"]", action.style.Merge(r.look.Link), link)
		r.target(out, destination, "", action.style)
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
		shown := r.plain(node)
		destination := newHyperlink(markdownText(string(node.Destination)), shown)
		words := destination.labelLink()
		*stack = append(*stack, inlineAction{
			target: true, destination: destination, shown: shown, style: action.style,
		})
		pushInlineChildren(stack, node, action.style.Merge(r.look.Link), &words)
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
	w.run.WriteString(inlineText(s))
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
	destination hyperlink
	shown       string
}

func pushInlineChildren(stack *[]inlineAction, n ast.Node, style grid.Style, link *string) {
	for child := n.LastChild(); child != nil; child = child.PreviousSibling() {
		*stack = append(*stack, inlineAction{node: child, style: style, link: link})
	}
}

// Hyperlinked words already carry their destination. Repeat the address only
// when its appearance is explicitly configured and it adds information, or when
// the label shows a different address and would otherwise be the only one seen.
func (r *renderer) target(out *inlineWriter, destination hyperlink, shown string, style grid.Style) {
	if destination.target == "" || destination.target == shown {
		return
	}
	if r.look.Target == (grid.Style{}) && !destination.disguised {
		return
	}
	out.add(" ("+destination.target+")", style.Merge(r.look.Target), destination.addressLink())
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
	var stack []ast.Node
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
		case *ast.AutoLink:
			b.Write(node.Label(r.source))
		default:
			pushNodes(node)
		}
	}
	return inlineText(b.String())
}

func inlineText(value string) string {
	if !strings.ContainsAny(value, "\r\n\f") {
		return value
	}
	return inlineWhitespace.Replace(value)
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
			if end := strings.IndexByte(source[i:min(i+maxEntityLength, len(source))], ';'); end >= 0 {
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
	digits, base, limit := name[1:], 10, maxDecimalEntityDigits
	if strings.HasPrefix(digits, "x") || strings.HasPrefix(digits, "X") {
		digits, base, limit = digits[1:], 16, maxHexEntityDigits
	}
	if len(digits) == 0 || len(digits) > limit {
		return "", false
	}
	codepoint, err := strconv.ParseUint(digits, base, 32)
	if err != nil {
		return "", false
	}
	// CommonMark maps numeric references directly to Unicode; HTML's legacy C1
	// remapping would turn control characters into different, visible text.
	if codepoint == 0 || codepoint > utf8.MaxRune {
		return string(utf8.RuneError), true
	}
	return string(rune(codepoint)), true
}

func (r *renderer) codeSpan(node *ast.CodeSpan) string {
	var out strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		textNode, ok := child.(*ast.Text)
		if !ok {
			continue
		}
		// normalizeSource has already turned every line ending into LF, and a
		// code span reads each one as a space.
		out.WriteString(strings.ReplaceAll(string(textNode.Segment.Value(r.source)), "\n", " "))
	}
	return out.String()
}
