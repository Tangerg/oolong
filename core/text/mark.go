package text

// Edit is a range of a document replaced by other text.
//
// It is the only shape a change to text has. An insertion is an empty range with
// text, a deletion is a range with none, and a replacement is both — so anything
// that has to react to a change reacts to one thing rather than to three.
//
// Offsets are bytes from the start of the whole document. Not lines and columns:
// how a document is stored is the business of whatever stores it, and a change
// described in line numbers could only be applied to something that had them.
type Edit struct {
	// Start and End are the range replaced, the end exclusive.
	Start, End int
	// Text is what goes there.
	Text string
}

// Apply is the document with the edit made.
//
// A range outside the document is clamped to it, and one the wrong way round is
// taken the right way round, so an edit worked out from a position that has since
// moved cannot panic.
func (e Edit) Apply(document string) string {
	e = e.within(len(document))
	if e.empty() {
		return document
	}
	return document[:e.Start] + e.Text + document[e.End:]
}

// Delta is how much longer a document of length n gets, which is negative when it
// shrinks. The edit is clamped to that document before the difference is measured,
// exactly as it is by [Edit.Apply] and [Edit.Shift].
func (e Edit) Delta(n int) int {
	e = e.within(n)
	return e.delta()
}

// bounds is the edit's range, put in order and held inside a document of length n.
func (e Edit) bounds(n int) (start, end int) {
	n = max(n, 0)
	start, end = e.Start, e.End
	if start > end {
		start, end = end, start
	}
	start = min(max(start, 0), n)
	end = min(max(end, start), n)
	return start, end
}

// within is the canonical edit that can be applied to a document of length n.
func (e Edit) within(n int) Edit {
	e.Start, e.End = e.bounds(n)
	return e
}

func (e Edit) delta() int { return len(e.Text) - (e.End - e.Start) }

// empty reports the one edit that is an identity for every document. A replacement
// with equal lengths may or may not replace the same bytes; an empty range with no
// text never changes anything.
func (e Edit) empty() bool { return e.Start == e.End && e.Text == "" }

// Mark is a range of a document that moves as the document is edited.
//
// It is how anything can be said about a piece of text without being said in the
// text: which run of it stands for a file the user picked, which run is a search
// result, which run somebody spelled wrong. Every one of those is a range that has
// to still be over the same words after something is typed somewhere else, and
// keeping them in step is the same problem each time — see [Edit.Shift].
//
// The identity and the kind mean nothing here. A caller keeps whatever the mark
// stands for beside it, keyed by the identity, and this only promises that the
// identity survives as long as the mark does.
type Mark struct {
	// ID is the caller's handle on the mark. Nothing here reads it.
	ID uint64
	// Kind is the caller's own label, for telling one family of marks from another.
	Kind int
	// Start and End are the byte range, the end exclusive.
	Start, End int
	// Atomic says the mark stands for one thing, so an edit reaching inside it
	// destroys it rather than stretching it.
	//
	// A chip in a prompt naming a file is atomic: text typed into the middle of it
	// names something else, and a chip that still looks like a file and points at
	// half of one is worse than no chip. A highlight is not: it is about the text it
	// covers, and text inserted in the middle of it is still covered.
	Atomic bool
}

// Empty reports whether the mark covers nothing.
func (m Mark) Empty() bool { return m.Start >= m.End }

// Covers reports whether a byte offset is inside the mark. The end is exclusive, so
// the offset just after a mark is outside it.
func (m Mark) Covers(at int) bool { return at >= m.Start && at < m.End }

// Within reports whether an offset is strictly inside the mark.
//
// Strictly, unlike [Mark.Covers]: a mark's two ends are places a cursor may sit, and
// only what is between them is not. They are different questions and the difference
// matters — treating the start as inside would mean a cursor arriving from the left
// skipped straight past the mark, and nothing could be typed in front of one.
func (m Mark) Within(at int) bool { return at > m.Start && at < m.End }

// Shift moves marks over the edit in a document of length n, in order, dropping the
// ones it destroyed.
//
// The edit is clamped to the document first, by the same rule as [Edit.Apply]. The
// length is therefore part of the operation rather than an optional validation
// hint: without it an edit before byte zero would replace one range in the text and
// move its metadata as though it had replaced a different one.
//
// # Which way a mark moves at the edges
//
// Text inserted exactly where a mark begins goes before it, and text inserted exactly
// where a mark ends goes after it. So typing on either side of a chip in a prompt
// leaves the chip the length it was, which is the only answer that lets a user type
// up against one — the other would swallow the next thing they wrote.
//
// # What happens to a mark the edit reached into
//
// An empty edit changes neither text nor marks. Its position may be inside a mark,
// but a position on its own did not reach into anything.
//
// An atomic mark is dropped: half of a thing that stood for something is not a
// smaller thing, it is a fragment that still looks like the thing and no longer is.
// Any other mark stretches to cover what replaced the part the edit took — and is
// dropped too if the edit took all of it, because a range covering nothing says
// nothing about the text and a caller keying a record off it would keep it for ever.
//
// The marks are shifted in place, which is what a caller that keeps them in a slice
// wants. The result is the slice with the destroyed ones removed, so a caller that
// held a mark by value has to find it again by identity.
func (e Edit) Shift(marks []Mark, n int) []Mark {
	e = e.within(n)
	if e.empty() {
		return marks
	}
	kept := marks[:0]
	for _, m := range marks {
		if m.Atomic && e.reaches(m) {
			continue
		}
		m.Start, m.End = e.opens(m.Start), e.closes(m.End)
		if m.Empty() {
			continue
		}
		kept = append(kept, m)
	}
	return kept
}

// reaches reports whether the edit's range extends strictly inside the mark.
//
// Strictly: an edit that only meets a mark at one of its ends took none of it, which
// is what makes deleting the character before a chip leave the chip alone.
func (e Edit) reaches(m Mark) bool {
	return e.Start < m.End && e.End > m.Start
}

// opens maps an offset that begins something: text inserted exactly there goes
// before it, so the offset moves along.
func (e Edit) opens(at int) int {
	switch {
	case at < e.Start:
		return at
	case at >= e.End:
		return at + e.delta()
	default:
		// Inside what was replaced. What began there begins where the new text does.
		return e.Start + len(e.Text)
	}
}

// closes maps an offset that ends something: text inserted exactly there goes after
// it, so the offset stays where it is.
func (e Edit) closes(at int) int {
	switch {
	case at <= e.Start:
		return at
	case at >= e.End:
		return at + e.delta()
	default:
		// Inside what was replaced. What ended there ends where the new text does.
		return e.Start + len(e.Text)
	}
}
