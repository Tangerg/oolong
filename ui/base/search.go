package base

import (
	"context"
	"errors"
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

// Scan is pure work over an immutable row snapshot. It owns no goroutine or
// delivery queue; the caller chooses task lifetime and checks Revision on delivery.
// Zero MaxMatches allows 10,000 matches. Exceeding it returns ErrSearchLimit,
// never a silently incomplete set which could masquerade as the whole document.
func Scan(ctx context.Context, config ScanConfig) (Result, error) {
	result := Result{Query: config.Query, Revision: config.Revision}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if config.Query == "" {
		return result, nil
	}
	limit := config.MaxMatches
	if limit == 0 {
		limit = 10000
	}
	if limit < 0 {
		return result, errors.New("base: search limit must not be negative")
	}
	re, err := compile(config.Query, config.Regex)
	if err != nil {
		return result, err
	}
	joined, starts := join(config.Rows)
	if err := ctx.Err(); err != nil {
		return result, err
	}
	count := limit
	if count < math.MaxInt {
		count++
	}
	locations := re.FindAllStringIndex(joined, count)
	if len(locations) == 0 {
		return result, nil
	}
	if len(locations) > limit {
		return result, ErrSearchLimit
	}
	spans := make([]Span, 0, len(locations))
	result.Matches = make([]Match, 0, len(locations))
	for _, location := range locations {
		if err := ctx.Err(); err != nil {
			return Result{Query: config.Query, Revision: config.Revision}, err
		}
		first := len(spans)
		row, next, ok := spread(spans, location[0], location[1], config.Rows, starts)
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
