package text

import "iter"

// Clusters retains source byte intervals while respecting independent span
// boundaries. Filtering can join source fragments, so To need not equal
// From plus the byte length of Text.
func (l Line) Clusters() iter.Seq[Cluster] {
	return func(yield func(Cluster) bool) {
		for cluster := range l.clusters() {
			if !yield(cluster) {
				return
			}
		}
	}
}

func (l Line) ColumnOf(at int) int {
	at = min(max(at, 0), l.ByteLen())
	column := 0
	for cluster := range l.Clusters() {
		if cluster.From >= at {
			break
		}
		column = advanceCluster(cluster.Text, column)
	}
	return column
}

func (l Line) OffsetAt(column int) int {
	if column <= 0 {
		return 0
	}
	used := 0
	for cluster := range l.Clusters() {
		next := advanceCluster(cluster.Text, used)
		if next > column {
			return cluster.From
		}
		used = next
	}
	return l.ByteLen()
}

func (l Line) NextCluster(at int) int {
	if at < 0 {
		return 0
	}
	for cluster := range l.Clusters() {
		if at < cluster.To {
			return cluster.To
		}
	}
	return l.ByteLen()
}

func (l Line) PrevCluster(at int) int {
	if at <= 0 {
		return 0
	}
	previous := 0
	for cluster := range l.Clusters() {
		if at <= cluster.From {
			return previous
		}
		if at <= cluster.To {
			return cluster.From
		}
		previous = cluster.From
	}
	return previous
}

// Slice keeps span appearance and independent boundaries at the requested byte
// interval. It panics for invalid indices, following ordinary Go slicing, and
// does not round a caller's range to grapheme boundaries.
func (l Line) Slice(from, to int) Line {
	if from < 0 || to < from || to > l.ByteLen() {
		panic("text: line slice out of range")
	}
	if from == to {
		return nil
	}
	var out Line
	offset := 0
	for _, span := range l {
		end := offset + len(span.Text)
		if offset >= to {
			break
		}
		if end > from || span.Text == "" && span.BreakBefore && offset >= from {
			start := max(from-offset, 0)
			span.Text = span.Text[start:min(to-offset, len(span.Text))]
			if start > 0 {
				span.BreakBefore = false
			}
			out = append(out, span)
		}
		offset = end
	}
	return out
}

func (l Line) WordAt(at int) (start, end int, ok bool) {
	if at < 0 || at >= l.ByteLen() {
		return 0, 0, false
	}
	type wordSpan struct {
		from, to int
		class    Class
	}
	var spans []wordSpan
	hit := -1
	for cluster := range l.Clusters() {
		if at >= cluster.From && at < cluster.To {
			hit = len(spans)
		}
		spans = append(spans, wordSpan{from: cluster.From, to: cluster.To, class: ClassOf(cluster.Text)})
	}
	if hit < 0 {
		return 0, 0, false
	}
	switch class := spans[hit].class; class {
	case Space:
		return 0, 0, false
	case Punct:
		return spans[hit].from, spans[hit].to, true
	default:
		first, last := hit, hit
		for first > 0 && spans[first-1].class == class {
			first--
		}
		for last+1 < len(spans) && spans[last+1].class == class {
			last++
		}
		return spans[first].from, spans[last].to, true
	}
}
