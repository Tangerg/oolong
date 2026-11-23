package base

import (
	"image"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/oolong/ui/core/fuzzy"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/setting"
)

// Filter is a list narrowed by a pattern: the items that match it, best first, with
// the characters that matched marked.
//
// It puts together the three pieces that were already here — a list, a fuzzy match,
// and somewhere to scroll — and it deliberately does not include the fourth. Where
// the pattern is typed is the caller's: an editor at the bottom of a dialog, one
// already on screen, a command's argument, or nothing at all in a test.
// Owning a text field as well would make this two widgets in a trench coat, and
// would decide where the field goes for everyone.
//
//	filter.SetPattern(editor.Controller().Text())
//
// A Filter without item text matches nothing, because it cannot guess how to read
// an item. A Filter must not be
// copied after first use: its source, ranked list and cursor are one mutable owner.
// Source and pattern changes prepare their matches before publication. A projection
// panic leaves the previous state intact; projections cannot reenter either setter.
// Exhausting the collection revision panics before replacing the source or pattern.
type Filter[T any] struct {
	element scene.Element
	noCopy  noCopy

	// items are private because replacing them must invalidate the ranked matches.
	// A public slice let the source change while the filter kept results from its old
	// contents.
	items []T
	// text is what an item reads as, which is what the pattern is matched against. A
	// filter with none matches nothing, because there is nothing to match: an item is
	// whatever the caller says it is, and this cannot guess how to read one.
	text       func(item T) string
	rowPainter func(v grid.View, at int, item T, match fuzzy.Match, selected bool)

	pattern string

	list rows[hit[T]]
}

type hit[T any] struct {
	item  T
	match fuzzy.Match
}

type FilterConfig[T any] struct {
	// Row draws one of the items that matched: at is where it sits among the rows
	// on screen, match says which characters of its text answered the pattern, and
	// selected says whether it is the one under the cursor. It runs during drawing
	// and must be an observationally pure projection.
	Row  func(v grid.View, at int, item T, match fuzzy.Match, selected bool)
	Keys *keymap.Map
	// Activate chooses a match, as [ListConfig.Activate] does; at is its place
	// among the matches.
	Activate func(at int, item T)
	// Same reports whether two items are one item, so replacing the source keeps
	// the cursor on that item among the new matches. Nil, or an item that no longer
	// matches, puts the cursor at the top. A new pattern always does.
	Same func(a, b T) bool
}

func NewFilter[T any](config FilterConfig[T]) *Filter[T] {
	f := &Filter[T]{rowPainter: config.Row}
	list := ListConfig[hit[T]]{Keys: config.Keys}
	if config.Activate != nil {
		list.Activate = func(at int, got hit[T]) { config.Activate(at, got.item) }
	}
	if config.Same != nil {
		list.Same = func(a, b hit[T]) bool { return config.Same(a.item, b.item) }
	}
	f.list.configure(list)
	return f
}

func (f *Filter[T]) Pattern() string { return f.pattern }

// SetPattern narrows the list, keeping the cursor at the top of what matched.
//
// The cursor goes to the top rather than staying where it was, because after a
// keystroke the rows under it are different rows: staying on the third one would
// leave the cursor on whatever happened to land there, which is how a reader picks
// something they did not mean.
func (f *Filter[T]) SetPattern(pattern string) {
	requireMutable(f.element)
	if pattern == f.pattern {
		return
	}
	pattern = strings.Clone(pattern)
	hits := f.matches(f.items, f.text, pattern)
	f.list.requireReplacement()
	f.list.matcher.Reset(func() {
		f.pattern = pattern
		f.replaceMatches(hits, 0)
	})
}

// SetItems replaces the source: the values there are to choose from and the one
// projection that says how each value reads for matching. They travel together
// because ranked matches are a fact about both; separate setters would expose a
// transient source whose values and projection disagree. Nil text makes no item
// match. The filter copies items, so the caller may reuse or change its input.
// text must be observationally pure and stable until the next SetItems.
// It cannot call SetItems or SetPattern on this filter; doing so panics. If text
// panics, the previous source, projection, matches and cursor remain intact. The
// cursor stays on the item [FilterConfig.Same] identifies with the one under it,
// and otherwise goes to the top of the new matches.
func (f *Filter[T]) SetItems(items []T, text func(item T) string) {
	requireMutable(f.element)
	items = slices.Clone(items)
	hits := f.matches(items, text, f.pattern)
	at := f.list.followed(hits, 0)
	f.list.requireReplacement()
	f.list.matcher.Reset(func() {
		f.items = items
		f.text = text
		f.replaceMatches(hits, at)
	})
}

// Items returns a copy of everything there is to choose from, matched or not.
func (f *Filter[T]) Items() []T {
	return slices.Clone(f.items)
}

