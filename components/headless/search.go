package headless

import (
	"regexp"
	"strings"
	"sync"

	"github.com/Tangerg/oolong/core/layout"
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
	// Query is the searched text. It does not identify a submission: identical text
	// can describe different corpora or modes. Use Current before applying a result.
	Query string
	// Matches is the complete geometry in document order. It is deliberately not a
	// viewport-sized window: a result must support stepping to any occurrence and
	// highlighting a different viewport after scrolling without retaining or
	// rescanning the transcript snapshot that produced it. Its retained cost is
	// therefore proportional to occurrences, while the source rows are released when
	// the scan finishes.
	Matches []Match
	// Err is set when the query was a pattern that would not compile. It is a
	// result rather than a refusal at submission time, because a user typing a
	// pattern spends most of the typing with an unfinished one and should not be
	// interrupted about it.
	Err     error
	search  *Search
	request *byte
}

// Current reports whether this result belongs to its Search's latest submission.
// It is a pure, concurrency-safe receipt, invalidated by Submit or Close even when
// the new query text is identical. The zero Result is never current. Check on the
// transcript owner immediately before applying the result. Serialize Submit, Close
// and application there so no lifetime transition intervenes after the check.
func (r Result) Current() bool {
	return r.search != nil && r.request != nil && !r.search.superseded(r.request)
}

// Search scans a transcript's text off the interface's goroutine.
//
// A search box searches on every keystroke and the answer to a query three letters
// old is worth nothing, so the newest query wins: a waiting scan and an unread result
// are replaced, and a running scan is discarded at its next cancellation boundary. The
// standard regexp matcher is not interruptible, so one in-progress match pass
// completes first rather than this package keeping a matcher of its own.
//
// Strings are all that cross the goroutine boundary. The transcript belongs to the
// goroutine that draws, and [Search.Submit] takes the live rows there, where reading
// them is safe; the rows are already strings, so taking them copies headers rather
// than text, and a superseded job releases its snapshot rather than becoming a second
// owner of committed history.
//
// Results arrive on [Search.Results], which a caller reads from a goroutine of its own
// and posts back to the event owner. That owner checks Result.Current before applying
// it; Query alone cannot distinguish a repeated submission. A Search must not be
// copied after construction; its worker, mailboxes and cancellation state are one owner.
type Search struct {
	mu sync.Mutex
	// next is the one scan waiting to start. A newer submission replaces it and
	// request invalidates running scans and results, including ones already received.
	next    job
	request *byte
	closed  bool

	wake    chan struct{}
	results chan Result
	stop    chan struct{}
	done    chan struct{}
}

// job is one scan, as the worker sees it.
type job struct {
	query   string
	regex   bool
	start   int
	corpus  []text.Row
	request *byte
}

// NewSearch starts a scanner. Close it when the interface it serves is done.
func NewSearch() *Search {
	s := &Search{
		// Wake carries no state. The mutex-protected mailbox above is the state, and
		// several replacements before the worker wakes still need only one signal.
		wake:    make(chan struct{}, 1),
		results: make(chan Result, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go s.run()
	return s
}

// Results is where finished scans arrive. It closes after [Search.Close] stops the
// scanner, so a consumer may range over it without another lifetime signal. The nil
// and zero Search return an already-closed stream.
func (s *Search) Results() <-chan Result {
	if s == nil || s.results == nil {
		return stoppedResults
	}
	return s.results
}

// Close stops the scanner and waits for its worker to release the current corpus.
// It is safe to call more than once. When it returns, Results is closed and Search
// retains no submitted rows or unread result.
func (s *Search) Close() {
	if s == nil || s.stop == nil || s.done == nil {
		return
	}
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.request = nil
		s.next = job{}
		// An unread answer has no owner after Close. Clear it before the sole producer
		// closes the channel so its retained matches are released immediately.
		select {
		case <-s.results:
		default:
		}
		close(s.stop)
	}
	s.mu.Unlock()
	<-s.done
}

// Submit schedules a scan of t for query, replacing any scan not yet finished.
//
// An empty query cancels an older scan and delivers nothing: it is what a search box
// looks like after its text was cleared. A nil transcript also cancels the old scan.
// Answering either with every row in the session or with the previous query is not
// what was asked.
func (s *Search) Submit(t *Transcript, query string, regex bool) {
	if s == nil || s.wake == nil {
		return
	}
	if query == "" || t == nil {
		s.offer(job{})
		return
	}
	if !s.accepting() {
		return
	}

	start := t.StartRow()
	s.offer(job{
		query:  strings.Clone(query),
		regex:  regex,
		start:  start,
		corpus: t.Rows(start, t.Height()),
	})
}

func (s *Search) accepting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

// offer replaces the question waiting to be answered. The empty job is a
// cancellation: it advances the request and wakes a running scan, but the worker
// has nothing new to deliver after abandoning it.
func (s *Search) offer(next job) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.request = new(byte)
	next.request = s.request
	s.next = next
	// A result nobody read answered an older request. Removing it here also makes
	// an empty submission a complete cancellation rather than merely no new work.
	select {
	case <-s.results:
	default:
	}
	s.mu.Unlock()

	select {
	case s.wake <- struct{}{}:
	default:
	}
}

