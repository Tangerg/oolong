package base

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/text"
)

var benchmarkSearchResult Result

func BenchmarkSearchDenseMatches(b *testing.B) {
	for _, rows := range []int{100, 10_000} {
		b.Run(fmt.Sprintf("rows=%d", rows), func(b *testing.B) {
			corpus := make([]text.Row, rows)
			for i := range corpus {
				corpus[i].Text = strings.Repeat("cat ", 9) + "cat"
			}
			config := ScanConfig{Query: "cat", Rows: corpus, MaxMatches: rows * 10}
			b.ReportAllocs()
			for b.Loop() {
				benchmarkSearchResult, _ = Scan(b.Context(), config)
			}
		})
	}
}

func BenchmarkSearchNoMatch(b *testing.B) {
	corpus := []text.Row{{Text: strings.Repeat("nothing here ", 10_000)}}
	config := ScanConfig{Query: "absent", Rows: corpus}
	b.ReportAllocs()
	for b.Loop() {
		benchmarkSearchResult, _ = Scan(b.Context(), config)
	}
}

func BenchmarkScanLongUnicodeRow(b *testing.B) {
	config := ScanConfig{Query: "猫", Rows: []text.Row{{Text: strings.Repeat("猫犬", 2000)}}}
	for b.Loop() {
		result, err := Scan(b.Context(), config)
		if err != nil || len(result.Matches) != 2000 {
			b.Fatal(err, len(result.Matches))
		}
	}
}
