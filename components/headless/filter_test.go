package headless_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/fuzzy"
	"github.com/Tangerg/oolong/core/grid"
)

func filtered() *headless.Filter[string] {
	f := new(headless.Filter[string])
	f.SetText(func(s string) string { return s })
	f.SetItems([]string{"core/term/pty.go", "core/grid/inline.go", "README.md"})
	return f
}

func matches(f *headless.Filter[string]) string {
	out := make([]string, 0, f.Matched())
	for at := range f.Matched() {
		f.Select(at)
		got, _ := f.Current()
		out = append(out, got)
	}
	return strings.Join(out, " ")
}

func TestAPatternNarrowsTheListAndRanksWhatIsLeft(t *testing.T) {
	f := filtered()
	if f.Matched() != 3 {
		t.Fatalf("with no pattern %d of three items showed", f.Matched())
	}

	f.SetPattern("inline")
	if got := matches(f); got != "core/grid/inline.go" {
		t.Fatalf("the matches are %q", got)
	}
	if got := f.Pattern(); got != "inline" {
		t.Fatalf("the filter says it is narrowed by %q", got)
	}

	f.SetPattern("go")
	if f.Matched() != 2 {
		t.Fatalf("%d items matched, want the two that are Go files", f.Matched())
	}

	f.SetPattern("zzz")
	if f.Matched() != 0 {
		t.Fatalf("%d items matched a pattern nothing answers", f.Matched())
	}
	if _, ok := f.Current(); ok {
		t.Fatal("something was under the cursor with nothing matching")
	}
}

func TestTheCursorGoesToTheTopWhenThePatternChanges(t *testing.T) {
	// After a keystroke the rows under the cursor are different rows. Staying on the
	// third one would leave it on whatever landed there, which is how a reader picks
	// something they did not mean.
	f := filtered()
	paintWidget(30, 3, f)
	f.Select(2)
	f.SetPattern("o")
	if got := f.Selected(); got != 0 {
		t.Fatalf("after narrowing, the cursor is on row %d", got)
	}
}

func TestARowIsToldWhichCharactersAnsweredThePattern(t *testing.T) {
	// Without it a filtered list cannot show why something matched, which is most of
	// what makes one usable.
	var seen fuzzy.Match
	f := filtered()
	f.Row = func(_ grid.View, at int, _ string, match fuzzy.Match, _ bool) {
		if at == 0 {
			seen = match
		}
	}
	f.SetPattern("pty")
	paintWidget(30, 3, f)
	if len(seen.At) != 3 {
		t.Fatalf("the row was told about %v, want the three characters that matched", seen.At)
	}
}

func TestAFilterThatCannotReadAnItemMatchesNothing(t *testing.T) {
	// An item is whatever the caller says it is, so a filter with no way to read one
	// says so rather than guessing.
	f := &headless.Filter[string]{}
	f.SetItems([]string{"a", "b"})
	if f.Matched() != 0 {
		t.Fatalf("%d items matched with no way to read them", f.Matched())
	}
}

func TestFilterOwnsItsSourceAndReturnsSnapshots(t *testing.T) {
	items := []string{"alpha", "beta"}
	f := new(headless.Filter[string])
	f.SetText(func(s string) string { return s })
	f.SetItems(items)
	items[0] = "changed input"
	f.SetPattern("alpha")
	if got, ok := f.Current(); !ok || got != "alpha" {
		t.Fatalf("current = %q, %v after caller mutation", got, ok)
	}

	snapshot := f.Items()
	snapshot[0] = "changed snapshot"
	if got := f.Items()[0]; got != "alpha" {
		t.Fatalf("first source item = %q after snapshot mutation", got)
	}
}

func TestChangingHowItemsReadInvalidatesMatches(t *testing.T) {
	f := new(headless.Filter[string])
	f.SetText(func(s string) string { return s })
	f.SetItems([]string{"alpha", "beta"})
	f.SetPattern("alpha")
	if f.Matched() != 1 {
		t.Fatalf("initial matches = %d", f.Matched())
	}

	f.SetText(func(string) string { return "alpha" })
	if f.Matched() != 2 {
		t.Fatalf("matches after changing the projection = %d", f.Matched())
	}
}
