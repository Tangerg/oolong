package grid_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	styledtext "github.com/Tangerg/oolong/ui/core/text"
)

const (
	rowAppearanceStart = "\x1b]8;;\x1b\\\x1b[0m"
	rowAppearanceEnd   = "\x1b[0m"
)

func TestEncodeRowOwnsItsAppearance(t *testing.T) {
	const inherited = "\x1b[1;38;2;250;20;30m\x1b]8;;https://previous.example/\x1b\\"
	const positioned = "\x1b[1;1H"
	for _, test := range []struct {
		name  string
		style grid.Style
	}{
		{name: "default"},
		{name: "explicit", style: grid.Style{Attr: grid.Underline}},
	} {
		t.Run(test.name, func(t *testing.T) {
			surface := grid.NewSurface(1, 1)
			surface.View().Text(0, 0, "X", test.style)
			wire := grid.EncodeRow(surface.Row(0), grid.TrueColor)
			lines := styledtext.Decode(inherited+positioned+wire+"\nY\n", grid.Style{})
			if len(lines) != 2 || len(lines[0]) != 1 || len(lines[1]) != 1 {
				t.Fatalf("unexpected decoded rows: %#v", lines)
			}
			row, next := lines[0][0], lines[1][0]
			if row.Style != test.style || row.Link != "" {
				t.Errorf("encoded cell inherited earlier wire appearance: style=%+v link=%q; want style=%+v and no link", row.Style, row.Link, test.style)
			}
			if next.Style != (grid.Style{}) || next.Link != "" {
				t.Errorf("EncodeRow did not leave its documented reset style/closed-link state: style=%+v link=%q", next.Style, next.Link)
			}
		})
	}
}

func TestEmptyEncodedRowStillEndsThePreviousAppearance(t *testing.T) {
	const inherited = "\x1b[1m\x1b]8;;https://previous.example/\x1b\\"
	for _, row := range [][]grid.Cell{nil, make([]grid.Cell, 3)} {
		wire := grid.EncodeRow(row, grid.TrueColor)
		lines := styledtext.Decode(inherited+wire+"Y", grid.Style{})
		if len(lines) != 1 || len(lines[0]) != 1 || lines[0][0].Style != (grid.Style{}) || lines[0][0].Link != "" {
			t.Fatalf("empty row left the previous appearance active: wire=%q, lines=%+v", wire, lines)
		}
	}
}
