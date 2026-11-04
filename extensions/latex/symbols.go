package latex

import (
	"fmt"
	"strings"
	"sync"

	"codeberg.org/go-latex/latex/drawtex"
	"codeberg.org/go-latex/latex/font"
	"codeberg.org/go-latex/latex/font/ttf"
	"codeberg.org/go-latex/latex/mtex/symbols"

	"github.com/Tangerg/oolong/ui/core/grid"
)

const (
	defaultSymbolFont = "default"
	romanSymbolFont   = "rm"
	symbolFontSize    = 12
	symbolDPI         = 72
)

func (p *formulaParser) namedMacro(name string, style grid.Style) (box, error) {
	// Repertoire chooses spelling only after support has been established; otherwise
	// the ASCII path would silently accept unknown commands as their bare names.
	symbol, err := symbolsForTerminal.resolve(name)
	switch {
	case err != nil:
		if !symbols.FunctionNames.Has(strings.TrimPrefix(name, `\`)) {
			return box{}, fmt.Errorf("unsupported macro %s", name)
		}
		symbol = strings.TrimPrefix(name, `\`)
	case grid.ClusterWidth(symbol) == 0:
		return box{}, fmt.Errorf("unsupported combining accent %s", name)
	case p.look.Glyphs.Plain:
		symbol = asciiSymbol(name)
	}
	if symbols.IsSpaced(name) {
		symbol = " " + symbol + " "
	}
	return atom(symbol, style), nil
}

func (p *formulaParser) plainSymbol(value string) string {
	if p.look.Glyphs.Plain {
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
type symbolResolver struct {
	mu    sync.RWMutex
	cache map[string]string
}

var symbolsForTerminal symbolResolver

func (r *symbolResolver) resolve(name string) (string, error) {
	r.mu.RLock()
	cached, ok := r.cache[name]
	r.mu.RUnlock()
	if ok {
		return cached, nil
	}
	result := resolveSymbol(name)
	if result.err != nil {
		return result.value, result.err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if cached, ok := r.cache[name]; ok {
		return cached, nil
	}
	if r.cache == nil {
		r.cache = make(map[string]string)
	}
	// A cached macro must not keep the formula's source allocation alive.
	r.cache[strings.Clone(name)] = result.value
	return result.value, nil
}

func resolveSymbol(name string) (result symbolResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result.err = fmt.Errorf("symbol lookup failed: %v", recovered)
		}
	}()
	canvas := drawtex.New()
	backend := ttf.New(canvas)
	backend.RenderGlyph(0, 0, font.Font{Name: defaultSymbolFont, Type: romanSymbolFont, Size: symbolFontSize}, name, symbolDPI)
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