var stoppedResults = func() <-chan Result {
	done := make(chan Result)
	close(done)
	return done
}()

func (s *Search) run() {
	defer close(s.done)
	defer close(s.results)
	for {
		select {
		case <-s.wake:
			j, ok := s.take()
			if !ok || j.query == "" {
				continue
			}
			result, superseded := s.scan(j)
			if superseded {
				continue
			}
			s.deliver(j.request, result)
		case <-s.stop:
			return
		}
	}
}

func (s *Search) take() (job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next.request == nil {
		return job{}, false
	}
	next := s.next
	s.next = job{}
	return next, true
}

func (s *Search) deliver(request *byte, result Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || request != s.request {
		return
	}
	select {
	case <-s.results:
	default:
	}
	// Results has room for one and was just emptied while the sole producer is
	// holding the mailbox lock, so this send cannot wait.
	result.search, result.request = s, request
	s.results <- result
}

// scan does the work, reporting whether it gave up because a newer query arrived.
func (s *Search) scan(j job) (Result, bool) {
	re, err := compile(j.query, j.regex)
	if err != nil {
		return Result{Query: j.query, Err: err}, false
	}
	if s.superseded(j.request) {
		return Result{}, true
	}

	joined, starts := join(j.corpus)
	// Joining and matching are separate linear passes. A close or newer query that
	// arrived while the snapshot was being joined must not also pay for a regexp pass
	// whose answer can no longer be observed. The matcher itself cannot be interrupted,
	// so this is the last cancellation boundary before entering it.
	if s.superseded(j.request) {
		return Result{}, true
	}
	locations := re.FindAllStringIndex(joined, -1)
	result := Result{Query: j.query}
	var spans []Span
	if len(locations) > 0 {
		// Every location produces at most one Match and normally one Span. A match
		// crossing rows grows spans from this estimate without making the common case
		// pay one allocation per occurrence.
		result.Matches = make([]Match, 0, len(locations))
		spans = make([]Span, 0, len(locations))
	}
	for _, loc := range locations {
		if s.superseded(j.request) {
			return Result{}, true
		}
		first := len(spans)
		row, next, ok := spread(spans, loc[0], loc[1], j.corpus, starts)
		if ok {
			spans = next
			last := len(spans)
			result.Matches = append(result.Matches, Match{
				Row:   layout.Sum(row, j.start),
				Spans: spans[first:last:last],
			})
		}
	}
	return result, false
}

func (s *Search) superseded(request *byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed || request != s.request
}

// Regexp case folding preserves source offsets; lowercasing can change UTF-8 lengths and invalidate matches.
func compile(query string, regex bool) (*regexp.Regexp, error) {
	if !regex {
		query = regexp.QuoteMeta(query)
	}
	return regexp.Compile("(?i)" + query)
}

// Join wrapped rows as originally written so a width change cannot split searchable phrases.
func join(rows []text.Row) (joined string, starts []int) {
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

func spread(dst []Span, from, to int, rows []text.Row, starts []int) (int, []Span, bool) {
	first := rowAt(starts, rows, from)
	if first < 0 {
		return 0, dst, false
	}
	before := len(dst)
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
		startCol := text.ColumnOf(rows[i].Text, lo)
		dst = append(dst, Span{
			Col: layout.Sum(rows[i].Offset, startCol), Width: text.ColumnOf(rows[i].Text, hi) - startCol,
		})
	}
	if len(dst) == before {
		// A real match can live entirely in whitespace consumed by wrapping.
		dst = append(dst, Span{Col: rows[first].Offset})
	}
	return first, dst, true
}

// A match in consumed wrap whitespace belongs to the next visible row; refusing it loses real source matches.
func rowAt(starts []int, rows []text.Row, offset int) int {
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
