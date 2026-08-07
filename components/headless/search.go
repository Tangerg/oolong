package headless

import (
	"regexp"
	"strings"
	"sync"

	"github.com/Tangerg/oolong/core/text"
)

// Span is a run of columns on one row.
type Span struct{ Col, Width int }

// Match is one occurrence of a query.
//
// Spans has one entry per row the match covers, starting at Row, because a match can
// cross a break the width made: the query was written as one line and the window
// wrapped it, and a search that only looked at rows would not find it at all.
type Match struct {
	Row   int
	Spans []Span
}

// Result is a finished scan.
type Result struct {
	// Query is what was searched for, so a caller can tell an answer to the question
	// it is still asking from an answer to one it has moved on from.
	Query string
	// Matches are in the order they appear.
	Matches []Match
	// Err is set when the query was a pattern that would not compile. It is a
	// result rather than a refusal at submission time, because a user typing a
	// pattern spends most of the typing with an unfinished one and should not be
	// interrupted about it.
	Err error
}

// Search scans a transcript's text off the interface's goroutine.
//
// # Why it is not simply a function
//
// A search box searches on every keystroke, and the answer to a query three letters
// old is worth nothing. So the newest query wins: a scan in progress is abandoned at
// the next row boundary when a later one arrives, and only the last of a burst
// finishes. Anything that ran scans to completion would spend a long transcript's
// worth of work per keystroke to produce answers nobody would read.
//
// # What crosses the goroutine boundary
//
// Strings, and nothing else. The transcript belongs to the goroutine that draws, and
// this never touches it: [Search.Submit] takes the live rows on the caller's goroutine,
// where reading them is safe, and hands over what it took. The rows are already
// strings, so taking them copies headers rather than text. A superseded job releases
// that snapshot instead of turning search into a second owner of committed history.
//
// Results arrive on [Search.Results]. A caller reads that from a goroutine of its own
// and posts what it gets back to the event owner. This package does not prescribe the
// dispatcher used to cross that boundary. Each submission owns its row snapshot until
// it finishes or is superseded; Search does not retain an older transcript generation
// after the work that needs it is gone.
type Search struct {
	jobs    chan job
	results chan Result
	stop    chan struct{}
	once    sync.Once
}

// job is one scan, as the worker sees it.
type job struct {
	query  string
	regex  bool
	start  int
	corpus []Row
}

// NewSearch starts a scanner. Close it when the interface it serves is done.
func NewSearch() *Search {
	s := &Search{
		// A job channel of one, replaced rather than queued: two waiting scans mean
		// the first is already stale.
		jobs:    make(chan job, 1),
		results: make(chan Result, 1),
		stop:    make(chan struct{}),
	}
	go s.run()
	return s
}

// Results is where finished scans arrive.
func (s *Search) Results() <-chan Result { return s.results }

// Close stops the scanner. It is safe to call more than once.
func (s *Search) Close() { s.once.Do(func() { close(s.stop) }) }

// Submit schedules a scan of t for query, replacing any scan not yet finished.
//
// An empty query schedules nothing and delivers nothing: it is what a search box
// looks like before anybody has typed, and answering it with every row in the session
// is not what was asked.
func (s *Search) Submit(t *Transcript, query string, regex bool) {
	if t == nil || query == "" {
		return
	}
	start := t.StartRow()
	next := job{
		query:  query,
		regex:  regex,
		start:  start,
		corpus: t.Rows(start, t.Height()),
	}
	for {
		select {
		case s.jobs <- next:
			return
		case <-s.jobs:
			// Displaced a scan that had not started. Nothing is lost that anyone
			// still wants.
		case <-s.stop:
			return
		}
	}
}

// run is the worker.
func (s *Search) run() {
	for {
		select {
		case j := <-s.jobs:
			result, superseded := s.scan(j)
			if superseded {
				continue
			}
			s.deliver(result)
		case <-s.stop:
			return
		}
	}
}

