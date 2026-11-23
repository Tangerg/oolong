package base

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/internal/setting"
)

type Candidate struct {
	Text string
	// Label is the row as shown. Empty shows Text, which is the common case: what is
	// offered and what it inserts are usually the same thing.
	Label  string
	Detail string
	// Matched are byte offsets in the label that the query matched, picked out as the
	// row is drawn. It is what [fuzzy.Match.At] returns, and leaving it nil simply
	// highlights nothing.
	Matched []int
}

func (c Candidate) shown() string {
	if c.Label != "" {
		return c.Label
	}
	return c.Text
}

// Completion offers candidates for a token someone is typing.
//
// It is the list and the keys, not the source: what the candidates are, where they
// came from and what accepting one means are the caller's. That is the whole of why
// this is reusable — a file list, a command palette and an emoji picker are this
// widget with different candidates, and none of them is something a terminal library
// should know about.
//
// It draws itself into the space it is given, and knows nothing about floating. A
// caller that wants it over the text composes it with an appearance-layer placement
// wrapper; that wrapper, not completion behavior, owns the geometry.
//
// The zero value is closed and ready. A Completion must not be copied after first use:
// its candidates, matcher and acceptance lifecycle are one mutable owner.
type Completion struct {
	element scene.Element
	noCopy  noCopy

	look         Look
	currentToken func() (Token, bool)
	renderer     CompletionRenderer
	maxRows      int
	keys         keyBinding
	accept       func(c Candidate, t Token)

	list    rows[Candidate]
	token   Token
	matcher keymap.Matcher
}

// DefaultCompletionRows is how many candidates are shown at once when nothing says
// otherwise: enough to choose from, few enough to leave the text visible behind it.
const DefaultCompletionRows = 8

type CompletionConfig struct {
	Look Look
	// CurrentToken ties an offer to a live editing position. Cursor movement alone
	// can invalidate it without an edit-session notification. The query must be pure.
	CurrentToken func() (Token, bool)
	// Renderer owns the one-row layout and its intrinsic width. Nil uses
	// DefaultCompletionRenderer. Callbacks must be observationally pure.
	Renderer CompletionRenderer
	// MaxRows caps how tall the list gets, so a thousand files do not become a
	// thousand rows. Zero uses [DefaultCompletionRows].
	MaxRows int
	// Keys say which keystrokes produce which of the actions a completion answers to:
	// its own two, and the list movement inside it. Nil reads through
	// [DefaultCompletionKeys].
	Keys *keymap.Map
	// Accept is called when the user takes a candidate, with the token it replaces.
	// The completion has closed itself by then, so this is free to change the text
	// the token came from. Without it there is nothing accepting could do, and the
	// keystroke is left for whatever else might want it rather than swallowed.
	Accept func(Candidate, Token)
}

// NewCompletion panics for negative MaxRows; zero selects DefaultCompletionRows.
func NewCompletion(config CompletionConfig) *Completion {
	requireNonNegative("completion MaxRows", config.MaxRows)
	return &Completion{
		look: config.Look, currentToken: config.CurrentToken, renderer: config.Renderer,
		maxRows: config.MaxRows, keys: keyBinding{current: config.Keys}, accept: config.Accept,
	}
}

// Offer opens the completion on a token, or closes it when there is nothing to offer.
//
// A popup with nothing in it is a popup in the way, so an empty offer is a dismissal
// rather than an empty box. The selection returns to the first candidate: the query
// changed, so which candidate was under the cursor is about a question nobody asked.
// The completion copies the token and candidates, including match offsets; the caller
// may reuse or change its inputs after this returns.
// Exhausting the collection revision panics before changing the offer or token.
func (c *Completion) Offer(t Token, candidates []Candidate) {
	requireMutable(c.element)
	if len(candidates) == 0 {
		c.Dismiss()
		return
	}
	c.list.requireReplacement()
	c.matcher.Reset(func() {
		c.token = t.clone()
		c.list.replaceItems(candidates, 0)
		for i := range c.list.items {
			c.list.items[i] = c.list.items[i].clone()
		}
		c.list.Scroll().ToTop()
		c.list.notifyChange()
	})
}

// Dismiss closes the completion and reports whether this dismissal retained its
// ownership through resolver cancellation. A newer Offer or Dismiss revokes it.
// Exhausting the collection revision panics before changing the offer or token.
func (c *Completion) Dismiss() bool {
	requireMutable(c.element)
	c.list.requireReplacement()
	return c.matcher.Reset(func() {
		c.token = Token{}
		c.list.replaceItems(nil, 0)
		c.list.notifyChange()
	})
}

// Open reports whether anything is being offered, which is what tells the interface
// around it to make room and to offer it keys first.
func (c *Completion) Open() bool {
	return c.list.Len() != 0 && c.valid(c.token)
}

