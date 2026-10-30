package ptytest_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/uitest/ptytest"
)

type inlineRow string

func (inlineRow) HeightForWidth(int) int { return 1 }

func (r inlineRow) Draw(view grid.View) {
	columns := view.Text(0, 0, string(r), grid.Style{Attr: grid.Bold})
	view.Link(0, 0, columns, "https://example.test/row")
}

type sparseInlineRow struct{ last int }

func (sparseInlineRow) HeightForWidth(int) int { return 1 }

func (r sparseInlineRow) Draw(view grid.View) {
	view.Text(0, 0, "a", grid.Style{})
	view.Text(r.last, 0, "b", grid.Style{})
}

func captureInlineRows(tb testing.TB, width int, body grid.Drawable) grid.Frozen {
	tb.Helper()
	snapshot, err := grid.Capture(grid.CaptureConfig{Width: width, Content: body})
	if err != nil {
		tb.Fatal(err)
	}
	return snapshot
}

func TestResizedInlineRowsClearBlankColumnsBeforeWrapping(t *testing.T) {
	for _, test := range []struct {
		name string
		last int
		want []string
	}{
		{"blank tail", 6, []string{"a    ", " b   ", ">    ", "     ", "     "}},
		{"blank row", 11, []string{"a    ", "     ", " b   ", ">    ", "     "}},
	} {
		t.Run(test.name, func(t *testing.T) {
			canvas := grid.NewInline(13, 2)
			frozen := captureInlineRows(t, 13, sparseInlineRow{last: test.last})
			canvas.Resize(5, 2)
			canvas.Frame().Text(0, 0, ">", grid.Style{})
			screen, err := ptytest.NewScreen(ptytest.Size{Cols: 5, Rows: 5})
			if err != nil {
				t.Fatal(err)
			}
			if err := screen.Apply([]byte("stale\r\nolder\x1b[H")); err != nil {
				t.Fatal(err)
			}
			flushInlineToScreen(t, canvas, screen, frozen)
			if got := screen.Rows(); !slices.Equal(got, test.want) {
				t.Fatalf("rows=%q want=%q", got, test.want)
			}
		})
	}
}

func TestCapturedInlineRowsWrapWithoutChangingTheLiveAnchor(t *testing.T) {
	for _, test := range []struct {
		name, source string
		wantRows     []string
	}{
		{"letters", "abcdefgh", []string{"abcde", "fgh  "}},
		{"wide atom", "abcd界", []string{"abcd ", "界   "}},
	} {
		t.Run(test.name, func(t *testing.T) {
			canvas := grid.NewInline(8, 4)
			screen, err := ptytest.NewScreen(ptytest.Size{Cols: 5, Rows: 6})
			if err != nil {
				t.Fatal(err)
			}
			if err := screen.Apply([]byte("\x1b[5;1HKEEP\x1b[H")); err != nil {
				t.Fatal(err)
			}
			frame := canvas.Frame()
			for y := range 4 {
				frame.Text(0, y, "old!", grid.Style{})
			}
			flushInlineToScreen(t, canvas, screen)
			frozen := captureInlineRows(t, 8, inlineRow(test.source))
			canvas.Resize(5, 4)
			canvas.Frame().Text(0, 0, ">", grid.Style{})
			flushInlineToScreen(t, canvas, screen, frozen)
			want := append(slices.Clone(test.wantRows), ">    ", "     ", "KEEP ", "     ")
			if got := screen.Rows(); !slices.Equal(got, want) {
				t.Fatalf("resized rows=%q want=%q", got, want)
			}
			canvas.Frame().Text(0, 0, ">>", grid.Style{})
			flushInlineToScreen(t, canvas, screen)
			want[2] = ">>   "
			if got := screen.Rows(); !slices.Equal(got, want) {
				t.Fatalf("redrawn rows=%q want=%q", got, want)
			}
		})
	}
}