// deliver hands a result over, replacing one nobody has read yet.
func (s *Search) deliver(r Result) {
	for {
		select {
		case s.results <- r:
			return
		case <-s.results:
			// The unread result answered an older query.
		case <-s.stop:
			return
		}
	}
}

// scan does the work, reporting whether it gave up because a newer query arrived.
func (s *Search) scan(j job) (Result, bool) {
	re, err := compile(j.query, j.regex)
	if err != nil {
		return Result{Query: j.query, Err: err}, false
	}

	joined, starts := join(j.corpus)
	result := Result{Query: j.query}
	for _, loc := range re.FindAllStringIndex(joined, -1) {
		select {
		case <-s.stop:
			return Result{}, true
		default:
		}
		if superseded(s.jobs) {
			return Result{}, true
		}
		if m, ok := spread(loc[0], loc[1], j.corpus, starts); ok {
			m.Row += j.start
			result.Matches = append(result.Matches, m)
		}
	}
	return result, false
}

// superseded reports whether a newer job is already waiting.
func superseded(jobs <-chan job) bool {
	return len(jobs) > 0
}

// compile turns a query into the pattern that finds it.
//
// A literal query goes through the same matcher as a pattern, quoted. That is not
// laziness: matching case-insensitively by lowercasing both sides gives byte offsets
// into a string nobody has — case folding changes lengths, so "İ" lowercases to two
// runes and every offset after it is wrong. Regexp folds without moving anything, and
// one matcher cannot disagree with itself about what a match is.
func compile(query string, regex bool) (*regexp.Regexp, error) {
	if !regex {
		query = regexp.QuoteMeta(query)
	}
	return regexp.Compile("(?i)" + query)
}

// join runs the rows together the way they were written and records where each one
// starts, so a match found in the whole can be put back on the rows it covers.
//
// The rows are joined as a copy would join them — see [Row] — because a query is
// written the way the text was written and not the way the window happened to break
// it. Searching row by row would fail to find a phrase the wrap split, which is the
// one a user is most likely to be looking for: a long one.
func join(rows []Row) (joined string, starts []int) {
	var b strings.Builder
	starts = make([]int, len(rows))
	for i, r := range rows {
		if i > 0 {
			b.WriteString(r.Separator())
		}
		starts[i] = b.Len()
		b.WriteString(r.Text)
	}
	return b.String(), starts
}

// spread turns a byte range of the joined text into the columns it covers on each row.
func spread(from, to int, rows []Row, starts []int) (Match, bool) {
	first := rowAt(starts, rows, from)
	if first < 0 {
		return Match{}, false
	}
	m := Match{Row: first}
	for i := first; i < len(rows); i++ {
		start, end := starts[i], starts[i]+len(rows[i].Text)
		if start >= to {
			break
		}
		lo, hi := max(from, start)-start, min(to, end)-start
		if lo > hi {
			// The match spans this row's break but none of its text, which a row of
			// nothing between two others produces.
			lo, hi = 0, 0
		}
		col := text.ColumnOf(rows[i].Text, lo)
		m.Spans = append(m.Spans, Span{Col: col, Width: text.ColumnOf(rows[i].Text, hi) - col})
	}
	if len(m.Spans) == 0 {
		return Match{}, false
	}
	return m, true
}

// rowAt is the row a byte offset of the joined text falls in, or -1 when there are no
// rows at all.
//
// An offset can land between two rows, in what the break consumed: a query of two
// spaces finds them in a gap that was three. The match belongs to the row where it
// becomes visible, which is the next one — attributing it to the row before would
// highlight columns the match is not on, and refusing it would drop a match the text
// really contains.
func rowAt(starts []int, rows []Row, offset int) int {
	lo, hi := 0, len(starts)-1
	if hi < 0 {
		return -1
	}
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if starts[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if offset > starts[lo]+len(rows[lo].Text) && lo+1 < len(rows) {
		return lo + 1
	}
	return lo
}
