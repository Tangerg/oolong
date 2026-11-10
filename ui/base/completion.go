package base

import (
	"math/bits"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Trigger is what opens a completion: the characters that begin a token, and where
// they count as beginning one.
type Trigger struct {
	Prefix  string
	AtStart bool
}

// Token is the run of text a completion is being offered for.
type Token struct {
	// Line distinguishes identical ranges in a multiline editor. TokenAt reads one line.
	Line int
	// Start and End are the byte range in the line that accepting a candidate
	// replaces: everything after the prefix, through the end of the token. The prefix
	// itself stays where it is — it is what the user typed to ask for the completion,
	// not part of the answer.
	Start, End int
	// Query is that range up to the cursor, which is what candidates are matched
	// against. What is after the cursor is replaced but not matched: a cursor put back
	// into the middle of a word is a request to reconsider its beginning.
	Query string
	// Trigger is what opened the token, so that a caller offering several can tell
	// which kind of thing is being asked for.
	Trigger Trigger
}

// TokenAt finds the token the cursor is inside, if any. The rightmost trigger before
// the cursor wins, so a file mentioned inside a command completes as a file.
//
// A token ends at the first space after it, which is the one rule general enough to
// be worth having: anything more — a path that may contain spaces, a quote that
// protects them — is the caller's grammar and not a terminal library's.
// Invalid UTF-8, out-of-range cursors and partial graphemes have no token. Both
// the query and replacement range must preserve the editor's text boundaries.
func TokenAt(line string, cursor int, triggers ...Trigger) (Token, bool) {
	if len(triggers) == 0 || cursor < 0 || cursor > len(line) || !utf8.ValidString(line) {
		return Token{}, false
	}
	boundaries := indexClusterBoundaries(line)
	if !boundaries.contains(cursor) {
		return Token{}, false
	}
	best, at := Token{}, -1
	for _, trigger := range triggers {
		if trigger.Prefix == "" || !utf8.ValidString(trigger.Prefix) {
			continue
		}
		opened, ok := trigger.openedBefore(line, cursor, at, boundaries)
		if !ok {
			continue
		}
		start := opened + len(trigger.Prefix)
		end := tokenEnd(line, start)
		if cursor < start || cursor > end || !boundaries.contains(end) {
			continue
		}
		best, at = Token{
			Start:   start,
			End:     end,
			Query:   line[start:cursor],
			Trigger: trigger,
		}, opened
	}
	return best, at >= 0
}

// Rejected prefixes must not reparse every preceding grapheme. This local index
// shares one segmentation pass and bounds scratch storage to one bit per byte offset.
type clusterBoundaries []uint

func indexClusterBoundaries(line string) clusterBoundaries {
	boundaries := make(clusterBoundaries, len(line)/bits.UintSize+1)
	boundaries[0] = 1
	for at, cluster := range text.Clusters(line) {
		end := at + len(cluster)
		boundaries[end/bits.UintSize] |= 1 << (end % bits.UintSize)
	}
	return boundaries
}

func (b clusterBoundaries) contains(at int) bool {
	return at >= 0 && at/bits.UintSize < len(b) && b[at/bits.UintSize]&(1<<(at%bits.UintSize)) != 0
}

func (t Trigger) openedBefore(line string, cursor, after int, boundaries clusterBoundaries) (int, bool) {
	for end := cursor; end >= len(t.Prefix); {
		candidate := strings.LastIndex(line[:end], t.Prefix)
		if candidate < 0 || candidate < after {
			return 0, false
		}
		if boundaries.contains(candidate) && boundaries.contains(candidate+len(t.Prefix)) && t.opensAt(line, candidate) {
			return candidate, true
		}
		end = candidate + len(t.Prefix) - 1
	}
	return 0, false
}

func (t Trigger) opensAt(line string, at int) bool {
	if t.AtStart {
		return at == 0
	}
	if at == 0 {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(line[:at])
	return !unicode.IsLetter(prev) && !unicode.IsDigit(prev) && prev != '_'
}

func tokenEnd(line string, start int) int {
	for end := start; end < len(line); {
		r, size := utf8.DecodeRuneInString(line[end:])
		if unicode.IsSpace(r) {
			return end
		}
		end += size
	}
	return len(line)
}

// TokenAtCursor reads the token on the current line. Masked text cannot be offered to a completion provider.
func (e *Editor) TokenAtCursor(triggers ...Trigger) (Token, bool) {
	lines := e.document().lines
	if e.Mask() != "" || e.line < 0 || e.line >= len(lines) {
		return Token{}, false
	}
	token, ok := TokenAt(lines[e.line], e.byteOffset, triggers...)
	if ok {
		token.Line = e.line
	}
	return token, ok
}

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

	list    List[Candidate]
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
func (c *Completion) Offer(t Token, candidates []Candidate) {
	requireMutable(c.element)
	c.matcher.Reset(func() {
		if len(candidates) == 0 {
			c.Dismiss()
			return
		}
		c.token = cloneToken(t)
		c.list.replaceItems(candidates, 0)
		for i := range c.list.items {
			c.list.items[i] = cloneCandidate(c.list.items[i])
		}
		c.list.Scroll().ToTop()
		c.list.notifyChange()
	})
}

// Dismiss closes the completion and reports whether this dismissal retained its
// ownership through resolver cancellation. A newer Offer or Dismiss revokes it.
func (c *Completion) Dismiss() bool {
	requireMutable(c.element)
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

func cloneToken(token Token) Token {
	token.Query = strings.Clone(token.Query)
	token.Trigger.Prefix = strings.Clone(token.Trigger.Prefix)
	return token
}

func cloneCandidate(candidate Candidate) Candidate {
	candidate.Text = strings.Clone(candidate.Text)
	candidate.Label = strings.Clone(candidate.Label)
	candidate.Detail = strings.Clone(candidate.Detail)
	candidate.Matched = slices.Clone(candidate.Matched)
	return candidate
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

// Do runs one of the completion's actions by name, or the list's, reporting whether it
// was one either of them knows. See [Doer].
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

// Width is how wide the widest row wants to be, so a caller sizing a layer around it
// does not have to measure the candidates itself.
func (c *Completion) Width() int {
	if !c.Open() {
		return 0
	}
	widest := 0
	renderer := c.resolvedRenderer()
	c.Element().ReadOnly(func() {
		for _, candidate := range c.list.items {
			widest = max(widest, renderer.Width(c.forRenderer(candidate)))
		}
	})
	return widest
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
	detail := base.Merge(look.Subtle)
	room := layout.Remaining(width, at, detailGap)
	if room <= 0 {
		return
	}
	v.Text(width-min(room, text.Width(candidate.Detail)), 0,
		text.Truncate(candidate.Detail, room, look.Ellipsis), detail)
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
	element := c.Element()
	requireMutable(element)
	if c.look == value {
		return
	}
	c.look = value
	element.Invalidate()
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
	element := c.Element()
	requireMutable(element)
	c.matcher.Reset(func() {
		c.currentToken = value
		element.Invalidate()
	})
}

func (c *Completion) Renderer() CompletionRenderer { return c.renderer }

func (c *Completion) SetRenderer(value CompletionRenderer) {
	element := c.Element()
	requireMutable(element)
	c.renderer = value
	element.Invalidate()
}

func (c *Completion) MaxRows() int { return c.maxRows }

// SetMaxRows panics for a negative limit. Zero selects DefaultCompletionRows.
func (c *Completion) SetMaxRows(value int) {
	element := c.Element()
	requireMutable(element)
	requireNonNegative("completion MaxRows", value)
	if c.maxRows == value {
		return
	}
	c.maxRows = value
	element.Invalidate()
}

func (c *Completion) Accept() func(Candidate, Token) { return c.accept }

func (c *Completion) SetAccept(value func(Candidate, Token)) {
	element := c.Element()
	requireMutable(element)
	c.matcher.Reset(func() {
		c.accept = value
		element.Invalidate()
	})
}

func (c *Completion) Keys() *keymap.Map { return c.keys.current }

func (c *Completion) SetKeys(keys *keymap.Map) {
	c.keys.set(c.Element(), &c.matcher, keys)
}
