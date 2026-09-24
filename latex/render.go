package latex

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	golatex "codeberg.org/go-latex/latex"
	"codeberg.org/go-latex/latex/ast"
	"codeberg.org/go-latex/latex/drawtex"
	"codeberg.org/go-latex/latex/font"
	"codeberg.org/go-latex/latex/font/ttf"
	"codeberg.org/go-latex/latex/mtex/symbols"

	"github.com/Tangerg/oolong/core/grid"
)

func render(source string, look Look) (out box, err error) {
	node, err := parse(source)
	if err != nil {
		return box{}, err
	}
	r := formulaRenderer{look: look}
	out, err = r.node(node, look.Text)
	if err != nil {
		return box{}, explain(err.Error())
	}
	return out, nil
}

// parse contains the recovery boundary around the external parser. That parser
// represents malformed untrusted input with panics in several paths; none of those
// conventions cross the module API.
func parse(source string) (node ast.Node, err error) {
	if !utf8.ValidString(source) {
		return nil, &parseError{message: "source is not valid UTF-8"}
	}
	source = withoutComments(source)
	if validationErr := validateSource(source); validationErr != nil {
		return nil, validationErr
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			node, err = nil, parseFailure(recovered)
		}
	}()
	node, err = golatex.ParseExpr(mathDelimiter + braceScriptAtoms(spacedTextArguments(source)) + mathDelimiter)
	if err != nil {
		return nil, explain(err.Error())
	}
	return node, nil
}

// textModeMacros take an argument the caller wrote as text. Everything else here is
// mathematics, where the space between two tokens is not a character and TeX is right
// to drop it.
var textModeMacros = []string{
	`\textbf`, `\textit`, `\texttt`, `\textsf`, `\textcal`, `\textdefault`,
	`\textbb`, `\textfrak`, `\textscr`, `\textregular`, `\operatorname`,
}

// spacedTextArguments makes the spaces of a text-mode argument survive a math-mode
// parser.
//
// The expression reaches that parser in math mode, where a space between two tokens
// is dropped before an AST exists — so a font applied afterwards cannot put back what
// the parse never saw, and two words come out as one. Each space is written as the
// escape the parser does carry and this package already renders as one column. Like
// [braceScriptAtoms], this is the boundary that owns the difference between the
// source the caller wrote and the source the parser reads.
func spacedTextArguments(source string) string {
	var out strings.Builder
	out.Grow(len(source))
	for at := 0; at < len(source); {
		name := textModeMacroAt(source, at)
		if name == "" {
			out.WriteByte(source[at])
			at++
			continue
		}
		out.WriteString(name)
		at += len(name)
		if at >= len(source) || source[at] != '{' {
			continue
		}
		end, closed := groupEnd(source, at)
		if !closed {
			// Unbalanced source is validateSource's to report, with the braces the
			// caller actually wrote.
			continue
		}
		out.WriteByte('{')
		out.WriteString(spacesAsAtoms(source[at+1 : end]))
		out.WriteByte('}')
		at = end + 1
	}
	return out.String()
}

// textModeMacroAt is the text-mode macro beginning at, or empty. A longer name is a
// different macro: \textbfx is not \textbf.
func textModeMacroAt(source string, at int) string {
	if source[at] != '\\' || escapedAt(source, at) {
		return ""
	}
	for _, name := range textModeMacros {
		if !strings.HasPrefix(source[at:], name) {
			continue
		}
		rest := source[at+len(name):]
		if rest == "" || !isMacroLetter(rest[0]) {
			return name
		}
	}
	return ""
}

func isMacroLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func groupEnd(source string, open int) (int, bool) {
	depth := 0
	for at := open; at < len(source); at++ {
		if escapedAt(source, at) {
			continue
		}
		switch source[at] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return at, true
			}
		}
	}
	return 0, false
}

func spacesAsAtoms(argument string) string {
	var out strings.Builder
	out.Grow(len(argument))
	for at := range len(argument) {
		if argument[at] == ' ' && !escapedAt(argument, at) {
			out.WriteString(`\,`)
			continue
		}
		out.WriteByte(argument[at])
	}
	return out.String()
}

