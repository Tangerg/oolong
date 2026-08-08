package ptytest_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/ptytest"
)

func TestScreenAppliesFullFramesAndCellDiffs(t *testing.T) {
	renderer := grid.NewScreen(8, 2)
	var output bytes.Buffer
	frame := renderer.Frame()
	frame.Text(0, 0, "alpha", grid.Style{})
	frame.Text(0, 1, "beta", grid.Style{})
	if err := renderer.Flush(&output); err != nil {
		t.Fatal(err)
	}

	shown, err := ptytest.NewScreen(ptytest.Size{Cols: 8, Rows: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := shown.Apply(output.Bytes()); err != nil {
		t.Fatal(err)
	}
	assertScreenRows(t, shown, "alpha   ", "beta    ")

	output.Reset()
	frame = renderer.Frame()
	frame.Text(0, 0, "alpha", grid.Style{})
	frame.Text(0, 1, "BETA", grid.Style{})
	if err := renderer.Flush(&output); err != nil {
		t.Fatal(err)
	}
	if err := shown.Apply(output.Bytes()); err != nil {
		t.Fatal(err)
	}
	assertScreenRows(t, shown, "alpha   ", "BETA    ")
}

func TestScreenAppliesInlineRepaintAndErasure(t *testing.T) {
	renderer := grid.NewInline(8, 3)
	shown, err := ptytest.NewScreen(ptytest.Size{Cols: 8, Rows: 3})
	if err != nil {
		t.Fatal(err)
	}

	flush := func(rows ...string) {
		t.Helper()
		frame := renderer.Frame()
		for y, row := range rows {
			frame.Text(0, y, row, grid.Style{})
		}
		var output bytes.Buffer
		if err := renderer.Flush(&output); err != nil {
			t.Fatal(err)
		}
		if err := shown.Apply(output.Bytes()); err != nil {
			t.Fatal(err)
		}
	}

	flush("one", "two")
	assertScreenRows(t, shown, "one     ", "two     ", "        ")
	flush("ONE")
	assertScreenRows(t, shown, "ONE     ", "        ", "        ")
}

func TestScreenCarriesSplitSequencesAndUTF8(t *testing.T) {
	shown, err := ptytest.NewScreen(ptytest.Size{Cols: 6, Rows: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range [][]byte{
		[]byte("\x1b[1;"),
		[]byte("2H\xe4\xb8"),
		[]byte("\xad"),
	} {
		if err := shown.Apply(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := shown.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := shown.At(1, 0); got != "中" {
		t.Fatalf("cell (1,0) = %q, want 中", got)
	}
	if got := shown.At(2, 0); got != "" {
		t.Fatalf("wide-character trailing cell = %q, want empty", got)
	}
}

func TestScreenIgnoresSessionTrafficThatPaintsNoCells(t *testing.T) {
	shown, err := ptytest.NewScreen(ptytest.Size{Cols: 6, Rows: 1})
	if err != nil {
		t.Fatal(err)
	}
	output := "\x1b[>31u\x1b[>0q\x1b[c\x1b[22;0t\x1b]10;?\x07text"
	if err := shown.Apply([]byte(output)); err != nil {
		t.Fatal(err)
	}
	if got := shown.Rows()[0]; got != "text  " {
		t.Fatalf("row = %q, want only painted text", got)
	}
}

func TestScreenAppliesRendererScrollOptimization(t *testing.T) {
	const width, height = 20, 6
	renderer := grid.NewScreen(width, height)
	shown, err := ptytest.NewScreen(ptytest.Size{Cols: width, Rows: height})
	if err != nil {
		t.Fatal(err)
	}
	rows := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"}

	flush := func(lines []string) string {
		t.Helper()
		frame := renderer.Frame()
		for y, line := range lines {
			frame.Text(0, y, line, grid.Style{})
		}
		var output bytes.Buffer
		if err := renderer.Flush(&output); err != nil {
			t.Fatal(err)
		}
		if err := shown.Apply(output.Bytes()); err != nil {
			t.Fatal(err)
		}
		return output.String()
	}

	flush(rows)
	second := append(append([]string(nil), rows[1:]...), "golf")
	if output := flush(second); !strings.Contains(output, "\x1b[1S") {
		t.Fatalf("renderer did not exercise the scroll path: %q", output)
	}
	for y, want := range second {
		if got := strings.TrimRight(shown.Rows()[y], " "); got != want {
			t.Fatalf("row %d = %q, want %q", y, got, want)
		}
	}
}

func TestScreenAppliesBothDirectionsInsideRendererScrollMargins(t *testing.T) {
	const width, height = 20, 6
	renderer := grid.NewScreen(width, height)
	shown, err := ptytest.NewScreen(ptytest.Size{Cols: width, Rows: height})
	if err != nil {
		t.Fatal(err)
	}

	flush := func(lines []string) string {
		t.Helper()
		frame := renderer.Frame()
		for y, line := range lines {
			frame.Text(0, y, line, grid.Style{})
		}
		var output bytes.Buffer
		if err := renderer.Flush(&output); err != nil {
			t.Fatal(err)
		}
		if err := shown.Apply(output.Bytes()); err != nil {
			t.Fatal(err)
		}
		return output.String()
	}

	before := []string{"fixed top", "alpha", "bravo", "charlie", "delta", "fixed bottom"}
	up := []string{"fixed top", "bravo", "charlie", "delta", "echo", "fixed bottom"}
	flush(before)
	if output := flush(up); !strings.Contains(output, "\x1b[2;5r") ||
		!strings.Contains(output, "\x1b[1S") || !strings.Contains(output, "\x1b[r") {
		t.Fatalf("upward region shift was not exercised: %q", output)
	}
	assertTrimmedScreenRows(t, shown, up)

	if output := flush(before); !strings.Contains(output, "\x1b[2;5r") ||
		!strings.Contains(output, "\x1b[1T") || !strings.Contains(output, "\x1b[r") {
		t.Fatalf("downward region shift was not exercised: %q", output)
	}
	assertTrimmedScreenRows(t, shown, before)
}

func TestScreenAppliesEveryDisplayErasureMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		seq  string
		want []string
	}{
		{
			name: "from cursor",
			seq:  "\x1b[2;3H\x1b[J",
			want: []string{"abcd", "ef", ""},
		},
		{
			name: "through cursor",
			seq:  "\x1b[2;3H\x1b[1J",
			want: []string{"", "   h", "ijkl"},
		},
		{
			name: "whole display",
			seq:  "\x1b[2J",
			want: []string{"", "", ""},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shown, err := ptytest.NewScreen(ptytest.Size{Cols: 4, Rows: 3})
			if err != nil {
				t.Fatal(err)
			}
			if err := shown.Apply([]byte("abcd\x1b[2;1Hefgh\x1b[3;1Hijkl" + tc.seq)); err != nil {
				t.Fatal(err)
			}
			assertTrimmedScreenRows(t, shown, tc.want)
		})
	}
}

func TestScreenClearsAWholeWideGlyphWhenAddressedThroughItsTrail(t *testing.T) {
	shown, err := ptytest.NewScreen(ptytest.Size{Cols: 4, Rows: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := shown.Apply([]byte("中a\x1b[1;2Hx")); err != nil {
		t.Fatal(err)
	}
	assertScreenRows(t, shown, " xa ")
}

func TestScreenRejectsInvalidErasureAndScrollMargins(t *testing.T) {
	shown, err := ptytest.NewScreen(ptytest.Size{Cols: 4, Rows: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, sequence := range []string{"\x1b[3J", "\x1b[3K", "\x1b[3;2r", "\x1b[1;4r"} {
		if err := shown.Apply([]byte(sequence)); !errors.Is(err, ptytest.ErrUnsupportedOutput) {
			t.Fatalf("sequence %q gave %v, want ErrUnsupportedOutput", sequence, err)
		}
	}
}

func TestScreenRejectsOutputOutsideItsCellSubset(t *testing.T) {
	shown, err := ptytest.NewScreen(ptytest.Size{Cols: 8, Rows: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := shown.Apply([]byte("\x1bPimage payload\x1b\\")); !errors.Is(err, ptytest.ErrUnsupportedOutput) {
		t.Fatalf("device-control output gave %v, want ErrUnsupportedOutput", err)
	}
}

func TestScreenRejectsInvalidSizesAndIncompleteOutput(t *testing.T) {
	if _, err := ptytest.NewScreen(ptytest.Size{}); err == nil {
		t.Fatal("NewScreen accepted a zero size")
	}
	shown, err := ptytest.NewScreen(ptytest.Size{Cols: 8, Rows: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := shown.Apply([]byte("\x1b[")); err != nil {
		t.Fatal(err)
	}
	if err := shown.Flush(); !errors.Is(err, ptytest.ErrUnsupportedOutput) {
		t.Fatalf("incomplete output gave %v, want ErrUnsupportedOutput", err)
	}
}

func assertScreenRows(t *testing.T, screen *ptytest.Screen, want ...string) {
	t.Helper()
	got := screen.Rows()
	if len(got) != len(want) {
		t.Fatalf("rows = %d, want %d", len(got), len(want))
	}
	for y := range want {
		if got[y] != want[y] {
			t.Errorf("row %d = %q, want %q", y, got[y], want[y])
		}
	}
}

func assertTrimmedScreenRows(t *testing.T, screen *ptytest.Screen, want []string) {
	t.Helper()
	got := screen.Rows()
	if len(got) != len(want) {
		t.Fatalf("rows = %d, want %d", len(got), len(want))
	}
	for y := range want {
		if trimmed := strings.TrimRight(got[y], " "); trimmed != want[y] {
			t.Errorf("row %d = %q, want %q", y, trimmed, want[y])
		}
	}
}
