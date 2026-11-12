package base

import (
	"context"
	"errors"
	"iter"
	"math"
	"regexp"
	"strings"

	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

type (
	// Span identifies the matched terminal cells within one visual row.
	Span struct{ Col, Width int }
	// Match identifies one query occurrence and its per-row visual spans.
	Match struct {
		Row   int
		Spans []Span
	}
)

// Result is an immutable search result tagged with the scanned content revision.
type Result struct {
	Query    string
	Revision uint64
	Matches  []Match
}

// ScanConfig supplies an immutable row snapshot and its content revision.
// Callers running Scan in a worker must keep Rows immutable for the call.
type ScanConfig struct {
	Query      string
	Regex      bool
	Rows       []text.Row
	StartRow   int
	Revision   uint64
	MaxMatches int
}

// ErrSearchLimit reports that a complete result would exceed the configured bound.
var ErrSearchLimit = errors.New("base: search match limit exceeded")

const defaultSearchMatches = 10_000

// Scan is pure work over an immutable row snapshot. It owns no goroutine or
// delivery queue; the caller chooses task lifetime and checks Revision on delivery.
// Zero MaxMatches uses the default budget. Exceeding it returns ErrSearchLimit,
// never a silently incomplete set which could masquerade as the whole document.
func Scan(ctx context.Context, config ScanConfig) (Result, error) {
	result := Result{Query: config.Query, Revision: config.Revision}
	if config.MaxMatches < 0 {
		return result, errors.New("base: search limit must not be negative")
	}
	if err := context.Cause(ctx); err != nil {
		return result, err
	}
	if config.Query == "" {
		return result, nil
	}
	limit := config.MaxMatches
	if limit == 0 {
		limit = defaultSearchMatches
	}
	re, err := compile(config.Query, config.Regex)
	if err != nil {
		return result, err
	}
	corpus := newSearchCorpus(config.Rows)
	if err := context.Cause(ctx); err != nil {
		return result, err
	}
	count := limit
	if count < math.MaxInt {
		count++
	}
	locations := re.FindAllStringIndex(corpus.text, count)
	if len(locations) == 0 {
		return result, nil
	}
	if len(locations) > limit {
		return result, ErrSearchLimit
	}
	spans := make([]Span, 0, len(locations))
	result.Matches = make([]Match, 0, len(locations))
	columns := matchColumns{row: -1}
	defer columns.close()
	for _, location := range locations {
		if err := context.Cause(ctx); err != nil {
			return Result{Query: config.Query, Revision: config.Revision}, err
		}
		first := len(spans)
		row, next, ok := columns.spread(spans, location[0], location[1], corpus)
		if ok {
			spans = next
			last := len(spans)
			result.Matches = append(result.Matches, Match{Row: layout.Sum(row, config.StartRow), Spans: spans[first:last:last]})
		}
	}
	return result, nil
}

// Regexp case folding preserves source offsets; lowercasing can change UTF-8 lengths and invalidate matches.
func compile(query string, regex bool) (*regexp.Regexp, error) {
	if !regex {
		query = regexp.QuoteMeta(query)
	}
	return regexp.Compile("(?i)" + query)
}

type searchCorpus struct {
	rows   []text.Row
	starts []int
	text   string
}

// Regex and geometry must see the same admitted text. A display cluster's source
// interval can cross removed controls, which cannot become searchable content.
func newSearchCorpus(source []text.Row) searchCorpus {
	corpus := searchCorpus{rows: make([]text.Row, len(source)), starts: make([]int, len(source))}
	var b strings.Builder
	for i, row := range source {
		row.Content = row.Content.Display()
		row.Gap = (text.Line{{Text: row.Gap}}).Display().String()
		corpus.rows[i] = row
		if i > 0 {
			b.WriteString(row.Separator())
		}
		corpus.starts[i] = b.Len()
		b.WriteString(row.Content.String())
	}
	corpus.text = b.String()
	return corpus
}

// Regex locations arrive in byte order. Retain the last grapheme so distinct
// rune matches in the same displayed cell can each project its complete extent.
type matchColumns struct {
	row         int
	start, end  int
	left, right int
	next        func() (text.Cluster, bool)
	stop        func()
}

func (c *matchColumns) close() {
	if c.stop != nil {
		c.stop()
		c.stop = nil
	}
}

func (c *matchColumns) column(row int, value text.Line, offset int, roundUp bool) int {
	if row != c.row {
		c.close()
		next, stop := iter.Pull(value.Clusters())
		*c = matchColumns{row: row, next: next, stop: stop}
	}
	if offset > c.end {
		for {
			cluster, ok := c.next()
			if !ok {
				return c.right
			}
			c.start, c.end = cluster.From, cluster.To
			c.left = c.right
			c.right = text.Advance(cluster.Text, c.left)
			if offset <= c.end {
				break
			}
		}
	}
	if offset > c.start && (offset == c.end || roundUp) {
		return c.right
	}
	return c.left
}

func (c *matchColumns) spread(dst []Span, from, to int, corpus searchCorpus) (int, []Span, bool) {
	first := corpus.rowAt(from)
	if first < 0 {
		return 0, dst, false
	}
	rows, starts := corpus.rows, corpus.starts
	before := len(dst)
	for i := first; i < len(rows); i++ {
		start, end := starts[i], corpus.rowEnd(i)
		if start >= to {
			break
		}
		lo, hi := max(from, start)-start, min(to, end)-start
		if lo > hi {
			// The match spans this row's break but none of its text, which a row of
			// nothing between two others produces.
			lo, hi = 0, 0
		}
		startCol := c.column(i, rows[i].Content, lo, lo == hi)
		endCol := c.column(i, rows[i].Content, hi, true)
		dst = append(dst, Span{Col: layout.Sum(rows[i].Offset, startCol), Width: endCol - startCol})
	}
	if len(dst) == before {
		// A real match can live entirely in whitespace consumed by wrapping.
		dst = append(dst, Span{Col: rows[first].Offset})
	}
	return first, dst, true
}

// A match in consumed wrap whitespace belongs to the next visible row; refusing it loses real source matches.
func (c searchCorpus) rowAt(offset int) int {
	lo, hi := 0, len(c.starts)-1
	if hi < 0 {
		return -1
	}
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if c.starts[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo+1 < len(c.rows) && c.rows[lo+1].Joined && offset >= c.rowEnd(lo) {
		return lo + 1
	}
	return lo
}

func (c searchCorpus) rowEnd(row int) int {
	if row+1 < len(c.rows) {
		return c.starts[row+1] - len(c.rows[row+1].Separator())
	}
	return len(c.text)
}
