package latex

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

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

type formulaMode uint8

const (
	mathMode formulaMode = iota
	textMode
	operatorMode
)

type formulaContext struct {
	cell grid.Style
	mode formulaMode
}

func render(source string, look Look) (box, error) {
	if !utf8.ValidString(source) {
		return box{}, &formulaError{message: "source is not valid UTF-8"}
	}
	parser := formulaParser{source: source, look: look}
	result, err := parser.sequence(0, formulaContext{cell: look.Text})
	if err != nil {
		return box{}, &formulaError{message: err.Error()}
	}
	return result, nil
}

func (p *formulaParser) sequence(closing byte, context formulaContext) (box, error) {
	if p.depth >= maxFormulaDepth {
		return box{}, fmt.Errorf("formula nesting exceeds %d groups", maxFormulaDepth)
	}
	p.depth++
	defer func() { p.depth-- }()
	var parts []box
	for {
		p.skipTrivia(context.mode)
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
		base, err := p.atom(context, false)
		if err != nil {
			return box{}, err
		}
		if context.mode != textMode {
			base, err = p.scripts(base, context)
			if err != nil {
				return box{}, err
			}
		}
		parts = append(parts, base)
	}
}

func (p *formulaParser) atom(context formulaContext, single bool) (box, error) {
	if p.at == len(p.source) {
		return box{}, errors.New("missing formula atom")
	}
	value, size := utf8.DecodeRuneInString(p.source[p.at:])
	p.at += size
	switch classifyFormulaRune(value) {
	case formulaGroupOpen:
		return p.sequence('}', context)
	case formulaGroupClose:
		return box{}, errors.New("source has an unmatched closing brace")
	case formulaControl:
		return p.control(context)
	case formulaScript:
		if context.mode == textMode {
			return box{}, errors.New("scripts require a mathematical argument")
		}
		return box{}, errors.New("script has no base or atom")
	case formulaMathDelimiter:
		return box{}, errors.New("source must not contain math delimiters")
	case formulaUnsupported, formulaComment:
		return box{}, fmt.Errorf("unsupported formula character %q", value)
	case formulaSpace:
		return atom(" ", context.cell), nil
	case formulaControlCharacter:
		return box{}, fmt.Errorf("source contains control character %U", value)
	default:
		return p.literal(size, context, single), nil
	}
}

func (p *formulaParser) literal(size int, context formulaContext, single bool) box {
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
		shown.WriteString(context.mode.symbol(cluster, cluster, p.look.Glyphs.Plain))
	}
	return atom(shown.String(), context.cell)
}

func isFormulaSpace(value rune) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func (p *formulaParser) skipTrivia(mode formulaMode) {
	for p.at < len(p.source) {
		value := p.source[p.at]
		kind := classifyFormulaRune(rune(value))
		if kind == formulaComment {
			lineEnd := strings.IndexByte(p.source[p.at:], '\n')
			if lineEnd < 0 {
				p.at = len(p.source)
				return
			}
			p.at += lineEnd + 1
			continue
		}
		if mode == textMode || kind != formulaSpace {
			return
		}
		p.at++
	}
}

func (p *formulaParser) scripts(base box, context formulaContext) (box, error) {
	var upper, lower box
	hasUpper, hasLower := false, false
	for {
		p.skipTrivia(context.mode)
		if p.at == len(p.source) {
			break
		}
		switch p.source[p.at] {
		case '^':
			if hasUpper {
				return box{}, errors.New("base has two superscripts")
			}
			hasUpper = true
			value, err := p.scriptAtom(context)
			if err != nil {
				return box{}, err
			}
			upper = value
		case '_':
			if hasLower {
				return box{}, errors.New("base has two subscripts")
			}
			hasLower = true
			value, err := p.scriptAtom(context)
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

func (p *formulaParser) scriptAtom(context formulaContext) (box, error) {
	p.at++
	p.skipTrivia(mathMode)
	return p.atom(context, true)
}

func (p *formulaParser) argument(context formulaContext) (box, error) {
	p.skipTrivia(mathMode)
	if p.at == len(p.source) || p.source[p.at] != '{' {
		return box{}, errors.New("control sequence needs a braced argument")
	}
	p.at++
	return p.sequence('}', context)
}

func (p *formulaParser) pair(context formulaContext) (box, box, error) {
	first, err := p.argument(context)
	if err != nil {
		return box{}, box{}, err
	}
	second, err := p.argument(context)
	return first, second, err
}