func braceScriptAtoms(source string) string {
	var out strings.Builder
	out.Grow(len(source))
	for at := 0; at < len(source); {
		marker := source[at] == '^' || source[at] == '_'
		if !marker || escapedAt(source, at) {
			out.WriteByte(source[at])
			at++
			continue
		}

		out.WriteByte(source[at])
		at++
		for at < len(source) && (source[at] == ' ' || source[at] == '\t' || source[at] == '\r' || source[at] == '\n') {
			at++
		}
		if at == len(source) || source[at] == '{' || source[at] == '\\' {
			continue
		}
		_, size := utf8.DecodeRuneInString(source[at:])
		out.WriteByte('{')
		out.WriteString(source[at : at+size])
		at += size
		out.WriteByte('}')
	}
	return out.String()
}

func escapedAt(source string, at int) bool {
	backslashes := 0
	for at > 0 && source[at-1] == '\\' {
		backslashes++
		at--
	}
	return backslashes%2 != 0
}

// The parser treats an unescaped percent as a comment through LF (including CR).
// Remove that trivia once so validation and script rewriting see the same tokens.
func withoutComments(source string) string {
	var out strings.Builder
	for at := 0; at < len(source); at++ {
		if source[at] == '\\' && at+1 < len(source) {
			out.WriteString(source[at : at+2])
			at++
			continue
		}
		if source[at] == '%' {
			for at < len(source) && source[at] != '\n' {
				at++
			}
			if at == len(source) {
				break
			}
		}
		out.WriteByte(source[at])
	}
	return out.String()
}

func validateSource(source string) error {
	if !utf8.ValidString(source) {
		return &parseError{message: "source is not valid UTF-8"}
	}
	var groups nesting
	escaped := false
	for at, r := range source {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return &parseError{message: fmt.Sprintf("source contains control character %U", r)}
		}
		if escaped {
			escaped = false
			continue
		}
		if err := refused(source, at, r); err != nil {
			return err
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if err := groups.step(r); err != nil {
			return err
		}
	}
	return groups.closed()
}

// refused is why r may not stand unescaped at at, or nil.
func refused(source string, at int, r rune) error {
	switch {
	case r == '$':
		// The same rule as \( and \[, for the same reason and one more: this package
		// delimits the expression with $ before handing it to the parser, so a $ of
		// the caller's would make every position the parser reports ambiguous between
		// their source and the wrapper around it.
		return &parseError{message: "source must not contain math delimiters"}
	case r == '\\' && at+1 < len(source) && strings.ContainsRune("([])", rune(source[at+1])):
		return &parseError{message: "source must not contain math delimiters"}
	case r == '^' || r == '_':
		rest := strings.TrimLeft(source[at+1:], " \t\r\n")
		if rest == "" || strings.ContainsRune("^_}$%", rune(rest[0])) {
			return &parseError{message: "script has no atom"}
		}
	}
	return nil
}

// maxGroups bounds how deep a source may nest. Braces and brackets share the budget
// because what it bounds is the tree the parser will build, not the spelling.
const maxGroups = 256

type nesting struct{ braces, brackets int }

func (n *nesting) step(r rune) error {
	switch r {
	case '[':
		n.brackets++
	case ']':
		n.brackets = max(0, n.brackets-1)
	case '{':
		n.braces++
	case '}':
		n.braces--
		if n.braces < 0 {
			return &parseError{message: "source has an unmatched closing brace"}
		}
	}
	if n.braces+n.brackets > maxGroups {
		return &parseError{message: fmt.Sprintf("source nesting exceeds %d groups", maxGroups)}
	}
	return nil
}

func (n *nesting) closed() error {
	if n.braces != 0 {
		return &parseError{message: "source has an unclosed group"}
	}
	return nil
}

type formulaRenderer struct {
	look  Look
	depth int
}

func (r *formulaRenderer) node(node ast.Node, style grid.Style) (box, error) {
	if r.depth >= 256 {
		return box{}, errors.New("formula nesting exceeds 256 nodes")
	}
	r.depth++
	defer func() { r.depth-- }()
	switch node := node.(type) {
	case ast.List:
		return r.sequence(node, style)
	case *ast.MathExpr:
		return r.sequence(node.List, style)
	case *ast.Arg:
		return r.sequence(node.List, style)
	case *ast.OptArg:
		return r.sequence(node.List, style)
	case *ast.Word:
		return atom(node.Text, style), nil
	case *ast.Literal:
		return atom(node.Text, style), nil
	case *ast.Symbol:
		value := r.plainSymbol(node.Text)
		if symbols.IsSpaced(node.Text) {
			value = " " + value + " "
		}
		return atom(value, style), nil
	case *ast.Macro:
		return r.macro(node, style)
	case *ast.Sub:
		return r.node(node.Node, style)
	case *ast.Sup:
		return r.node(node.Node, style)
	case nil:
		return box{}, errors.New("missing formula atom")
	default:
		return box{}, fmt.Errorf("unsupported syntax %T", node)
	}
}

