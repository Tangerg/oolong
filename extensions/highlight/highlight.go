// Package highlight isolates lexer dependencies so terminal-only consumers need not
// import them. Public values use core text and named schemes to keep Chroma private.
package highlight

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Scheme names a color scheme. An unknown name selects the fallback scheme so
// source remains readable.
type Scheme string

// Schemes sorts scheme names so application selection lists remain deterministic.
func Schemes() []Scheme {
	names := styles.Names()
	available := make([]Scheme, len(names))
	for i, name := range names {
		available[i] = Scheme(name)
	}
	return available
}

// Renderer resolves an immutable scheme once to keep each fenced block independent of
// later configuration. The zero value uses the fallback scheme.
type Renderer struct{ scheme *chroma.Style }

// New uses a fallback scheme for unknown names so highlighting never hides source.
func New(scheme Scheme) Renderer { return Renderer{scheme: styles.Get(string(scheme))} }

// Lines keeps source readable when classification fails by falling back to plain text.
func (r Renderer) Lines(language, source string) []text.Line {
	return highlight(lexerFor(language, source), source, r.resolved())
}

// Background belongs to the pane; applying it to every token would make
// source text impose page appearance on its container.
func (r Renderer) Background() (grid.Color, bool) {
	entry := r.resolved().Get(chroma.Background)
	if !entry.Background.IsSet() {
		return grid.Color{}, false
	}
	return colour(entry.Background), true
}

func (r Renderer) resolved() *chroma.Style {
	if r.scheme != nil {
		return r.scheme
	}
	return styles.Fallback
}

func highlight(lexer chroma.Lexer, source string, scheme *chroma.Style) []text.Line {
	source = text.NormalizeLineEndings(source)
	tokens := tokenize(lexer, source)
	if len(tokens) == 0 {
		return plain(source)
	}

	rows := chroma.SplitTokensIntoLines(tokens)
	out := make([]text.Line, 0, len(rows))
	for _, row := range rows {
		var line text.Line
		for _, token := range row {
			// The newline is the row boundary and not part of the row: everything above
			// this lays a line out in columns, where a line break is not a character.
			value := strings.TrimSuffix(token.Value, "\n")
			if value == "" {
				continue
			}
			line = append(line, text.Span{
				Text: strings.Clone(value), Style: styleOf(scheme.Get(token.Type)),
			})
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return plain(source)
	}
	// Chroma omits the final empty row and some lexers synthesize their own
	// terminator. Only a line ending in the caller's source opens another row.
	if strings.HasSuffix(source, "\n") {
		out = append(out, nil)
	}
	return out
}

// Chroma permits lexer failures to panic while iterating, after Tokenise returned
// successfully. Discard partial tokens so fallback renders the complete source.
func tokenize(lexer chroma.Lexer, source string) (tokens []chroma.Token) {
	defer func() {
		if recover() != nil {
			tokens = nil
		}
	}()
	iterator, err := lexer.Tokenise(nil, source)
	if err != nil {
		return nil
	}
	return sourceTokens(iterator, source)
}

// Lexers can return zero-width capture groups. Bound consecutive empty tokens
// before coalescing: an iterator stuck at one input position would otherwise spin
// inside the coalescer. Source owns the text, the lexer only classifies its bytes.
func sourceTokens(next chroma.Iterator, source string) []chroma.Token {
	const maxEmptyTokens = 100
	var tokens []chroma.Token
	for at, empty := 0, 0; at < len(source); {
		token := next()
		if token == chroma.EOF {
			return nil
		}
		if token.Value == "" {
			empty++
			if empty >= maxEmptyTokens {
				return nil
			}
			continue
		}
		empty = 0
		remaining := source[at:]
		if len(token.Value) > len(remaining) {
			// A line-oriented lexer can supply its own final newline.
			token.Value, _ = strings.CutSuffix(token.Value, "\n")
		}
		if !strings.HasPrefix(remaining, token.Value) {
			return nil
		}
		end := at + len(token.Value)
		last := len(tokens) - 1
		if last >= 0 && tokens[last].Type == token.Type {
			tokens[last].Value = source[at-len(tokens[last].Value) : end]
		} else {
			tokens = append(tokens, chroma.Token{Type: token.Type, Value: source[at:end]})
		}
		at = end
	}
	return tokens
}

func lexerFor(language, source string) chroma.Lexer {
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Analyse(source)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return lexer
}

// Token backgrounds describe the highlighter's page. The application owns the
// pane, so only foreground and text attributes cross this boundary.
func styleOf(entry chroma.StyleEntry) grid.Style {
	var style grid.Style
	if entry.Colour.IsSet() {
		style.FG = colour(entry.Colour)
	}
	if entry.Bold == chroma.Yes {
		style.Attr |= grid.Bold
	}
	if entry.Italic == chroma.Yes {
		style.Attr |= grid.Italic
	}
	if entry.Underline == chroma.Yes {
		style.Attr |= grid.Underline
	}
	return style
}

func colour(c chroma.Colour) grid.Color {
	return grid.RGBColor(c.Red(), c.Green(), c.Blue())
}

func plain(source string) []text.Line {
	rows := strings.Split(source, "\n")
	out := make([]text.Line, 0, len(rows))
	for _, row := range rows {
		out = append(out, text.Of(strings.Clone(row), grid.Style{}))
	}
	return out
}
