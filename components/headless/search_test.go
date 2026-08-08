package headless_test

import (
	"testing"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// found waits for the answer to a query, ignoring answers to older ones.
func found(t *testing.T, s *headless.Search, query string) headless.Result {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case r, ok := <-s.Results():
			if !ok {
				t.Fatalf("search closed before answering %q", query)
			}
			if r.Query == query {
				return r
			}
		case <-deadline:
			t.Fatalf("no answer to %q arrived", query)
			return headless.Result{}
		}
	}
}

func searching(t *testing.T) *headless.Search {
	t.Helper()
	s := headless.NewSearch()
	t.Cleanup(s.Close)
	return s
}

func waitForUnreadResult(t *testing.T, s *headless.Search) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for len(s.Results()) == 0 {
		select {
		case <-deadline:
			t.Fatal("the search produced no result")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestSearchFindsEveryOccurrence(t *testing.T) {
	tr := transcriptOf(plainRows("the cat sat", "on the mat", "with a cat")...)
	s := searching(t)

	s.Submit(tr, "cat", false)
	r := found(t, s, "cat")
	if r.Err != nil {
		t.Fatalf("err = %v", r.Err)
	}
	if len(r.Matches) != 2 {
		t.Fatalf("found %d matches, want 2: %+v", len(r.Matches), r.Matches)
	}
	if got := r.Matches[0]; got.Row != 0 || len(got.Spans) != 1 || got.Spans[0] != (headless.Span{Col: 4, Width: 3}) {
		t.Errorf("first match = %+v, want row 0 columns 4..7", got)
	}
	if got := r.Matches[1]; got.Row != 2 || got.Spans[0] != (headless.Span{Col: 7, Width: 3}) {
		t.Errorf("second match = %+v, want row 2 columns 7..10", got)
	}
}

func TestSearchIgnoresCase(t *testing.T) {
	tr := transcriptOf(plainRows("The Cat SAT")...)
	s := searching(t)
	s.Submit(tr, "cat", false)
	if got := found(t, s, "cat"); len(got.Matches) != 1 {
		t.Errorf("found %d matches, want 1", len(got.Matches))
	}
}

// TestSearchKeepsOffsetsRightWhereCaseFoldingChangesLength is why the literal path
// goes through the same matcher as a pattern. Lowercasing both sides and comparing
// gives offsets into a string nobody has: this Turkish capital folds to two runes,
// and everything after it would be reported a byte out.
func TestSearchKeepsOffsetsRightWhereCaseFoldingChangesLength(t *testing.T) {
	tr := transcriptOf(plainRows("İstanbul then cat")...)
	s := searching(t)
	s.Submit(tr, "cat", false)

	r := found(t, s, "cat")
	if len(r.Matches) != 1 {
		t.Fatalf("found %d matches, want 1", len(r.Matches))
	}
	// "İstanbul then " is fourteen columns wide, whatever it is in bytes.
	if got := r.Matches[0].Spans[0]; got != (headless.Span{Col: 14, Width: 3}) {
		t.Errorf("match at %+v, want column 14 width 3", got)
	}
}

func TestSearchReportsColumnsAndNotBytes(t *testing.T) {
	// A match after a wide character does not begin at the column its byte offset
	// names, and columns are what a highlight is painted in.
	tr := transcriptOf(plainRows("中文 cat")...)
	s := searching(t)
	s.Submit(tr, "cat", false)

	r := found(t, s, "cat")
	if len(r.Matches) != 1 {
		t.Fatalf("found %d matches, want 1", len(r.Matches))
	}
	if got := r.Matches[0].Spans[0]; got != (headless.Span{Col: 5, Width: 3}) {
		t.Errorf("match at %+v, want column 5 width 3", got)
	}
}

func TestSearchReportsTheRenderedContentOffset(t *testing.T) {
	tr := transcriptOf(text.Row{Text: "find me", Offset: 3})
	s := searching(t)
	s.Submit(tr, "me", false)

	result := found(t, s, "me")
	if len(result.Matches) != 1 || result.Matches[0].Spans[0] != (headless.Span{Col: 8, Width: 2}) {
		t.Fatalf("offset match = %+v, want columns 8..10", result.Matches)
	}
}

// TestSearchFindsAPhraseTheWidthBroke is the reason the corpus is joined rather than
// scanned row by row. A long phrase is exactly the one a user searches for, and it is
// exactly the one the window is likeliest to have split.
func TestSearchFindsAPhraseTheWidthBroke(t *testing.T) {
	tr := transcriptOf(
		text.Row{Text: "the quick"},
		text.Row{Text: "brown fox", Joined: true, Gap: " "},
	)
	s := searching(t)
	s.Submit(tr, "quick brown", false)

	r := found(t, s, "quick brown")
	if len(r.Matches) != 1 {
		t.Fatalf("found %d matches, want 1: %+v", len(r.Matches), r.Matches)
	}
	m := r.Matches[0]
	if m.Row != 0 {
		t.Errorf("match begins on row %d, want 0", m.Row)
	}
	if len(m.Spans) != 2 {
		t.Fatalf("match covers %d rows, want 2: %+v", len(m.Spans), m.Spans)
	}
	if m.Spans[0] != (headless.Span{Col: 4, Width: 5}) {
		t.Errorf("the first row's span is %+v, want columns 4..9", m.Spans[0])
	}
	if m.Spans[1] != (headless.Span{Col: 0, Width: 5}) {
		t.Errorf("the second row's span is %+v, want columns 0..5", m.Spans[1])
	}
}

func TestSearchDoesNotRunAPhraseAcrossARealLineBreak(t *testing.T) {
	// Two lines the text broke, not the width. "sat on" is not in this text.
	tr := transcriptOf(plainRows("the cat sat", "on the mat")...)
	s := searching(t)
	s.Submit(tr, "sat on", false)
	if got := found(t, s, "sat on"); len(got.Matches) != 0 {
		t.Errorf("found %+v across a line break the text made", got.Matches)
	}
}

func TestSearchTakesAPattern(t *testing.T) {
	tr := transcriptOf(plainRows("error 42 and error 7")...)
	s := searching(t)
	s.Submit(tr, `error \d+`, true)

	r := found(t, s, `error \d+`)
	if len(r.Matches) != 2 {
		t.Fatalf("found %d matches, want 2", len(r.Matches))
	}
}

func TestSearchTreatsALiteralQueryLiterally(t *testing.T) {
	tr := transcriptOf(plainRows("a+b and aab")...)
	s := searching(t)
	s.Submit(tr, "a+b", false)
	if got := found(t, s, "a+b"); len(got.Matches) != 1 {
		t.Errorf("found %d matches, want only the literal one", len(got.Matches))
	}
}

func TestSearchReportsAPatternThatWillNotCompile(t *testing.T) {
	// A user typing a pattern spends most of the typing with an unfinished one, so
	// this is a result rather than an interruption.
	tr := transcriptOf(plainRows("anything")...)
	s := searching(t)
	s.Submit(tr, "(unclosed", true)
	if got := found(t, s, "(unclosed"); got.Err == nil {
		t.Error("a pattern that cannot compile produced no error")
	}
}

func TestSearchAnswersNothingWhenNothingWasAsked(t *testing.T) {
	tr := transcriptOf(plainRows("anything")...)
	s := searching(t)
	s.Submit(tr, "", false)
	s.Submit(nil, "cat", false)

	select {
	case r := <-s.Results():
		t.Errorf("an answer arrived anyway: %+v", r)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestClearingSearchCancelsAnUnreadAnswer(t *testing.T) {
	tr := transcriptOf(plainRows("the cat sat")...)
	s := searching(t)
	s.Submit(tr, "cat", false)
	waitForUnreadResult(t, s)

	s.Submit(tr, "", false)
	select {
	case r := <-s.Results():
		t.Errorf("clearing the query left an answer behind: %+v", r)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestSearchAnswersTheNewestQuery: the answer to a query three keystrokes old is
// worth nothing, so a burst produces one answer and it is the last one.
func TestSearchAnswersTheNewestQuery(t *testing.T) {
	var tr headless.Transcript
	tr.Resize(80)
	for range 200 {
		tr.Append(&lines{rows: plainRows("the cat sat on the mat with another cat")})
	}
	s := searching(t)

	for _, q := range []string{"c", "ca", "cat", "cat ", "cat s"} {
		s.Submit(&tr, q, false)
	}
	r := found(t, s, "cat s")
	if len(r.Matches) != 200 {
		t.Errorf("found %d matches of the newest query, want 200", len(r.Matches))
	}
}

// TestSearchDoesNotOwnTheTranscriptBetweenScans. A cached row snapshot would keep
// committed text alive after the transcript released it. Taking the live rows for each
// scan does not change the scan's linear complexity and leaves ownership unambiguous.
func TestSearchDoesNotOwnTheTranscriptBetweenScans(t *testing.T) {
	var tr headless.Transcript
	tr.Resize(80)
	counted := &counting{rows: plainRows("the cat sat")}
	tr.Append(counted)
	s := searching(t)

	s.Submit(&tr, "cat", false)
	found(t, s, "cat")
	afterFirst := counted.calls

	s.Submit(&tr, "sat", false)
	found(t, s, "sat")
	if counted.calls <= afterFirst {
		t.Errorf("the second scan reused transcript storage: %d calls then %d", afterFirst, counted.calls)
	}

	// A change is scanned from the same single-owner snapshot path.
	afterSecond := counted.calls
	tr.Append(&lines{rows: plainRows("another")})
	s.Submit(&tr, "cat", false)
	found(t, s, "cat")
	if counted.calls <= afterSecond {
		t.Error("the rows were not taken again after the transcript changed")
	}
}

func TestSearchReportsRowsInTheLiveCoordinateSpaceAfterCommit(t *testing.T) {
	var tr headless.Transcript
	tr.Resize(80)
	first := tr.Append(&lines{rows: plainRows("finished")})
	tr.Append(&lines{rows: plainRows("find the needle")})
	tr.Finish(first)
	tr.Commit(func(headless.Block, int) bool { return true })

	s := searching(t)
	s.Submit(&tr, "needle", false)
	r := found(t, s, "needle")
	if len(r.Matches) != 1 || r.Matches[0].Row != tr.StartRow() {
		t.Fatalf("matches are %+v, want one at live row %d", r.Matches, tr.StartRow())
	}
}

func TestSearchCloseIsSafeTwice(t *testing.T) {
	s := headless.NewSearch()
	s.Close()
	s.Close()

	// And submitting to a closed search neither blocks nor answers. The result stream
	// closes with its sole producer, so consumers ranging over it can stop as well.
	s.Submit(transcriptOf(plainRows("x")...), "x", false)
	select {
	case r, ok := <-s.Results():
		if ok {
			t.Errorf("a closed search answered with %+v", r)
		}
	case <-time.After(time.Second):
		t.Fatal("the result stream remained open after Search.Close")
	}
}

func TestTheZeroSearchIsStopped(t *testing.T) {
	var zero headless.Search
	zero.Submit(transcriptOf(plainRows("x")...), "x", false)
	zero.Close()
	zero.Close()
	var nilSearch *headless.Search
	nilSearch.Submit(transcriptOf(plainRows("x")...), "x", false)
	nilSearch.Close()

	for name, results := range map[string]<-chan headless.Result{
		"zero": zero.Results(),
		"nil":  nilSearch.Results(),
	} {
		select {
		case _, ok := <-results:
			if ok {
				t.Errorf("%s search produced a result", name)
			}
		default:
			t.Errorf("%s search returned a live or nil result stream", name)
		}
	}
}

// TestSearchReplacesAnAnswerNobodyRead. A result waiting to be read answered an older
// query, and the newer answer is the one worth having.
func TestSearchReplacesAnAnswerNobodyRead(t *testing.T) {
	tr := transcriptOf(plainRows("the cat sat on the mat")...)
	s := searching(t)

	s.Submit(tr, "cat", false)
	// Wait for the answer to be waiting rather than for a length of time. What this
	// test is about is the second query arriving while the first answer is sitting in
	// the channel unread, and the channel itself says when that is true.
	waitForUnreadResult(t, s)
	s.Submit(tr, "mat", false)

	if got := found(t, s, "mat"); len(got.Matches) != 1 {
		t.Errorf("found %d matches of the newer query", len(got.Matches))
	}
}

func TestSearchAcrossARowWithNothingOnIt(t *testing.T) {
	// A match that spans a break but none of the row between, which is what a blank
	// row inside one logical line comes to.
	tr := transcriptOf(
		text.Row{Text: "ab"},
		text.Row{Text: "", Joined: true},
		text.Row{Text: "cd", Joined: true},
	)
	s := searching(t)
	s.Submit(tr, "abcd", false)

	r := found(t, s, "abcd")
	if len(r.Matches) != 1 {
		t.Fatalf("found %d matches, want 1", len(r.Matches))
	}
	if got := len(r.Matches[0].Spans); got != 3 {
		t.Errorf("the match covers %d rows, want 3: %+v", got, r.Matches[0].Spans)
	}
}

func TestSearchForALineBreakItself(t *testing.T) {
	// A pattern can match the separator, which is not on any row. It has to come back
	// as something rather than as an index nobody can use.
	tr := transcriptOf(plainRows("first", "second")...)
	s := searching(t)
	s.Submit(tr, `t
s`, true)
	if got := found(t, s, `t
s`); got.Err != nil {
		t.Errorf("err = %v", got.Err)
	}
}

// counting is a block that records how often its rows were asked for.
type counting struct {
	rows  []text.Row
	calls int
}

func (c *counting) Measure(int) int { return len(c.rows) }
func (c *counting) Draw(grid.View)  {}

func (c *counting) Rows(int) []text.Row {
	c.calls++
	return c.rows
}

// TestSearchFindsAMatchInsideWhatTheBreakAte. A wrap can swallow several spaces, and
// a query that matches inside them is a match the text really contains — it has to be
// reported on the row where it becomes visible rather than dropped.
func TestSearchFindsAMatchInsideWhatTheBreakAte(t *testing.T) {
	tr := transcriptOf(
		text.Row{Text: "before"},
		text.Row{Text: "after", Joined: true, Gap: "    "},
	)
	s := searching(t)
	s.Submit(tr, "   a", false)

	r := found(t, s, "   a")
	if len(r.Matches) != 1 {
		t.Fatalf("found %d matches, want 1: %+v", len(r.Matches), r.Matches)
	}
	if got := r.Matches[0].Row; got != 1 {
		t.Errorf("the match is on row %d, want 1 — where it becomes visible", got)
	}
}