func (r *formulaRenderer) sequence(nodes ast.List, style grid.Style) (box, error) {
	parts := make([]box, 0, len(nodes))
	for at := 0; at < len(nodes); {
		if err := unattached(nodes[at]); err != nil {
			return box{}, err
		}
		base, err := r.node(nodes[at], style)
		if err != nil {
			return box{}, err
		}
		attached, next, err := r.scriptsAfter(nodes, at+1, style)
		if err != nil {
			return box{}, err
		}
		parts = append(parts, scripted(base, attached.sup, attached.sub))
		at = next
	}
	return horizontal(parts...), nil
}

// unattached is the error for a script with nothing in front of it to attach to.
func unattached(node ast.Node) error {
	switch node.(type) {
	case *ast.Sub:
		return errors.New("subscript has no base")
	case *ast.Sup:
		return errors.New("superscript has no base")
	}
	return nil
}

// scripts are what one base carries. Each may be given once, because a base with two
// superscripts is the source saying two things about one position.
type scripts struct {
	sup, sub       box
	hasSup, hasSub bool
}

// scriptsAfter renders the run of scripts starting at at, and reports where the next
// base begins.
func (r *formulaRenderer) scriptsAfter(nodes ast.List, at int, style grid.Style) (scripts, int, error) {
	var attached scripts
	for ; at < len(nodes); at++ {
		var err error
		switch script := nodes[at].(type) {
		case *ast.Sup:
			if attached.hasSup {
				return scripts{}, 0, errors.New("base has two superscripts")
			}
			attached.hasSup = true
			attached.sup, err = r.node(script.Node, style)
		case *ast.Sub:
			if attached.hasSub {
				return scripts{}, 0, errors.New("base has two subscripts")
			}
			attached.hasSub = true
			attached.sub, err = r.node(script.Node, style)
		default:
			return attached, at, nil
		}
		if err != nil {
			return scripts{}, 0, err
		}
	}
	return attached, at, nil
}

func (r *formulaRenderer) macro(macro *ast.Macro, style grid.Style) (box, error) {
	if macro.Name == nil {
		return box{}, errors.New("macro has no name")
	}
	name := macro.Name.Name
	args := macro.Args
	if rendered, handled, err := r.structuralMacro(name, args, style); handled {
		return rendered, err
	}
	if rendered, handled, err := r.appearanceMacro(name, args, style); handled {
		return rendered, err
	}
	return r.namedMacro(name, args, style)
}

func (r *formulaRenderer) structuralMacro(name string, args ast.List, style grid.Style) (box, bool, error) {
	switch name {
	case `\frac`, `\dfrac`, `\tfrac`:
		rendered, err := r.fraction(name, args, style)
		return rendered, true, err
	case `\binom`:
		rendered, err := r.binomial(name, args, style)
		return rendered, true, err
	case `\stackrel`:
		rendered, err := r.stackedRelation(name, args, style)
		return rendered, true, err
	case `\sqrt`:
		rendered, err := r.squareRoot(name, args, style)
		return rendered, true, err
	case `\overline`:
		rendered, err := r.overline(name, args, style)
		return rendered, true, err
	default:
		return box{}, false, nil
	}
}

func (r *formulaRenderer) fraction(name string, args ast.List, style grid.Style) (box, error) {
	if len(args) != 2 {
		return box{}, fmt.Errorf("%s needs numerator and denominator", name)
	}
	numerator, err := r.node(args[0], style)
	if err != nil {
		return box{}, err
	}
	denominator, err := r.node(args[1], style)
	if err != nil {
		return box{}, err
	}
	return stack(numerator, denominator, true, r.look.Glyphs, r.look.Rule), nil
}