func (f *Filter[T]) Len() int {
	return len(f.items)
}

func (f *Filter[T]) Matched() int {
	return f.list.Len()
}

// Selected is the row the cursor is on among the matches, or -1 when nothing
// matched.
func (f *Filter[T]) Selected() int {
	return f.list.Selected()
}

// Current is the item under the cursor and whether there is one.
func (f *Filter[T]) Current() (T, bool) {
	got, ok := f.list.Current()
	return got.item, ok
}

// Hit returns the index among ranked matches at point in the filter's local
// coordinates, in the same order as Selected and Row, not the source Items order.
// It uses [List.Hit]'s complete-frame contract. Changing the pattern or source
// invalidates hits until the new matches are drawn. Hit changes no filter state.
func (f *Filter[T]) Hit(point image.Point) (int, bool) {
	return f.list.Hit(point)
}

// Select moves the cursor to one of the matches, clamped to them.
func (f *Filter[T]) Select(at int) {
	f.list.Select(at)
}

func (f *Filter[T]) Scroll() *Scroll { return f.list.Scroll() }

// Handle answers the keys, the wheel and a press that move the cursor. What narrows
// the list is not an event here: it is [Filter.SetPattern], because the pattern is
// typed somewhere this does not own.
func (f *Filter[T]) Handle(ev input.Event) bool {
	requireMutable(f.element)
	return f.list.Handle(ev)
}

func (f *Filter[T]) Do(action keymap.Action) bool {
	return f.list.Do(action)
}

func (f *Filter[T]) focusChanged(has bool) { f.list.focusChanged(has) }

func (f *Filter[T]) Focused() bool { return f.list.Focused() }

func (f *Filter[T]) HeightForWidth(int) int { return f.list.Len() }

func (f *Filter[T]) Draw(v scene.Frame) {
	f.list.DrawRows(v, f.row)
}

func (f *Filter[T]) row(v grid.View, at int, got hit[T], selected bool) {
	if f.rowPainter != nil {
		got.match.At = slices.Clone(got.match.At)
		f.rowPainter(v, at, got.item, got.match, selected)
	}
}

// replaceMatches installs ranked matches with the cursor at at. A cursor kept on
// the same item keeps it in view; one sent to the top shows the best matches.
func (f *Filter[T]) replaceMatches(hits []hit[T], at int) {
	f.list.replaceItems(hits, at)
	if at == 0 {
		f.list.scroll.toTop()
	}
	f.list.notifyChange()
}

// Preparation borrows complete inputs and leaves the last publication readable.
func (f *Filter[T]) matches(items []T, project func(T) string, pattern string) []hit[T] {
	if project == nil {
		return nil
	}
	if pattern == "" {
		hits := make([]hit[T], 0, len(items))
		for _, item := range items {
			hits = append(hits, hit[T]{item: item})
		}
		return hits
	}

	var ranked []fuzzy.Ranked
	f.Element().ReadOnly(func() { ranked = fuzzy.FilterFunc(pattern, items, project) })
	hits := make([]hit[T], 0, len(ranked))
	for _, r := range ranked {
		hits = append(hits, hit[T]{item: items[r.Index], match: r.Match})
	}
	return hits
}

func (f *Filter[T]) Row() func(grid.View, int, T, fuzzy.Match, bool) { return f.rowPainter }

func (f *Filter[T]) SetRow(value func(grid.View, int, T, fuzzy.Match, bool)) {
	setting.Replace(f.element, &f.rowPainter, value)
}

func (f *Filter[T]) Keys() *keymap.Map { return f.list.Keys() }

// KeysFor is what the effective key map binds to action, defaults included.
func (f *Filter[T]) KeysFor(action keymap.Action) []input.Keys { return f.list.KeysFor(action) }

func (f *Filter[T]) SetKeys(keys *keymap.Map) { f.list.SetKeys(keys) }

func (f *Filter[T]) Element() scene.Element {
	if f.element.IsZero() {
		f.element = scene.NewElement(scene.ElementConfig{
			Measure: layout.MeasureRows(f.HeightForWidth), Paint: f.Draw,
			Handle: func(e scene.Event) scene.EventResult {
				return partEvent(e, f.Handle, &f.list.geometry, &f.list.pointer.geometry)
			}, Focusable: func() bool { return true }, FocusChanged: f.focusChanged,
			Semantics: func() scene.Semantic {
				return scene.Semantic{Role: scene.RoleList, ContentRevision: f.list.contentRevision, Name: f.Pattern(), Value: strconv.Itoa(f.Selected())}
			}, Actions: append(collectionActions(&f.list, func(row hit[T]) T { return row.item }), collectionActivateAction(&f.list), f.patternAction()),
		})
	}
	f.list.adopt(f.element)
	return f.element
}
