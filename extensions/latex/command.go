package latex

import (
	"errors"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func (p *formulaParser) control(context formulaContext) (box, error) {
	if p.at == len(p.source) {
		return box{}, errors.New("source ends with an incomplete control sequence")
	}
	start := p.at - 1
	value, width := utf8.DecodeRuneInString(p.source[p.at:])
	if isFormulaSpace(value) {
		p.at += width
		return atom(" ", context.cell), nil
	}
	if value == '(' || value == ')' || value == '[' || value == ']' {
		return box{}, errors.New("source must not contain math delimiters")
	}
	for p.at < len(p.source) && controlLetter(p.source[p.at]) {
		p.at++
	}
	if p.at == start+1 {
		p.at += width
	}
	name := p.source[start:p.at]
	if inner, ok := argumentContext(name); ok {
		inner.cell = context.cell.Merge(inner.cell)
		if inner.mode == mathMode && context.mode == operatorMode {
			inner.mode = operatorMode
		}
		return p.argument(inner)
	}
	return p.command(name, context)
}

func controlLetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func (p *formulaParser) command(name string, context formulaContext) (box, error) {
	switch name {
	case `\frac`, `\dfrac`, `\tfrac`:
		return p.fraction(context)
	case `\binom`:
		return p.binomial(context)
	case `\stackrel`:
		return p.stackedRelation(context)
	case `\sqrt`:
		return p.squareRoot(context)
	case `\overline`:
		content, err := p.argument(context)
		return overlined(content, p.look.Glyphs, p.look.Rule), err
	case `\,`, `\:`, `\;`:
		return atom(" ", context.cell), nil
	case `\quad`:
		return atom("  ", context.cell), nil
	case `\qquad`:
		return atom("    ", context.cell), nil
	case `\!`:
		return box{}, nil
	default:
		return p.namedMacro(name, context)
	}
}

func (p *formulaParser) fraction(context formulaContext) (box, error) {
	numerator, denominator, err := p.pair(context)
	if err != nil {
		return box{}, err
	}
	return stack(numerator, denominator, true, p.look.Glyphs, p.look.Rule), nil
}

func (p *formulaParser) binomial(context formulaContext) (box, error) {
	top, bottom, err := p.pair(context)
	if err != nil {
		return box{}, err
	}
	return delimited(stack(top, bottom, false, p.look.Glyphs, p.look.Rule), p.look.Glyphs.Left, p.look.Glyphs.Right, context.cell), nil
}

func (p *formulaParser) stackedRelation(context formulaContext) (box, error) {
	annotation, base, err := p.pair(context)
	if err != nil {
		return box{}, err
	}
	return annotated(base, annotation), nil
}

func (p *formulaParser) squareRoot(context formulaContext) (box, error) {
	p.skipTrivia(mathMode)
	var index box
	if p.at < len(p.source) && p.source[p.at] == '[' {
		p.at++
		var err error
		index, err = p.sequence(']', context)
		if err != nil {
			return box{}, err
		}
	}
	content, err := p.argument(context)
	if err != nil {
		return box{}, err
	}
	return indexed(radical(content, p.look.Glyphs, context.cell, p.look.Rule), index), nil
}

func argumentContext(name string) (formulaContext, bool) {
	switch name {
	case `\textbf`:
		return formulaContext{cell: grid.Style{Attr: grid.Bold}, mode: textMode}, true
	case `\textit`:
		return formulaContext{cell: grid.Style{Attr: grid.Italic}, mode: textMode}, true
	case `\texttt`, `\textsf`, `\textcal`, `\textdefault`, `\textbb`, `\textfrak`,
		`\textscr`, `\textregular`:
		return formulaContext{mode: textMode}, true
	case `\operatorname`:
		return formulaContext{mode: operatorMode}, true
	case `\mathbf`:
		return formulaContext{cell: grid.Style{Attr: grid.Bold}, mode: mathMode}, true
	case `\mathit`:
		return formulaContext{cell: grid.Style{Attr: grid.Italic}, mode: mathMode}, true
	case `\mathtt`, `\mathsf`, `\mathcal`, `\mathdefault`, `\mathbb`, `\mathfrak`, `\mathscr`, `\mathregular`:
		return formulaContext{mode: mathMode}, true
	default:
		return formulaContext{}, false
	}
}
