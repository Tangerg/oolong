package highlight_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/extensions/highlight"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestCodeComesBackAsStyledLines(t *testing.T) {
	lines := highlight.New("github-dark").Lines("go", "func main() {\n\tprintln(\"hi\")\n}")
	if len(lines) != 3 {
		t.Fatalf("%d lines, want one per line of source", len(lines))
	}
	if got := lines[0].String(); got != "func main() {" {
		t.Fatalf("the first line reads %q", got)
	}
	// A line break is the boundary between lines and not a character in one:
	// everything above this lays a line out in columns.
	for i, line := range lines {
		if strings.Contains(line.String(), "\n") {
			t.Fatalf("line %d carries a line break", i)
		}
	}

	var styles []grid.Style
	for _, span := range lines[0] {
		styles = append(styles, span.Style)
	}
	if len(styles) < 2 {
		t.Fatalf("the line came to %d spans, want the parts coloured apart", len(styles))
	}
	same := true
	for _, style := range styles[1:] {
		same = same && style == styles[0]
	}
	if same {
		t.Fatal("every span of the line is drawn the same")
	}
}

func TestCodeNobodyCouldNameIsStillCode(t *testing.T) {
	// An unknown language is guessed at, and a guess that fails is plain text. The
	// whole cost of going wrong here is code that is not coloured.
	for _, language := range []string{"", "not-a-language", "COBOL"} {
		lines := highlight.New(highlight.Scheme(language)).Lines(language, "one\ntwo")
		if len(lines) != 2 || lines[0].String() != "one" || lines[1].String() != "two" {
			t.Fatalf("%q came out as %v", language, lines)
		}
	}
}

func TestZeroRendererUsesTheFallbackScheme(t *testing.T) {
	var renderer highlight.Renderer
	lines := renderer.Lines("go", "package main")
	if len(lines) != 1 || lines[0].String() != "package main" {
		t.Fatalf("zero renderer produced %v", lines)
	}
	if _, ok := renderer.Background(); !ok {
		t.Fatal("the fallback scheme did not expose its background")
	}
}

func TestASchemeSaysWhatItExpectsToSitOn(t *testing.T) {
	// A whole picture: light text from a dark scheme on a light terminal is
	// unreadable, and the pane belongs to the caller.
	dark, ok := highlight.New("github-dark").Background()
	if !ok {
		t.Fatal("a scheme with a background did not say so")
	}
	if dark.Default() || !dark.RGB().Dark() {
		t.Fatalf("the dark scheme expects to sit on %v", dark.RGB())
	}
	if light, ok := highlight.New("github").Background(); !ok || light.RGB().Dark() {
		t.Fatalf("the light scheme expects to sit on %v (said %v)", light.RGB(), ok)
	}
}

func TestTheSchemesOnOfferAreTheOnesThatWork(t *testing.T) {
	names := highlight.Schemes()
	if len(names) < 10 {
		t.Fatalf("%d schemes on offer", len(names))
	}
	// What a program offers a user is what will be accepted, or the offer is a lie.
	for _, name := range names[:5] {
		if lines := highlight.New(name).Lines("go", "package main"); len(lines) != 1 {
			t.Fatalf("%q highlighted to %v", name, lines)
		}
	}
}

func TestOneFunctionIsTheWholeOfTheWiring(t *testing.T) {
	// The function shape Markdown's extension registry accepts, written out because
	// this peer module must not import Markdown merely to satisfy its seam.
	call := func(renderer func(info, source string) []text.Line) []text.Line {
		return renderer("go", "var x = 1")
	}
	renderer := highlight.New("monokai")
	if lines := call(renderer.Lines); len(lines) != 1 || lines[0].String() != "var x = 1" {
		t.Fatalf("the plugged-in highlighter produced %v", lines)
	}
}

func TestHighlightPreservesEverySourceRow(t *testing.T) {
	for _, language := range []string{"go", "diff", "Bash Session"} {
		t.Run(language, func(t *testing.T) {
			for _, source := range []string{"", "x", "x\n", "x\n\n", "\n\n", "x\r\n\r", "x\ry"} {
				normalized := strings.ReplaceAll(strings.ReplaceAll(source, "\r\n", "\n"), "\r", "\n")
				lines := highlight.New("").Lines(language, source)
				want := strings.Split(normalized, "\n")
				if len(lines) != len(want) {
					t.Fatalf("Lines(%q) returned %d rows, want %d", source, len(lines), len(want))
				}
				for i, row := range want {
					if lines[i].String() != row {
						t.Fatalf("Lines(%q)[%d] = %q, want %q", source, i, lines[i].String(), row)
					}
				}
			}
		})
	}
}

func TestStalledLexerReturnsTheCompleteSource(t *testing.T) {
	const source = "value ^"
	lines := highlight.New("").Lines("jsonata", source)
	if len(lines) != 1 || lines[0].String() != source {
		t.Fatalf("incomplete highlighting of %q: %v", source, lines)
	}
}
