package text

import (
	"iter"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/internal/grapheme"
)

// Filtering can join previously separated graphemes. The display is segmented
// after filtering; source runs retain the original bytes for appearance and
// selection without giving either projection its own interpretation of a cluster.
type displayProjection struct {
	source string
	text   string
	runs   []sourceRun
}

type sourceRun struct {
	displayStart, sourceStart int
}

// Cluster is one display atom and the half-open source byte interval it occupies.
// The interval can include filtered bytes that are absent from Text.
type Cluster struct {
	Text     string
	From, To int
}

func (l Line) sections() iter.Seq2[int, Line] {
	return func(yield func(int, Line) bool) {
		offset := 0
		for start := 0; start < len(l); {
			end := l.sectionEnd(start)
			section := l[start:end]
			if !yield(offset, section) {
				return
			}
			offset += section.ByteLen()
			start = end
		}
	}
}

func (l Line) sectionEnd(start int) int {
	end := start + 1
	for end < len(l) && !l[end].BreakBefore {
		end++
	}
	return end
}

func projectDisplay(source string) displayProjection {
	p := displayProjection{source: source, text: source}
	if utf8.ValidString(source) && !strings.ContainsFunc(source, droppedRune) {
		return p
	}
	var value strings.Builder
	value.Grow(len(source))
	var filter displayFilter
	from := 0
	for at, cluster := range Clusters(source) {
		if !filter.hides(cluster) {
			continue
		}
		if from < at {
			p.runs = append(p.runs, sourceRun{displayStart: value.Len(), sourceStart: from})
			value.WriteString(source[from:at])
		}
		from = at + len(cluster)
	}
	if from < len(source) {
		p.runs = append(p.runs, sourceRun{displayStart: value.Len(), sourceStart: from})
		value.WriteString(source[from:])
	}
	p.text = value.String()
	return p
}

func (p displayProjection) clusters() iter.Seq[Cluster] {
	return func(yield func(Cluster) bool) {
		run := 0
		sourceByte := func(at int) int {
			if len(p.runs) == 0 {
				return at
			}
			for run+1 < len(p.runs) && p.runs[run+1].displayStart <= at {
				run++
			}
			return p.runs[run].sourceStart + at - p.runs[run].displayStart
		}
		clusters := grapheme.New(p.text)
		for clusters.Next() {
			cluster := Cluster{
				Text: clusters.Value(),
				From: sourceByte(clusters.Start()),
				To:   sourceByte(clusters.End()-1) + 1,
			}
			if !yield(cluster) {
				return
			}
		}
	}
}

func (p displayProjection) containsSourceByte(at int) bool {
	if at < 0 || at >= len(p.source) {
		return false
	}
	if len(p.runs) == 0 {
		return p.text != ""
	}
	index := sort.Search(len(p.runs), func(i int) bool { return p.runs[i].sourceStart > at }) - 1
	if index < 0 {
		return false
	}
	end := len(p.text)
	if index+1 < len(p.runs) {
		end = p.runs[index+1].displayStart
	}
	return at-p.runs[index].sourceStart < end-p.runs[index].displayStart
}

func (p displayProjection) match(cluster Cluster, offsets []int) (bool, []int) {
	matched := false
	for len(offsets) > 0 && offsets[0] < cluster.To {
		if offsets[0] >= cluster.From && p.containsSourceByte(offsets[0]) {
			matched = true
		}
		offsets = offsets[1:]
	}
	return matched, offsets
}

func (l Line) clusters() iter.Seq2[Cluster, *Span] {
	return func(yield func(Cluster, *Span) bool) {
		offset := 0
		for start := 0; start < len(l); {
			end := l.sectionEnd(start)
			section := l[start:end]
			display := projectDisplay(section.String())
			appearance := spanCursor{spans: section, end: len(section[0].Text)}
			for cluster := range display.clusters() {
				span := appearance.at(cluster.From)
				cluster.From += offset
				cluster.To += offset
				if !yield(cluster, span) {
					return
				}
			}
			offset += section.ByteLen()
			start = end
		}
	}
}

func styledClusters(line Line, display displayProjection) iter.Seq2[Cluster, *Span] {
	return func(yield func(Cluster, *Span) bool) {
		appearance := spanCursor{spans: line, end: len(line[0].Text)}
		for cluster := range display.clusters() {
			if !yield(cluster, appearance.at(cluster.From)) {
				return
			}
		}
	}
}

type spanCursor struct {
	spans      Line
	index, end int
}

func (c *spanCursor) at(sourceByte int) *Span {
	for c.index+1 < len(c.spans) && sourceByte >= c.end {
		c.index++
		c.end += len(c.spans[c.index].Text)
	}
	return &c.spans[c.index]
}
