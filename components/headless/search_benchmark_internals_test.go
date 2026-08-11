package headless

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/text"
)

var benchmarkSearchResult Result

func BenchmarkSearchDenseMatches(b *testing.B) {
	for _, rows := range []int{100, 10_000} {
		b.Run(fmt.Sprintf("rows=%d", rows), func(b *testing.B) {
			corpus := make([]text.Row, rows)
			for i := range corpus {
				corpus[i].Text = strings.Repeat("cat ", 9) + "cat"
			}
			s := &Search{generation: 1}
			j := job{query: "cat", corpus: corpus, generation: 1}
			b.ReportAllocs()
			for b.Loop() {
				benchmarkSearchResult, _ = s.scan(j)
			}
		})
	}
}

func BenchmarkSearchNoMatch(b *testing.B) {
	corpus := []text.Row{{Text: strings.Repeat("nothing here ", 10_000)}}
	s := &Search{generation: 1}
	j := job{query: "absent", corpus: corpus, generation: 1}
	b.ReportAllocs()
	for b.Loop() {
		benchmarkSearchResult, _ = s.scan(j)
	}
}