func (c *Completion) valid(offered Token) bool {
	if c.currentToken == nil {
		return true
	}
	return readProjection(c.Element(), func() bool {
		token, ok := c.currentToken()
		return ok && token == offered
	})
}

// Token is what is being completed, and whether anything is.
func (c *Completion) Token() (Token, bool) {
	if c.Open() {
		return c.token, true
	}
	return Token{}, false
}

// Current is a snapshot of the candidate under the cursor, and whether there is one.
func (c *Completion) Current() (Candidate, bool) {
	if !c.Open() {
		return Candidate{}, false
	}
	candidate, ok := c.list.Current()
	return candidate.snapshot(), ok
}

func (c Candidate) snapshot() Candidate {
	c.Matched = slices.Clone(c.Matched)
	return c
}

func (c Candidate) clone() Candidate {
	c.Text = strings.Clone(c.Text)
	c.Label = strings.Clone(c.Label)
	c.Detail = strings.Clone(c.Detail)
	c.Matched = slices.Clone(c.Matched)
	return c
}

// Handle answers movement, acceptance and dismissal while the completion is open, and
// nothing at all while it is closed.
//
// A closed completion consuming keys would be a completion that had opinions about
// text it is not offering anything for.
func (c *Completion) Handle(ev input.Event) bool {
	requireMutable(c.element)
	if !c.Open() {
		return false
	}
	key, ok := ev.(input.Key)
	if !ok {
		// The wheel over an open completion is the list's, and nothing else about a
		// pointer is.
		return c.list.Handle(ev)
	}
	_, handled := c.matcher.Handle(c.resolvedKeys(), key, keyboardReceipt(&c.element), c.Do)
	return handled
}

// Do runs the completion's actions and the list's.
//
// The list is driven by name rather than handed the event, because both read through
// the same map: an event offered twice would be resolved twice, and the second lookup
// would know nothing about the first.
func (c *Completion) Do(action keymap.Action) bool {
	requireMutable(c.element)
	if !c.Open() {
		return false
	}
	switch action {
	case Dismiss:
		c.Dismiss()
		return true
	case Accept:
		_, handled := c.acceptIndex(c.list.Selected())
		return handled
	}
	return c.list.Do(action)
}

// A superseding offer consumes the physical key but did not accept its old
// candidate. The semantic caller needs that outcome separately.
func (c *Completion) acceptIndex(index int) (scene.ActionResult[Candidate], bool) {
	candidate, ok := c.list.At(index)
	accept, token := c.accept, c.token
	if !ok || accept == nil {
		return scene.ActionResult[Candidate]{}, false
	}
	if !c.Dismiss() {
		return scene.ActionResult[Candidate]{}, true
	}
	if !c.valid(token) {
		return scene.ActionResult[Candidate]{}, true
	}
	accept(candidate.snapshot(), token)
	return scene.ActionResult[Candidate]{State: scene.Applied, Value: candidate}, true
}

func (c *Completion) HeightForWidth(int) int {
	if !c.Open() {
		return 0
	}
	return min(c.list.Len(), c.rows())
}

// Width is how wide the widest row wants to be. The element reports the same
// width when measured across, so a panel or box around it adds its own frame
// instead of its caller adding the frame to this.
func (c *Completion) Width() int {
	var widest int
	c.Element().ReadOnly(func() { widest = c.widest() })
	return widest
}

// widest measures the rows; renderer callbacks need the read-only projection
// that measurement already holds.
func (c *Completion) widest() int {
	if !c.Open() {
		return 0
	}
	widest := 0
	renderer := c.resolvedRenderer()
	for _, candidate := range c.list.items {
		widest = max(widest, renderer.Width(c.forRenderer(candidate)))
	}
	return widest
}

func (c *Completion) measure(axis layout.Axis, width int) int {
	if axis == layout.Across {
		return c.widest()
	}
	return c.HeightForWidth(width)
}

// CompletionRenderer draws and measures one candidate row. The row is one cell tall;
// Width reports its intrinsic columns. The caller retains candidate ownership.
type CompletionRenderer interface {
	DrawRow(view grid.View, candidate Candidate, selected bool, look Look)
	Width(candidate Candidate) int
}

type DefaultCompletionRenderer struct{}

// Width includes the label, the detail and their gap.
func (DefaultCompletionRenderer) Width(candidate Candidate) int {
	width := text.Width(candidate.shown())
	if candidate.Detail != "" {
		width = layout.Sum(width, detailGap, text.Width(candidate.Detail))
	}
	return width
}

func (c *Completion) forRenderer(candidate Candidate) Candidate {
	if c.renderer != nil {
		return candidate.snapshot()
	}
	return candidate
}

func (c *Completion) resolvedRenderer() CompletionRenderer {
	if c.renderer != nil {
		return c.renderer
	}
	return DefaultCompletionRenderer{}
}

const detailGap = 2

