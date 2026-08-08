package headless

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/core/fuzzy"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
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
	// items are private because replacing them must invalidate the ranked matches.
	// A public slice let the source change while the filter kept results from its old
	// contents.
	items []T
	// text is what an item reads as, which is what the pattern is matched against. A
	// filter with none matches nothing, because there is nothing to match: an item is
	// whatever the caller says it is, and this cannot guess how to read one.
	text func(item T) string
	// Row draws one of the items that matched. at is where it sits among the rows on
	// screen, match says which characters of its text answered the pattern, and
	// selected says whether it is the one under the cursor.
	Row func(v grid.View, at int, item T, match fuzzy.Match, selected bool)
	// Keys say which keystrokes move the cursor. Nil reads through [DefaultListKeys].
	Keys *keymap.Map

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
	f.pattern, f.fresh = strings.Clone(pattern), false
	f.match()
	f.list.Select(0)
}

// SetItems replaces what there is to choose from and matches the pattern again. The
// filter copies the slice; the caller may reuse or change its input afterwards.
func (f *Filter[T]) SetItems(items []T) {
	f.items, f.fresh = own(f.items, items), false
	f.match()
}

// SetText says how an item reads for matching and recalculates the ranked rows. Nil
// makes no item match. The function is behavior rather than a field because changing
// it must invalidate every cached match.
func (f *Filter[T]) SetText(read func(item T) string) {
	f.text, f.fresh = read, false
	f.match()
	f.list.Select(0)
}

// Items returns a copy of everything there is to choose from, matched or not.
func (f *Filter[T]) Items() []T {
	if f == nil {
		return nil
	}
	return slices.Clone(f.items)
}

// Len reports how many unfiltered items the filter owns.
func (f *Filter[T]) Len() int {
	if f == nil {
		return 0
	}
	return len(f.items)
}

// Matched is how many items answered the pattern.
func (f *Filter[T]) Matched() int {
	f.match()
	return f.list.Len()
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
func (f *Filter[T]) Do(action keymap.Action) bool {
	f.match()
	return f.list.Do(action)
}

// Focus takes the keyboard, or gives it up — see [List.Focus].
func (f *Filter[T]) Focus(has bool) { f.list.Focus(has) }

// Focused reports whether this list has the keyboard.
func (f *Filter[T]) Focused() bool { return f.list.Focused() }

// Measure is one row per match.
func (f *Filter[T]) Measure(int) int { return f.Matched() }

// Draw paints the matches that fit.
func (f *Filter[T]) Draw(v Frame) {
	f.match()
	f.list.DrawRows(v, f.row)
}

// row hands one match to whoever knows what a row looks like, with the characters
// that answered the pattern.
func (f *Filter[T]) row(v grid.View, at int, got hit[T], selected bool) {
	if f.Row != nil {
		f.Row(v, at, got.item, got.match, selected)
	}
}

// match works out what answers the pattern, once per change.
//
// Every item is read and scored, which is what a fuzzy match is; the memo is so that
// measuring and drawing in the same frame do it once. An empty pattern matches
// everything with nothing marked, and keeps the order the caller gave — which is the
// order they meant when they had nothing to rank by.
func (f *Filter[T]) match() {
	// Keys do not change which items match, but they are still live configuration of
	// the inner list and must not wait for an unrelated match invalidation.
	f.list.Keys = f.Keys
	if f.fresh {
		return
	}

	if f.text == nil {
		f.list.SetItems(nil)
		f.fresh = true
		return
	}
	if f.pattern == "" {
		hits := make([]hit[T], 0, len(f.items))
		for _, item := range f.items {
			hits = append(hits, hit[T]{item: item})
		}
		f.list.SetItems(hits)
		f.fresh = true
		return
	}

	candidates := make([]string, 0, len(f.items))
	for _, item := range f.items {
		candidates = append(candidates, f.text(item))
	}
	ranked := fuzzy.Filter(f.pattern, candidates)
	hits := make([]hit[T], 0, len(ranked))
	for _, r := range ranked {
		hits = append(hits, hit[T]{item: f.items[r.Index], match: r.Match})
	}
	f.list.SetItems(hits)
	f.fresh = true
}
