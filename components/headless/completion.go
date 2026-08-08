package headless

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/text"
)

// Trigger is what opens a completion: the characters that begin a token, and where
// they count as beginning one.
type Trigger struct {
	// Prefix begins the token — "@" for a file, "/" for a command, ":" for an emoji.
	Prefix string
	// AtStart limits it to the beginning of the line, which is what makes "/help" a
	// command and "and/or" not one.
	AtStart bool
}

// Token is the run of text a completion is being offered for.
type Token struct {
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
func TokenAt(line string, cursor int, triggers ...Trigger) (Token, bool) {
	cursor = min(max(cursor, 0), len(line))
	best, at := Token{}, -1
	for _, trigger := range triggers {
		if trigger.Prefix == "" {
			continue
		}
		opened := strings.LastIndex(line[:cursor], trigger.Prefix)
		if opened < 0 || opened < at {
			continue
		}
		if trigger.AtStart && opened != 0 {
			continue
		}
		if !trigger.AtStart && opened > 0 {
			// A trigger inside a word is part of the word: the @ of an email address
			// is not a request to complete a filename.
			prev, _ := utf8.DecodeLastRuneInString(line[:opened])
			if unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_' {
				continue
			}
		}
		start := opened + len(trigger.Prefix)
		if cursor < start {
			continue
		}
		end := start
		for end < len(line) {
			r, size := utf8.DecodeRuneInString(line[end:])
			if unicode.IsSpace(r) {
				break
			}
			end += size
		}
		if cursor > end {
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

// Candidate is one thing a completion offers.
type Candidate struct {
	// Text is what accepting it puts in place of the token.
	Text string
	// Label is the row as shown. Empty shows Text, which is the common case: what is
	// offered and what it inserts are usually the same thing.
	Label string
	// Detail is shown after the label, receding — a description, a kind, a size.
	Detail string
	// Matched are byte offsets in the label that the query matched, picked out as the
	// row is drawn. It is what [fuzzy.Match.At] returns, and leaving it nil simply
	// highlights nothing.
	Matched []int
}

// shown is the label, or the text when there is no label.
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
// caller that wants it over the text puts it in an [Overlay], which is the piece that
// owns placement.
type Completion struct {
	// Look is how the rows are drawn: the text, the row under the cursor, the
	// characters the query matched, and the detail beside a candidate. It is the one
	// way anything here that draws itself is dressed — see [Look].
	Look Look
	// MaxRows caps how tall the list gets, so a thousand files do not become a
	// thousand rows. Zero uses [DefaultCompletionRows].
	MaxRows int
	// Keys say which keystrokes produce which of the actions a completion answers to —
	// its own two, and the list movement inside it. Nil reads through
	// [DefaultCompletionKeys].
	Keys *keymap.Map
	// Accept is called when the user takes a candidate, with the token it replaces.
	// The completion has closed itself by then, so this is free to change the text the
	// token came from.
	//
	// Without it there is nothing accepting could do, and the keystroke is left for
	// whatever else might want it rather than swallowed.
	Accept func(c Candidate, t Token)

	list  List[Candidate]
	token Token
	open  bool
	// pending is how far into a multi-chord binding the keys typed so far have got.
	pending keymap.Pending
}

// DefaultCompletionRows is how many candidates are shown at once when nothing says
// otherwise: enough to choose from, few enough to leave the text visible behind it.
const DefaultCompletionRows = 8

// Offer opens the completion on a token, or closes it when there is nothing to offer.
//
// A popup with nothing in it is a popup in the way, so an empty offer is a dismissal
// rather than an empty box. The selection returns to the first candidate: the query
// changed, so which candidate was under the cursor is about a question nobody asked.
// The completion copies the token and candidates, including match offsets; the caller
// may reuse or change its inputs after this returns.
func (c *Completion) Offer(t Token, candidates []Candidate) {
	if len(candidates) == 0 {
		c.Dismiss()
		return
	}
	c.token, c.open = cloneToken(t), true
	c.list.SetItems(candidates)
	for i := range c.list.items {
		c.list.items[i] = cloneCandidate(c.list.items[i])
	}
	// The row renderer is wired here rather than while drawing: a Draw that assigns
	// to the thing it is about to draw is a Draw with a side effect, and this is the
	// one place the list's contents change anyway.
	c.list.Row = c.drawRow
	c.list.Select(0)
}

// Dismiss closes the completion.
func (c *Completion) Dismiss() {
	c.open = false
	c.token = Token{}
	c.list.SetItems(nil)
	c.list.Select(0)
}

// Open reports whether anything is being offered, which is what tells the interface
// around it to make room and to offer it keys first.
func (c *Completion) Open() bool { return c.open && c.list.Len() > 0 }

// Token is what is being completed, and whether anything is.
func (c *Completion) Token() (Token, bool) { return c.token, c.Open() }

// Current is a snapshot of the candidate under the cursor, and whether there is one.
func (c *Completion) Current() (Candidate, bool) {
	if !c.Open() {
		return Candidate{}, false
	}
	candidate, ok := c.list.Current()
	return cloneCandidate(candidate), ok
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
	if !c.Open() {
		return false
	}
	key, ok := ev.(input.Key)
	if !ok {
		// The wheel over an open completion is the list's, and nothing else about a
		// pointer is.
		return c.list.Handle(ev)
	}
	action, mine := c.keys().Lookup(key, &c.pending)
	switch {
	case !mine:
		return false
	case action == "":
		return true // the start of a binding more than one chord long
	}
	return c.Do(action)
}

// Do runs one of the completion's actions by name, or the list's, reporting whether it
// was one either of them knows. See [Doer].
//
// The list is driven by name rather than handed the event, because both read through
// the same map: an event offered twice would be resolved twice, and the second lookup
// would know nothing about the first.
func (c *Completion) Do(action keymap.Action) bool {
	if !c.Open() {
		return false
	}
	switch action {
	case Dismiss:
		c.Dismiss()
		return true
	case Accept:
		if c.Accept == nil {
			// Nothing accepting could do, so the keystroke is left for whatever else
			// might want it rather than swallowed.
			return false
		}
		candidate, ok := c.list.Current()
		if !ok {
			return false
		}
		token := c.token
		// Closed before the callback, so what it does to the text cannot be
		// interpreted as another query for a completion that is still open.
		c.Dismiss()
		c.Accept(candidate, token)
		return true
	}
	return c.list.Do(action)
}

// Measure is how tall the list wants to be: a row per candidate, capped.
func (c *Completion) Measure(int) int {
	if !c.Open() {
		return 0
	}
	return min(c.list.Len(), c.rows())
}

// Width is how wide the widest row wants to be, so a caller sizing a layer around it
// does not have to measure the candidates itself.
func (c *Completion) Width() int {
	widest := 0
	for _, candidate := range c.list.items {
		w := text.Width(candidate.shown())
		if candidate.Detail != "" {
			w += detailGap + text.Width(candidate.Detail)
		}
		widest = max(widest, w)
	}
	return widest
}

// detailGap is what separates a row's label from its detail.
const detailGap = 2

// Draw renders the visible candidates.
func (c *Completion) Draw(v Frame) {
	if !c.Open() {
		return
	}
	width, height := v.Size()
	c.list.Draw(v.Sub(grid.Rect(0, 0, width, min(height, c.rows()))))
}

// drawRow draws one candidate: its label with the matched characters picked out, and
// its detail pushed to the right.
func (c *Completion) drawRow(v grid.View, _ int, candidate Candidate, selected bool) {
	width, _ := v.Size()
	base := c.Look.Text
	if selected {
		base = base.Merge(c.Look.Selection)
		v.Fill(v.Bounds(), base)
	}
	at := c.drawMatched(v, candidate.shown(), candidate.Matched, base)

	if candidate.Detail == "" {
		return
	}
	detail := base.Merge(c.Look.Subtle)
	room := width - at - detailGap
	if room <= 0 {
		return
	}
	v.Text(width-min(room, text.Width(candidate.Detail)), 0,
		text.Truncate(candidate.Detail, room, "…"), detail)
}

// drawMatched writes label, emphasising the clusters the query matched, and returns
// the column it reached.
//
// The matched offsets can land inside a grapheme cluster, so a cluster is emphasised
// when it contains one rather than when it begins at one: a pattern character that
// matched a combining mark is still that cluster being matched.
func (c *Completion) drawMatched(v grid.View, label string, matched []int, base grid.Style) int {
	hit := base.Merge(c.Look.Accent)
	next, at := 0, 0
	for off, cluster := range text.Clusters(label) {
		for next < len(matched) && matched[next] < off {
			next++
		}
		style := base
		if next < len(matched) && matched[next] < off+len(cluster) {
			style = hit
		}
		at += v.Text(at, 0, cluster, style)
	}
	return at
}

func (c *Completion) rows() int {
	if c.MaxRows > 0 {
		return c.MaxRows
	}
	return DefaultCompletionRows
}

// keys is the map to read through, standing in the default for a caller who set none.
func (c *Completion) keys() *keymap.Map {
	if c.Keys != nil {
		return c.Keys
	}
	return completionKeys()
}
