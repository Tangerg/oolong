// Package highlight colours source code.
//
// It is a module of its own for the same reason markdown is: it carries somebody
// else's tree. A syntax highlighter is a lexer per language and a palette per
// theme — several megabytes of them — and the two modules a terminal interface is
// built on promise a dependency list that can be adopted without thinking about it.
// So this is where that weight is allowed to be, and a program that does not show
// code never hears about it.
//
// # What it produces
//
// [github.com/Tangerg/oolong/core/text.Line]s, which is what everything that lays
// text out already takes. Nothing here draws, measures or wraps: a highlighted
// block of code is styled text and is laid out like any other styled text.
//
// # Integration
//
// A [Renderer] has the same [Renderer.Lines] method whether it is called directly
// or installed in a semantic content registry. The first argument is a language
// and the second is source; this peer module need not import whichever composer
// consumes the result:
//
//	renderer := highlight.New("github-dark")
//	lines := renderer.Lines("go", source)
//
// # What it does not expose
//
// The highlighter. No type here comes from it and none goes back to it: a style is
// its name, a language is its name, and what comes out is text. That is the same
// boundary markdown keeps around its parser, and it is what lets either be replaced
// without anything above noticing.
package highlight

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// Style is a colour scheme by name — "github-dark", "monokai", "solarized-light".
// [Styles] is the list of them, and a name that is not one of them gets the
// highlighter's own default rather than nothing.
type Style string

// Styles returns the available schemes in alphabetical order. A program offering
// the choice to a user reads from this list, so its values have the same [Style]
// type [New] accepts and require no conversion at the call site.
func Styles() []Style {
	names := styles.Names()
	available := make([]Style, len(names))
	for i, name := range names {
		available[i] = Style(name)
	}
	return available
}

// Renderer highlights source with one resolved colour scheme.
//
// Its zero value uses the highlighter's fallback scheme. A Renderer is immutable
// and cheap to copy; resolving the named style once keeps [Renderer.Lines] suitable
// for every fenced block of a streaming answer.
type Renderer struct{ scheme *chroma.Style }

// New constructs a renderer for style. An unknown name uses the highlighter's
// fallback scheme, so source always remains readable.
func New(style Style) Renderer { return Renderer{scheme: schemeOf(style)} }

// Lines is one block of code, highlighted.
//
// The language is what the author wrote after the fence, unexamined — "go", "Go",
// "golang" and "" all end up somewhere sensible, because an unknown one is guessed
// at from the source and a guess that fails is plain text rather than an error. Code
// nobody could name is still code, and showing it uncoloured is the whole of what
// going wrong here should cost.
func (r Renderer) Lines(language, source string) []text.Line {
	return highlight(language, source, r.resolved())
}

// Background is the colour a style expects its code to sit on, and whether it has
// one.
//
// It is worth asking because a scheme is a whole picture: light text from a dark
// scheme on a light terminal is unreadable, and the caller is the one holding the
// pane it goes in. A caller that would rather keep its own background ignores this,
// which is why it is a question rather than something written into the lines.
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

// schemeOf resolves a name to a scheme, falling back rather than failing.
func schemeOf(style Style) *chroma.Style {
	if scheme := styles.Get(string(style)); scheme != nil {
		return scheme
	}
	return styles.Fallback
}

// highlight lexes the source and turns each token into a styled span.
func highlight(language, source string, scheme *chroma.Style) []text.Line {
	lexer := lexerFor(language, source)
	iterator, err := lexer.Tokenise(nil, source)
	if err != nil {
		// Whatever went wrong, the code is still the code.
		return plain(source)
	}

	rows := chroma.SplitTokensIntoLines(iterator.Tokens())
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
	return out
}

// lexerFor picks the lexer for a language, guessing from the source when the name
// says nothing and coalescing what it finds — which merges the runs of one token
// type that a lexer emits character by character, and is the difference between a
// line of forty spans and a line of six.
func lexerFor(language, source string) chroma.Lexer {
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Analyse(source)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return chroma.Coalesce(lexer)
}

// styleOf turns one scheme entry into a cell style.
//
// The colour and the three attributes a scheme states, and not the background: a
// token's own background belongs to the scheme's idea of a page, and the page here
// is whatever pane the code was put in. [Background] is how a caller asks for that
// separately, and how it stays the caller's decision.
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

// colour is a scheme's colour as a cell's.
func colour(c chroma.Colour) grid.Color {
	return grid.RGBColor(c.Red(), c.Green(), c.Blue())
}

// plain is the source with no styling at all, which is what code nobody could lex
// comes to.
func plain(source string) []text.Line {
	rows := strings.Split(source, "\n")
	out := make([]text.Line, 0, len(rows))
	for _, row := range rows {
		out = append(out, text.Of(strings.Clone(row), grid.Style{}))
	}
	return out
}