func (r *formulaRenderer) binomial(name string, args ast.List, style grid.Style) (box, error) {
	if len(args) != 2 {
		return box{}, fmt.Errorf("%s needs two arguments", name)
	}
	top, err := r.node(args[0], style)
	if err != nil {
		return box{}, err
	}
	bottom, err := r.node(args[1], style)
	if err != nil {
		return box{}, err
	}
	return delimited(
		stack(top, bottom, false, r.look.Glyphs, r.look.Rule),
		r.look.Glyphs.Left, r.look.Glyphs.Right, style,
	), nil
}

func (r *formulaRenderer) stackedRelation(name string, args ast.List, style grid.Style) (box, error) {
	if len(args) != 2 {
		return box{}, fmt.Errorf("%s needs an annotation and a base", name)
	}
	annotation, err := r.node(args[0], style)
	if err != nil {
		return box{}, err
	}
	base, err := r.node(args[1], style)
	if err != nil {
		return box{}, err
	}
	return annotated(base, annotation), nil
}

func (r *formulaRenderer) squareRoot(name string, args ast.List, style grid.Style) (box, error) {
	if len(args) == 0 {
		return box{}, fmt.Errorf("%s needs a radicand", name)
	}
	content, err := r.node(args[len(args)-1], style)
	if err != nil {
		return box{}, err
	}
	root := radical(content, r.look.Glyphs, style, r.look.Rule)
	if len(args) == 2 {
		index, err := r.node(args[0], style)
		if err != nil {
			return box{}, err
		}
		root = indexed(root, index)
	}
	return root, nil
}

func (r *formulaRenderer) overline(name string, args ast.List, style grid.Style) (box, error) {
	if len(args) != 1 {
		return box{}, fmt.Errorf("%s needs one argument", name)
	}
	content, err := r.node(args[0], style)
	if err != nil {
		return box{}, err
	}
	return overlined(content, r.look.Glyphs, r.look.Rule), nil
}

func (r *formulaRenderer) appearanceMacro(name string, args ast.List, style grid.Style) (box, bool, error) {
	switch name {
	case `\mathbf`, `\textbf`:
		rendered, err := r.styledArg(name, args, style.Merge(grid.Style{Attr: grid.Bold}))
		return rendered, true, err
	case `\mathit`, `\textit`:
		rendered, err := r.styledArg(name, args, style.Merge(grid.Style{Attr: grid.Italic}))
		return rendered, true, err
	case `\mathtt`, `\texttt`, `\mathsf`, `\textsf`, `\mathcal`, `\textcal`,
		`\mathdefault`, `\textdefault`, `\mathbb`, `\textbb`, `\mathfrak`, `\textfrak`,
		`\mathscr`, `\textscr`, `\mathregular`, `\textregular`:
		rendered, err := r.styledArg(name, args, style)
		return rendered, true, err
	case `\operatorname`:
		rendered, err := r.styledArg(name, args, style)
		return rendered, true, err
	case `\exp`:
		content, err := r.styledArg(name, args, style)
		if err != nil {
			return box{}, true, err
		}
		return horizontal(atom("exp ", style), content), true, nil
	case `\ `, `\,`, `\:`, `\;`:
		return atom(" ", style), true, nil
	case `\quad`:
		return atom("  ", style), true, nil
	case `\qquad`:
		return atom("    ", style), true, nil
	case `\!`:
		return box{}, true, nil
	case `\hspace`:
		return atom(" ", style), true, nil
	default:
		return box{}, false, nil
	}
}

// letteredOperators are TeX's log-like operators: control sequences whose rendering
// is their own letters rather than a glyph. They are named rather than derived
// because "has no glyph" is also true of every control sequence this package does not
// implement, and the two must not be confused.
var letteredOperators = map[string]bool{
	`\arccos`: true, `\arcsin`: true, `\arctan`: true, `\arg`: true,
	`\cos`: true, `\cosh`: true, `\cot`: true, `\coth`: true, `\csc`: true,
	`\deg`: true, `\det`: true, `\dim`: true, `\gcd`: true, `\hom`: true,
	`\inf`: true, `\ker`: true, `\lg`: true, `\lim`: true, `\liminf`: true,
	`\limsup`: true, `\ln`: true, `\log`: true, `\max`: true, `\min`: true,
	`\Pr`: true, `\sec`: true, `\sin`: true, `\sinh`: true, `\sup`: true,
	`\tan`: true, `\tanh`: true,
}

