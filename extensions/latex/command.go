package latex

import (
	"errors"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func (p *formulaParser) control(style formulaStyle) (box, error) {
	if p.at == len(p.source) {
		return box{}, errors.New("source ends with an incomplete control sequence")
	}
	start := p.at - 1
	value, width := utf8.DecodeRuneInString(p.source[p.at:])
	if isFormulaSpace(value) {
		p.at += width
		return atom(" ", style.cell), nil
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
	if appearance, textSpaces, ok := controlAppearance(name); ok {
		return p.argument(formulaStyle{cell: style.cell.Merge(appearance), textSpaces: textSpaces})
	}
	return p.command(name, style)
}

func controlLetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func (p *formulaParser) command(name string, style formulaStyle) (box, error) {
	switch name {
	case `\frac`, `\dfrac`, `\tfrac`:
		return p.fraction(style)
	case `\binom`:
		return p.binomial(style)
	case `\stackrel`:
		return p.stackedRelation(style)
	case `\sqrt`:
		return p.squareRoot(style)
	case `\overline`:
		content, err := p.argument(style)
		return overlined(content, p.look.Glyphs, p.look.Rule), err
	case `\exp`:
		content, err := p.argument(style)
		return horizontal(atom("exp ", style.cell), content), err
	case `\,`, `\:`, `\;`:
		return atom(" ", style.cell), nil
	case `\quad`:
		return atom("  ", style.cell), nil
	case `\qquad`:
		return atom("    ", style.cell), nil
	case `\!`:
		return box{}, nil
	default:
		return p.namedMacro(name, style.cell)
	}
}

func (p *formulaParser) fraction(style formulaStyle) (box, error) {
	numerator, denominator, err := p.pair(style)
	if err != nil {
		return box{}, err
	}
	return stack(numerator, denominator, true, p.look.Glyphs, p.look.Rule), nil
}

func (p *formulaParser) binomial(style formulaStyle) (box, error) {
	top, bottom, err := p.pair(style)
	if err != nil {
		return box{}, err
	}
	return delimited(stack(top, bottom, false, p.look.Glyphs, p.look.Rule), p.look.Glyphs.Left, p.look.Glyphs.Right, style.cell), nil
}

func (p *formulaParser) stackedRelation(style formulaStyle) (box, error) {
	annotation, base, err := p.pair(style)
	if err != nil {
		return box{}, err
	}
	return annotated(base, annotation), nil
}

func (p *formulaParser) squareRoot(style formulaStyle) (box, error) {
	p.skipTrivia(false)
	var index box
	if p.at < len(p.source) && p.source[p.at] == '[' {
		p.at++
		var err error
		index, err = p.sequence(']', style)
		if err != nil {
			return box{}, err
		}
	}
	content, err := p.argument(style)
	if err != nil {
		return box{}, err
	}
	return indexed(radical(content, p.look.Glyphs, style.cell, p.look.Rule), index), nil
}

func controlAppearance(name string) (grid.Style, bool, bool) {
	switch name {
	case `\textbf`:
		return grid.Style{Attr: grid.Bold}, true, true
	case `\textit`:
		return grid.Style{Attr: grid.Italic}, true, true
	case `\texttt`, `\textsf`, `\textcal`, `\textdefault`, `\textbb`, `\textfrak`,
		`\textscr`, `\textregular`, `\operatorname`:
		return grid.Style{}, true, true
	case `\mathbf`:
		return grid.Style{Attr: grid.Bold}, false, true
	case `\mathit`:
		return grid.Style{Attr: grid.Italic}, false, true
	case `\mathtt`, `\mathsf`, `\mathcal`, `\mathdefault`, `\mathbb`, `\mathfrak`, `\mathscr`, `\mathregular`:
		return grid.Style{}, false, true
	default:
		return grid.Style{}, false, false
	}
}
