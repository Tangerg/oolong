package latex

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"codeberg.org/go-latex/latex/mtex/symbols"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

const maxFormulaDepth = 256

type formulaToken uint8

const (
	formulaLiteral formulaToken = iota
	formulaSpace
	formulaGroupOpen
	formulaGroupClose
	formulaControl
	formulaScript
	formulaMathDelimiter
	formulaUnsupported
	formulaComment
	formulaBracket
	formulaControlCharacter
)

// Syntax boundaries precede Unicode segmentation: a Prepend character must not
// absorb a brace or command into its display cluster and hide that syntax.
func classifyFormulaRune(value rune) formulaToken {
	if isFormulaSpace(value) {
		return formulaSpace
	}
	if unicode.IsControl(value) {
		return formulaControlCharacter
	}
	switch value {
	case '{':
		return formulaGroupOpen
	case '}':
		return formulaGroupClose
	case '\\':
		return formulaControl
	case '^', '_':
		return formulaScript
	case '$':
		return formulaMathDelimiter
	case '#', '&':
		return formulaUnsupported
	case '%':
		return formulaComment
	case '[', ']':
		return formulaBracket
	default:
		return formulaLiteral
	}
}

// Parsing produces the terminal box directly, so command arity, text whitespace
// and script attachment each have one owner. No Go literal scanner or global
// diagnostic writer participates in the expression grammar.
type formulaParser struct {
	source string
	at     int
	depth  int
	look   Look
}

type formulaStyle struct {
	cell       grid.Style
	textSpaces bool
}

func render(source string, look Look) (box, error) {
	if !utf8.ValidString(source) {
		return box{}, &parseError{message: "source is not valid UTF-8"}
	}
	parser := formulaParser{source: source, look: look}
	result, err := parser.sequence(0, formulaStyle{cell: look.Text})
	if err != nil {
		return box{}, &parseError{message: err.Error()}
	}
	return result, nil
}

func (p *formulaParser) sequence(closing byte, style formulaStyle) (box, error) {
	if p.depth >= maxFormulaDepth {
		return box{}, fmt.Errorf("formula nesting exceeds %d groups", maxFormulaDepth)
	}
	p.depth++
	defer func() { p.depth-- }()
	var parts []box
	for {
		p.skipTrivia(style.textSpaces)
		if p.at == len(p.source) {
			if closing != 0 {
				return box{}, errors.New("source has an unclosed group")
			}
			return horizontal(parts...), nil
		}
		if closing != 0 && p.source[p.at] == closing {
			p.at++
			return horizontal(parts...), nil
		}
		base, err := p.atom(style, false)
		if err != nil {
			return box{}, err
		}
		base, err = p.scripts(base, style)
		if err != nil {
			return box{}, err
		}
		parts = append(parts, base)
	}
}

func (p *formulaParser) atom(style formulaStyle, single bool) (box, error) {
	if p.at == len(p.source) {
		return box{}, errors.New("missing formula atom")
	}
	value, size := utf8.DecodeRuneInString(p.source[p.at:])
	p.at += size
	switch classifyFormulaRune(value) {
	case formulaGroupOpen:
		return p.sequence('}', style)
	case formulaGroupClose:
		return box{}, errors.New("source has an unmatched closing brace")
	case formulaControl:
		return p.control(style)
	case formulaScript:
		return box{}, errors.New("script has no base or atom")
	case formulaMathDelimiter:
		return box{}, errors.New("source must not contain math delimiters")
	case formulaUnsupported, formulaComment:
		return box{}, fmt.Errorf("unsupported formula character %q", value)
	case formulaSpace:
		return atom(" ", style.cell), nil
	case formulaControlCharacter:
		return box{}, fmt.Errorf("source contains control character %U", value)
	default:
		return p.literal(size, style, single), nil
	}
}

func (p *formulaParser) literal(size int, style formulaStyle, single bool) box {
	start := p.at - size
	end := len(p.source)
	if single {
		end = start + text.NextCluster(p.source[start:], 0)
	}
	for p.at < end {
		value, width := utf8.DecodeRuneInString(p.source[p.at:end])
		if classifyFormulaRune(value) != formulaLiteral {
			break
		}
		p.at += width
	}
	var shown strings.Builder
	shown.Grow(p.at - start)
	for _, cluster := range text.Clusters(p.source[start:p.at]) {
		spaced := symbols.IsSpaced(cluster)
		if spaced {
			shown.WriteByte(' ')
		}
		shown.WriteString(p.plainSymbol(cluster))
		if spaced {
			shown.WriteByte(' ')
		}
	}
	return atom(shown.String(), style.cell)
}

func isFormulaSpace(value rune) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func (p *formulaParser) skipTrivia(keepSpaces bool) {
	for p.at < len(p.source) {
		value := p.source[p.at]
		kind := classifyFormulaRune(rune(value))
		if kind == formulaComment {
			lineEnd := strings.IndexByte(p.source[p.at:], '\n')
			if lineEnd < 0 {
				p.at = len(p.source)
				return
			}
			p.at += lineEnd
			continue
		}
		if keepSpaces || kind != formulaSpace {
			return
		}
		p.at++
	}
}

func (p *formulaParser) scripts(base box, style formulaStyle) (box, error) {
	var upper, lower box
	hasUpper, hasLower := false, false
	for {
		p.skipTrivia(style.textSpaces)
		if p.at == len(p.source) {
			break
		}
		switch p.source[p.at] {
		case '^':
			if hasUpper {
				return box{}, errors.New("base has two superscripts")
			}
			hasUpper = true
			value, err := p.scriptAtom(style)
			if err != nil {
				return box{}, err
			}
			upper = value
		case '_':
			if hasLower {
				return box{}, errors.New("base has two subscripts")
			}
			hasLower = true
			value, err := p.scriptAtom(style)
			if err != nil {
				return box{}, err
			}
			lower = value
		default:
			return scripted(base, upper, lower), nil
		}
	}
	return scripted(base, upper, lower), nil
}

func (p *formulaParser) scriptAtom(style formulaStyle) (box, error) {
	p.at++
	p.skipTrivia(false)
	return p.atom(style, true)
}

func (p *formulaParser) argument(style formulaStyle) (box, error) {
	p.skipTrivia(false)
	if p.at == len(p.source) || p.source[p.at] != '{' {
		return box{}, errors.New("control sequence needs a braced argument")
	}
	p.at++
	return p.sequence('}', style)
}

func (p *formulaParser) pair(style formulaStyle) (box, box, error) {
	first, err := p.argument(style)
	if err != nil {
		return box{}, box{}, err
	}
	second, err := p.argument(style)
	return first, second, err
}
