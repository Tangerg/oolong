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
		return box{}, &parseError{message: err.Error()}
	}
	return out, nil
}

// parse contains the recovery boundary around the external parser. That parser
// represents malformed untrusted input with panics in several paths; none of those
// conventions cross the module API.
func parse(source string) (node ast.Node, err error) {
	if validationErr := validateSource(source); validationErr != nil {
		return nil, validationErr
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			node, err = nil, parseFailure(recovered)
		}
	}()
	node, err = golatex.ParseExpr("$" + braceNumericScripts(source) + "$")
	if err != nil {
		return nil, &parseError{message: err.Error()}
	}
	return node, nil
}

// braceNumericScripts adapts TeX's script tokens to the external parser's Go
// scanner. text/scanner accepts underscores inside numeric literals, while TeX
// always treats an underscore as a new script. Making the numeric atom explicit
// keeps x^2_1 equivalent to x^{2}_{1} without teaching the layout layer about a
// dependency's tokenization.
func braceNumericScripts(source string) string {
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
		start := at
		for at < len(source) && source[at] >= '0' && source[at] <= '9' {
			at++
		}
		if start == at {
			continue
		}
		out.WriteByte('{')
		out.WriteString(source[start:at])
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

func validateSource(source string) error {
	if !utf8.ValidString(source) {
		return &parseError{message: "source is not valid UTF-8"}
	}
	depth := 0
	scriptDepth := 0
	escaped := false
	for _, r := range source {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return &parseError{message: fmt.Sprintf("source contains control character %U", r)}
		}
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			scriptDepth = 0
			continue
		}
		if r == '^' || r == '_' {
			scriptDepth++
			if scriptDepth > 256 {
				return &parseError{message: "source nesting exceeds 256 scripts"}
			}
		} else {
			scriptDepth = 0
		}
		switch r {
		case '{':
			depth++
			if depth > 256 {
				return &parseError{message: "source nesting exceeds 256 groups"}
			}
		case '}':
			depth--
			if depth < 0 {
				return &parseError{message: "source has an unmatched closing brace"}
			}
		}
	}
	if depth != 0 {
		return &parseError{message: "source has an unclosed group"}
	}
	return nil
}

type formulaRenderer struct{ look Look }

func (r *formulaRenderer) node(node ast.Node, style grid.Style) (box, error) {
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
		return box{}, nil
	default:
		return box{}, fmt.Errorf("unsupported syntax %T", node)
	}
}

func (r *formulaRenderer) sequence(nodes ast.List, style grid.Style) (box, error) {
	parts := make([]box, 0, len(nodes))
	for i := 0; i < len(nodes); {
		node := nodes[i]
		if _, sub := node.(*ast.Sub); sub {
			return box{}, errors.New("subscript has no base")
		}
		if _, sup := node.(*ast.Sup); sup {
			return box{}, errors.New("superscript has no base")
		}

		base, err := r.node(node, style)
		if err != nil {
			return box{}, err
		}
		i++
		var superscript, subscript box
		var hasSuperscript, hasSubscript bool
		for i < len(nodes) {
			switch script := nodes[i].(type) {
			case *ast.Sup:
				if hasSuperscript {
					return box{}, errors.New("base has two superscripts")
				}
				hasSuperscript = true
				superscript, err = r.node(script.Node, style)
			case *ast.Sub:
				if hasSubscript {
					return box{}, errors.New("base has two subscripts")
				}
				hasSubscript = true
				subscript, err = r.node(script.Node, style)
			default:
				parts = append(parts, scripted(base, superscript, subscript))
				goto next
			}
			if err != nil {
				return box{}, err
			}
			i++
		}
		parts = append(parts, scripted(base, superscript, subscript))
	next:
	}
	return horizontal(parts...), nil
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
	return scripted(base, annotation, box{}), nil
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
		root = scripted(root, index, box{})
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

func (r *formulaRenderer) namedMacro(name string, args ast.List, style grid.Style) (box, error) {
	if len(args) > 0 {
		label := strings.TrimPrefix(name, `\`)
		content, err := r.node(args[len(args)-1], style)
		if err != nil {
			return box{}, err
		}
		return horizontal(atom(label, style), content), nil
	}

	symbol, err := terminalSymbol(name, r.look.Glyphs.Plain)
	if err != nil {
		// Function names such as sin are lettered operators rather than font glyphs.
		symbol = strings.TrimPrefix(name, `\`)
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

func terminalSymbol(name string, plain bool) (string, error) {
	if plain {
		return asciiSymbol(name), nil
	}
	return symbolsForTerminal.resolve(name)
}

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
