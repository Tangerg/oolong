package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestSearchCarriesSnapshotRevisionAndLiveRowOffset(t *testing.T) {
	rows := []text.Row{{Text: "the needle"}}
	previous, err := base.Scan(t.Context(), base.ScanConfig{Rows: rows, Query: "needle", Revision: 7, StartRow: 99})
	if err != nil {
		t.Fatal(err)
	}
	next, err := base.Scan(t.Context(), base.ScanConfig{Rows: rows, Query: "needle", Revision: 8, StartRow: 100})
	if err != nil {
		t.Fatal(err)
	}
	if previous.Revision != 7 || next.Revision != 8 || previous.Matches[0].Row != 99 || next.Matches[0].Row != 100 {
		t.Fatal("scan lost caller-owned snapshot identity")
	}
	rows[0].Text = "replaced"
	if previous.Matches[0].Spans[0] != (base.Span{Col: 4, Width: 6}) {
		t.Fatal("returned result retained mutable input row storage")
	}
}
