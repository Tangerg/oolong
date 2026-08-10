package highlight_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/highlight"
)

func TestCodeComesBackAsStyledLines(t *testing.T) {
	lines := highlight.Lines("go", "func main() {\n\tprintln(\"hi\")\n}", "github-dark")
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

	// The keyword and the string are not drawn the same, which is the whole point.
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
		lines := highlight.Lines(language, "one\ntwo", highlight.Style(language))
		if len(lines) != 2 || lines[0].String() != "one" || lines[1].String() != "two" {
			t.Fatalf("%q came out as %v", language, lines)
		}
	}
}

func TestASchemeSaysWhatItExpectsToSitOn(t *testing.T) {
	// A whole picture: light text from a dark scheme on a light terminal is
	// unreadable, and the pane belongs to the caller.
	dark, ok := highlight.Background("github-dark")
	if !ok {
		t.Fatal("a scheme with a background did not say so")
	}
	if dark.Default() || !dark.RGB().Dark() {
		t.Fatalf("the dark scheme expects to sit on %v", dark.RGB())
	}
	if light, ok := highlight.Background("github"); !ok || light.RGB().Dark() {
		t.Fatalf("the light scheme expects to sit on %v (said %v)", light.RGB(), ok)
	}
}

func TestTheSchemesOnOfferAreTheOnesThatWork(t *testing.T) {
	names := highlight.Styles()
	if len(names) < 10 {
		t.Fatalf("%d schemes on offer", len(names))
	}
	// What a program offers a user is what will be accepted, or the offer is a lie.
	for _, name := range names[:5] {
		if lines := highlight.Lines("go", "package main", highlight.Style(name)); len(lines) != 1 {
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
	if lines := call(highlight.Of("monokai")); len(lines) != 1 || lines[0].String() != "var x = 1" {
		t.Fatalf("the plugged-in highlighter produced %v", lines)
	}
}
