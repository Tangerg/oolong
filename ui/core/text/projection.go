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

type displayCluster struct {
	text     string
	from, to int
}

type preparedLine struct {
	source  Line
	display displayProjection
}

func (l Line) prepare() preparedLine {
	return preparedLine{source: l, display: projectDisplay(l.String())}
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

func (p displayProjection) clusters() iter.Seq[displayCluster] {
	return func(yield func(displayCluster) bool) {
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
			cluster := displayCluster{
				text: clusters.Value(),
				from: sourceByte(clusters.Start()),
				to:   sourceByte(clusters.End()-1) + 1,
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

func (p displayProjection) match(cluster displayCluster, offsets []int) (bool, []int) {
	matched := false
	for len(offsets) > 0 && offsets[0] < cluster.to {
		if offsets[0] >= cluster.from && p.containsSourceByte(offsets[0]) {
			matched = true
		}
		offsets = offsets[1:]
	}
	return matched, offsets
}

func (p preparedLine) clusters() iter.Seq2[displayCluster, *Span] {
	return func(yield func(displayCluster, *Span) bool) {
		line := p.source
		spanIndex, spanEnd := 0, 0
		if len(line) > 0 {
			spanEnd = len(line[0].Text)
		}
		for cluster := range p.display.clusters() {
			for spanIndex+1 < len(line) && cluster.from >= spanEnd {
				spanIndex++
				spanEnd += len(line[spanIndex].Text)
			}
			if !yield(cluster, &line[spanIndex]) {
				return
			}
		}
	}
}
