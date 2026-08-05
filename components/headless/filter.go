package headless

import (
	"github.com/Tangerg/oolong/core/fuzzy"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// Filter is a list narrowed by a pattern: the items that match it, best first, with
// the characters that matched marked.
//
// It puts together the three pieces that were already here — a list, a fuzzy match,
// and somewhere to scroll — and it deliberately does not include the fourth. Where
// the pattern is typed is the caller's: an editor at the bottom of a dialog, a
// composer already on screen, a command's argument, or nothing at all in a test.
// Owning a text field as well would make this two widgets in a trench coat, and
// would decide where the field goes for everyone.
//
//	filter.SetPattern(composer.Text())
//
// The zero Filter shows everything, in the order it was given.
type Filter[T any] struct {
	// Items are everything there is to choose from, matched or not.
	Items []T
	// Text is what an item reads as, which is what the pattern is matched against. A
	// filter with none matches nothing, because there is nothing to match: an item is
	// whatever the caller says it is, and this cannot guess how to read one.
	Text func(item T) string
	// Row draws one of the items that matched. at is where it sits among the rows on
	// screen, match says which characters of its text answered the pattern, and
	// selected says whether it is the one under the cursor.
	Row func(v grid.View, at int, item T, match fuzzy.Match, selected bool)
	// Keys say which keystrokes move the cursor. Nil reads through [DefaultListKeys].
	Keys *input.Keymap

	pattern string
	// list is what matched. It owns the cursor and the scroll.
	list List[hit[T]]
	// fresh says the matches are the ones this pattern and these items produce.
	fresh bool
}

// hit is one item that matched, and how.
type hit[T any] struct {
	item  T
	match fuzzy.Match
}

// Pattern is what the items are being narrowed by.
func (f *Filter[T]) Pattern() string { return f.pattern }

// SetPattern narrows the list, keeping the cursor at the top of what matched.
//
// The cursor goes to the top rather than staying where it was, because after a
// keystroke the rows under it are different rows: staying on the third one would
// leave the cursor on whatever happened to land there, which is how a reader picks
// something they did not mean.
func (f *Filter[T]) SetPattern(pattern string) {
	if pattern == f.pattern && f.fresh {
		return
	}
	f.pattern, f.fresh = pattern, false
	f.match()
	f.list.Select(0)
}

// SetItems replaces what there is to choose from and matches the pattern again.
func (f *Filter[T]) SetItems(items []T) {
	f.Items, f.fresh = items, false
	f.match()
}

// Matched is how many items answered the pattern.
func (f *Filter[T]) Matched() int {
	f.match()
	return len(f.list.Items)
}

// Selected is the row the cursor is on among the matches, or -1 when nothing
// matched.
func (f *Filter[T]) Selected() int {
	f.match()
	return f.list.Selected()
}

// Current is the item under the cursor and whether there is one.
func (f *Filter[T]) Current() (T, bool) {
	f.match()
	got, ok := f.list.Current()
	return got.item, ok
}

// Select moves the cursor to one of the matches, clamped to them.
func (f *Filter[T]) Select(at int) {
	f.match()
	f.list.Select(at)
}

// Scroll exposes the position, for a scrollbar drawn beside the list.
func (f *Filter[T]) Scroll() *Scroll { return f.list.Scroll() }

// Handle answers the keys, the wheel and a press that move the cursor. What narrows
// the list is not an event here: it is [Filter.SetPattern], because the pattern is
// typed somewhere this does not own.
func (f *Filter[T]) Handle(ev input.Event) bool {
	f.match()
	return f.list.Handle(ev)
}

// Do runs one of the list's actions by name. See [Doer].
func (f *Filter[T]) Do(action input.Action) bool {
	f.match()
	return f.list.Do(action)
}

// Measure is one row per match.
func (f *Filter[T]) Measure(int) int { return f.Matched() }

// Draw paints the matches that fit.
func (f *Filter[T]) Draw(v grid.View) {
	f.match()
	f.list.Keys = f.Keys
	f.list.Row = func(row grid.View, at int, got hit[T], selected bool) {
		if f.Row != nil {
			f.Row(row, at, got.item, got.match, selected)
		}
	}
	f.list.Draw(v)
}

// match works out what answers the pattern, once per change.
//
// Every item is read and scored, which is what a fuzzy match is; the memo is so that
// measuring and drawing in the same frame do it once. An empty pattern matches
// everything with nothing marked, and keeps the order the caller gave — which is the
// order they meant when they had nothing to rank by.
func (f *Filter[T]) match() {
	if f.fresh {
		return
	}
	f.fresh = true
	f.list.Keys = f.Keys

	if f.Text == nil {
		f.list.SetItems(nil)
		return
	}
	if f.pattern == "" {
		hits := make([]hit[T], 0, len(f.Items))
		for _, item := range f.Items {
			hits = append(hits, hit[T]{item: item})
		}
		f.list.SetItems(hits)
		return
	}

	candidates := make([]string, 0, len(f.Items))
	for _, item := range f.Items {
		candidates = append(candidates, f.Text(item))
	}
	ranked := fuzzy.Filter(f.pattern, candidates)
	hits := make([]hit[T], 0, len(ranked))
	for _, r := range ranked {
		hits = append(hits, hit[T]{item: f.Items[r.Index], match: r.Match})
	}
	f.list.SetItems(hits)
}