func TestCompletedContributionsHaveIndependentRowsAcrossResize(t *testing.T) {
	for _, alreadyFlushed := range []bool{false, true} {
		t.Run(map[bool]string{false: "one frame", true: "two frames"}[alreadyFlushed], func(t *testing.T) {
			canvas := grid.NewInline(8, 2)
			screen, err := ptytest.NewScreen(ptytest.Size{Cols: 5, Rows: 5})
			if err != nil {
				t.Fatal(err)
			}
			first := captureInlineRows(t, 8, inlineRow("ab"))
			pending := []grid.Frozen{first}
			if alreadyFlushed {
				canvas.Frame().Text(0, 0, ">", grid.Style{})
				flushInlineToScreen(t, canvas, screen, first)
				pending = nil
			}
			pending = append(pending, captureInlineRows(t, 8, inlineRow("cd")))
			canvas.Resize(5, 2)
			pending = append(pending, captureInlineRows(t, 5, inlineRow("ef")))
			canvas.Frame().Text(0, 0, ">", grid.Style{})
			flushInlineToScreen(t, canvas, screen, pending...)
			if got, want := screen.Rows(), []string{"ab   ", "cd   ", "ef   ", ">    ", "     "}; !slices.Equal(got, want) {
				t.Fatalf("rows=%q want=%q", got, want)
			}
		})
	}
}

func TestCapturedInlineOutputSurvivesAZeroWidthRefusal(t *testing.T) {
	canvas := grid.NewInline(8, 2)
	frozen := captureInlineRows(t, 8, inlineRow("abcde"))
	canvas.Resize(0, 2)
	var output bytes.Buffer
	if err := canvas.Flush(&output, frozen); err == nil || output.Len() != 0 {
		t.Fatalf("zero width=%v output=%q", err, output.String())
	}
	canvas.Resize(5, 2)
	if err := canvas.Flush(&output, frozen); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "abcde") {
		t.Fatalf("captured output disappeared: %q", output.String())
	}
}

func flushInlineToScreen(t *testing.T, canvas *grid.Inline, screen *ptytest.Screen, contents ...grid.Frozen) {
	t.Helper()
	var output bytes.Buffer
	if err := canvas.Flush(&output, contents...); err != nil {
		t.Fatal(err)
	}
	if err := screen.Apply(output.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := screen.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestCapturedInlineOutputUsesTheCurrentTerminalWidth(t *testing.T) {
	for _, test := range []struct {
		name        string
		before, now int
		text, want  string
	}{
		{"narrower", 8, 5, "abcde", "abcde"}, {"wider", 5, 8, "abcde", "abcde   "},
	} {
		t.Run(test.name, func(t *testing.T) {
			canvas := grid.NewInline(test.before, 2)
			frozen := captureInlineRows(t, test.before, inlineRow(test.text))
			canvas.Resize(test.now, 2)
			canvas.Frame().Text(0, 0, ">", grid.Style{})
			screen, err := ptytest.NewScreen(ptytest.Size{Cols: test.now, Rows: 4})
			if err != nil {
				t.Fatal(err)
			}
			if err := screen.Apply([]byte(strings.Repeat("x", test.now) + "\r")); err != nil {
				t.Fatal(err)
			}
			flushInlineToScreen(t, canvas, screen, frozen)
			blank := strings.Repeat(" ", test.now)
			want := []string{test.want, ">" + blank[1:], blank, blank}
			if got := screen.Rows(); !slices.Equal(got, want) {
				t.Fatalf("rows=%q want=%q", got, want)
			}
		})
	}
}

func TestInlineRefusesAnUnrepresentableCapturedAtomBeforeWriting(t *testing.T) {
	canvas := grid.NewInline(4, 2)
	frozen := captureInlineRows(t, 4, inlineRow("ab界"))
	canvas.Resize(1, 2)
	var output bytes.Buffer
	if err := canvas.Flush(&output, frozen); !errors.Is(err, errors.ErrUnsupported) || output.Len() != 0 {
		t.Fatalf("unrepresentable output=%q err=%v", output.String(), err)
	}
	canvas.Resize(4, 2)
	canvas.Frame().Text(0, 0, ">", grid.Style{})
	screen, err := ptytest.NewScreen(ptytest.Size{Cols: 4, Rows: 3})
	if err != nil {
		t.Fatal(err)
	}
	flushInlineToScreen(t, canvas, screen, frozen)
	if got, want := screen.Rows(), []string{"ab界", ">   ", "    "}; !slices.Equal(got, want) {
		t.Fatalf("restored rows=%q want=%q", got, want)
	}
}