func (r *formulaRenderer) namedMacro(name string, args ast.List, style grid.Style) (box, error) {
	if len(args) > 0 {
		return box{}, fmt.Errorf("unsupported macro %s", name)
	}

	// Whether this package implements a control sequence is a fact about the sequence,
	// decided here and once. Letting the ASCII spelling decide it as well meant a look
	// with Plain set implemented everything there is: \bf came back as the letters
	// "bf" and no error, because that is what is left of its name without the
	// backslash.
	symbol, err := symbolsForTerminal.resolve(name)
	switch {
	case err != nil:
		// A lettered operator is spelled with its own letters and has no glyph, so
		// failing to find one is how it is recognised. Only the ones TeX defines,
		// though: stripping the backslash from anything else turns a control sequence
		// this package does not implement into the letters it happens to be made of,
		// and reports success for input it did not render.
		if !letteredOperators[name] {
			return box{}, fmt.Errorf("unsupported macro %s", name)
		}
		symbol = strings.TrimPrefix(name, `\`)
	case grid.ClusterWidth(symbol) == 0:
		return box{}, fmt.Errorf("unsupported combining accent %s", name)
	case r.look.Glyphs.Plain:
		// Plain changes how a symbol this package has is spelled and nothing else.
		symbol = asciiSymbol(name)
	}
	if symbols.IsSpaced(name) {
		symbol = " " + symbol + " "
	}
	return atom(symbol, style), nil
}

func (r *formulaRenderer) styledArg(name string, args ast.List, style grid.Style) (box, error) {
	if len(args) != 1 {
		return box{}, fmt.Errorf("%s needs one argument", name)
	}
	return r.node(args[0], style)
}

func (r *formulaRenderer) plainSymbol(value string) string {
	if r.look.Glyphs.Plain {
		return value
	}
	if value == "-" {
		return "−"
	}
	return value
}

type symbolResult struct {
	value string
	err   error
}

// symbolResolver memoises only symbols the dependency successfully resolves.
// Unknown macro names come from untrusted formula source and are deliberately not
// retained: caching failures would turn distinct invalid inputs into process-lifetime
// global state.
type symbolResolver struct{ cache sync.Map }

var symbolsForTerminal symbolResolver

func (r *symbolResolver) resolve(name string) (string, error) {
	if cached, ok := r.cache.Load(name); ok {
		if value, valid := cached.(string); valid {
			return value, nil
		}
		r.cache.Delete(name)
	}
	result := resolveSymbol(name)
	if result.err == nil {
		actual, _ := r.cache.LoadOrStore(name, result.value)
		if value, valid := actual.(string); valid {
			return value, nil
		}
		r.cache.Delete(name)
	}
	return result.value, result.err
}

func resolveSymbol(name string) (result symbolResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result.err = parseFailure(recovered)
		}
	}()
	canvas := drawtex.New()
	backend := ttf.New(canvas)
	backend.RenderGlyph(0, 0, font.Font{Name: "default", Type: "rm", Size: 12}, name, 72)
	for _, operation := range canvas.Ops() {
		glyph, ok := operation.(drawtex.GlyphOp)
		if ok && glyph.Glyph.Symbol != "" {
			return symbolResult{value: glyph.Glyph.Symbol}
		}
	}
	return symbolResult{err: fmt.Errorf("no terminal glyph for %s", name)}
}

func asciiSymbol(name string) string {
	switch name {
	case `\cdot`, `\ast`, `\star`, `\bullet`:
		return "*"
	case `\times`:
		return "x"
	case `\div`:
		return "/"
	case `\pm`:
		return "+/-"
	case `\mp`:
		return "-/+"
	case `\leq`:
		return "<="
	case `\geq`:
		return ">="
	case `\neq`:
		return "!="
	case `\rightarrow`, `\longrightarrow`, `\Rightarrow`, `\Longrightarrow`:
		return "->"
	case `\leftarrow`, `\longleftarrow`, `\Leftarrow`, `\Longleftarrow`:
		return "<-"
	case `\leftrightarrow`, `\longleftrightarrow`, `\Leftrightarrow`, `\Longleftrightarrow`:
		return "<->"
	case `\in`:
		return "in"
	case `\notin`:
		return "notin"
	case `\infty`:
		return "infinity"
	}
	return strings.TrimPrefix(name, `\`)
}