func (c *Completion) Draw(v scene.Frame) {
	if !c.Open() {
		return
	}
	width, height := v.Size()
	renderer := c.resolvedRenderer()
	c.list.DrawRows(v.Sub(grid.Area(0, 0, width, min(height, c.rows()))), func(view grid.View, _ int, candidate Candidate, selected bool) {
		renderer.DrawRow(view, c.forRenderer(candidate), selected, c.look)
	})
}

func (DefaultCompletionRenderer) DrawRow(v grid.View, candidate Candidate, selected bool, look Look) {
	width, _ := v.Size()
	base := look.Text
	if selected {
		base = base.Merge(look.Selection)
		v.Fill(v.Bounds(), base)
	}
	label := text.Highlight(candidate.shown(), candidate.Matched, base, base.Merge(look.Accent))
	at := label.Draw(v, 0, 0)

	if candidate.Detail == "" {
		return
	}
	room := layout.Remaining(width, at, detailGap)
	if room <= 0 {
		return
	}
	detail := text.Of(candidate.Detail, base.Merge(look.Subtle)).Truncate(room, look.Ellipsis)
	detail.Draw(v, width-detail.Width(), 0)
}

func (c *Completion) rows() int {
	if c.maxRows > 0 {
		return c.maxRows
	}
	return DefaultCompletionRows
}

func (c *Completion) resolvedKeys() *keymap.Map { return c.keys.resolve(completionKeys) }

func (c *Completion) Look() Look { return c.look }

func (c *Completion) SetLook(value Look) {
	setting.Assign(c.element, &c.look, value)
}

func (c *Completion) CurrentToken() (Token, bool) {
	if c.currentToken == nil {
		return Token{}, false
	}
	var token Token
	var current bool
	c.Element().ReadOnly(func() { token, current = c.currentToken() })
	return token, current
}

func (c *Completion) SetCurrentToken(value func() (Token, bool)) {
	element := c.element
	requireMutable(element)
	c.matcher.Reset(func() {
		c.currentToken = value
		element.Invalidate()
	})
}

func (c *Completion) Renderer() CompletionRenderer { return c.renderer }

func (c *Completion) SetRenderer(value CompletionRenderer) {
	setting.Replace(c.element, &c.renderer, value)
}

func (c *Completion) MaxRows() int { return c.maxRows }

// SetMaxRows panics for a negative limit. Zero selects DefaultCompletionRows.
func (c *Completion) SetMaxRows(value int) {
	requireNonNegative("completion MaxRows", value)
	setting.Assign(c.element, &c.maxRows, value)
}

func (c *Completion) Accept() func(Candidate, Token) { return c.accept }

func (c *Completion) SetAccept(value func(Candidate, Token)) {
	element := c.element
	requireMutable(element)
	c.matcher.Reset(func() {
		c.accept = value
		element.Invalidate()
	})
}

func (c *Completion) Keys() *keymap.Map { return c.keys.current }

// KeysFor is what the effective key map binds to action, defaults included.
func (c *Completion) KeysFor(action keymap.Action) []input.Keys {
	return c.keys.bound(completionKeys, action)
}

func (c *Completion) SetKeys(keys *keymap.Map) {
	c.keys.set(c.element, &c.matcher, keys)
}

// Element is the completion's one candidate-list identity. It receives pointer
// input while keyboard focus remains with the composing editor.
func (c *Completion) Element() scene.Element {
	if c.element.IsZero() {
		accept := scene.NewAction(scene.ActionConfig[CollectionIndex, Candidate]{ID: scene.ActionActivate, Label: "Accept completion", Availability: func(address CollectionIndex) scene.Availability {
			if !c.Open() || c.accept == nil {
				return scene.Unavailable("completion is not accepting")
			}
			return c.list.indexAvailability(address)
		}, Run: func(_ context.Context, address CollectionIndex) (scene.ActionResult[Candidate], error) {
			result, _ := c.acceptIndex(address.Index)
			return result, nil
		}})
		dismiss := scene.NewAction(scene.ActionConfig[struct{}, struct{}]{ID: scene.ActionDismiss, Label: "Dismiss completion", Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
			state := scene.AppliedIf(c.Dismiss())
			return scene.ActionResult[struct{}]{State: state}, nil
		}})
		c.element = scene.NewElement(scene.ElementConfig{Enabled: c.Open, Measure: c.measure, Paint: c.Draw, Handle: func(event scene.Event) scene.EventResult {
			return partEvent(event, c.Handle, &c.list.geometry, &c.list.pointer.geometry)
		}, Semantics: func() scene.Semantic {
			return scene.Semantic{Role: scene.RoleList, ContentRevision: c.list.contentRevision, Value: strconv.Itoa(c.list.Selected())}
		}, Actions: append([]scene.ActionBinding{accept.Binding(), dismiss.Binding()}, collectionActions(&c.list, Candidate.snapshot)...)})
	}
	c.list.adopt(c.element)
	c.keys.attach(c.element, &c.matcher)
	return c.element
}
