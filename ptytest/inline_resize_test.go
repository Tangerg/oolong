package ptytest_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/ptytest"
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

func TestResizedInlineRowsClearBlankColumnsBeforeWrapping(t *testing.T) {
	for _, operation := range []string{"Print", "Append"} {
		for _, test := range []struct {
			name string
			last int
			want []string
		}{
			{name: "blank tail", last: 6, want: []string{"a    ", " b   ", ">    ", "     ", "     "}},
			{name: "blank row", last: 11, want: []string{"a    ", "     ", " b   ", ">    ", "     "}},
		} {
			t.Run(operation+"/"+test.name, func(t *testing.T) {
				canvas := grid.NewInline(13, 2)
				row := sparseInlineRow{last: test.last}
				var err error
				if operation == "Print" {
					err = canvas.Print(row)
				} else {
					err = canvas.Append(func(view grid.View) bool { row.Draw(view); return false })
				}
				if err != nil {
					t.Fatal(err)
				}
				canvas.Resize(5, 2)
				canvas.Frame().Text(0, 0, ">", grid.Style{})
				screen, err := ptytest.NewScreen(ptytest.Size{Cols: 5, Rows: 5})
				if err != nil {
					t.Fatal(err)
				}
				if err := screen.Apply([]byte("stale\r\nolder\x1b[H")); err != nil {
					t.Fatal(err)
				}
				flushInlineToScreen(t, canvas, screen)
				if got := screen.Rows(); !slices.Equal(got, test.want) {
					t.Fatalf("rows = %q, want %q", got, test.want)
				}
			})
		}
	}
}

func TestQueuedInlineRowsWrapWithoutChangingTheLiveAnchor(t *testing.T) {
	for _, test := range []struct {
		name     string
		parts    []string
		wantRows []string
	}{
		{name: "letters", parts: []string{"abcdefgh"}, wantRows: []string{"abcde", "fgh  "}},
		{name: "wide atom", parts: []string{"abcd界"}, wantRows: []string{"abcd ", "界   "}},
		{name: "continuation", parts: []string{"abc", "defgh"}, wantRows: []string{"abcde", "fgh  "}},
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
			for _, part := range test.parts {
				if err := canvas.Append(func(view grid.View) bool { inlineRow(part).Draw(view); return false }); err != nil {
					t.Fatal(err)
				}
			}
			canvas.Resize(5, 4)
			canvas.Frame().Text(0, 0, ">", grid.Style{})
			flushInlineToScreen(t, canvas, screen)
			want := append(slices.Clone(test.wantRows), ">    ", "     ", "KEEP ", "     ")
			if got := screen.Rows(); !slices.Equal(got, want) {
				t.Fatalf("rows after resize = %q, want %q", got, want)
			}
			canvas.Frame().Text(0, 0, ">>", grid.Style{})
			flushInlineToScreen(t, canvas, screen)
			want[2] = ">>   "
			if got := screen.Rows(); !slices.Equal(got, want) {
				t.Fatalf("rows after redraw = %q, want %q", got, want)
			}
		})
	}
}

func TestInlineWidthChangesEndTheOldAppendRow(t *testing.T) {
	for _, alreadyFlushed := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued", true: "flushed"}[alreadyFlushed], func(t *testing.T) {
			canvas := grid.NewInline(8, 2)
			screen, err := ptytest.NewScreen(ptytest.Size{Cols: 5, Rows: 5})
			if err != nil {
				t.Fatal(err)
			}
			if err := canvas.Append(func(view grid.View) bool { inlineRow("ab").Draw(view); return false }); err != nil {
				t.Fatal(err)
			}
			if alreadyFlushed {
				canvas.Frame().Text(0, 0, ">", grid.Style{})
				flushInlineToScreen(t, canvas, screen)
			}
			if err := canvas.Append(func(view grid.View) bool { inlineRow("cd").Draw(view); return false }); err != nil {
				t.Fatal(err)
			}
			canvas.Resize(5, 2)
			if col, open := canvas.Tail(); col != 0 || open {
				t.Fatalf("tail after width change = %d, %v", col, open)
			}
			if err := canvas.Append(func(view grid.View) bool {
				if width, _ := view.Size(); width != 5 {
					t.Fatalf("new append width = %d, want 5", width)
				}
				inlineRow("ef").Draw(view)
				return false
			}); err != nil {
				t.Fatal(err)
			}
			canvas.Frame().Text(0, 0, ">", grid.Style{})
			flushInlineToScreen(t, canvas, screen)
			want := []string{"abcd ", "ef   ", ">    ", "     ", "     "}
			if alreadyFlushed {
				want = []string{"ab   ", "cd   ", "ef   ", ">    ", "     "}
			}
			if got := screen.Rows(); !slices.Equal(got, want) {
				t.Fatalf("rows = %q, want %q", got, want)
			}
		})
	}
}

func TestQueuedInlineOutputSurvivesAZeroWidth(t *testing.T) {
	canvas := grid.NewInline(8, 2)
	if err := canvas.Print(inlineRow("abcde")); err != nil {
		t.Fatal(err)
	}
	canvas.Resize(0, 2)
	var output bytes.Buffer
	if err := canvas.Flush(&output); err != nil || output.Len() != 0 {
		t.Fatalf("zero-width flush = %v, %q", err, output.String())
	}
	canvas.Resize(5, 2)
	if err := canvas.Flush(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "abcde") {
		t.Fatalf("accepted output disappeared: %q", output.String())
	}
}

func flushInlineToScreen(t *testing.T, canvas *grid.Inline, screen *ptytest.Screen) {
	t.Helper()
	var output bytes.Buffer
	if err := canvas.Flush(&output); err != nil {
		t.Fatal(err)
	}
	if err := screen.Apply(output.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := screen.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestQueuedInlineOutputUsesTheCurrentTerminalWidth(t *testing.T) {
	for _, operation := range []string{"Print", "Append"} {
		for _, test := range []struct {
			name        string
			before, now int
			text, want  string
		}{
			{name: "narrower", before: 8, now: 5, text: "abcde", want: "abcde"},
			{name: "wider", before: 5, now: 8, text: "abcde", want: "abcde   "},
		} {
			t.Run(operation+"/"+test.name, func(t *testing.T) {
				canvas := grid.NewInline(test.before, 2)
				row := inlineRow(test.text)
				var err error
				if operation == "Print" {
					err = canvas.Print(row)
				} else {
					err = canvas.Append(func(view grid.View) bool { row.Draw(view); return false })
				}
				if err != nil {
					t.Fatal(err)
				}
				canvas.Resize(test.now, 2)
				canvas.Frame().Text(0, 0, ">", grid.Style{})
				screen, err := ptytest.NewScreen(ptytest.Size{Cols: test.now, Rows: 4})
				if err != nil {
					t.Fatal(err)
				}
				if err := screen.Apply([]byte(strings.Repeat("x", test.now) + "\r")); err != nil {
					t.Fatal(err)
				}
				flushInlineToScreen(t, canvas, screen)
				blank := strings.Repeat(" ", test.now)
				want := []string{test.want, ">" + blank[1:], blank, blank}
				if got := screen.Rows(); !slices.Equal(got, want) {
					t.Fatalf("rows = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestInlineRefusesAnUnrepresentableQueuedAtomBeforeWriting(t *testing.T) {
	for _, operation := range []string{"Print", "Append"} {
		t.Run(operation, func(t *testing.T) {
			canvas := grid.NewInline(4, 2)
			row := inlineRow("ab界")
			var err error
			if operation == "Print" {
				err = canvas.Print(row)
			} else {
				err = canvas.Append(func(view grid.View) bool { row.Draw(view); return false })
			}
			if err != nil {
				t.Fatal(err)
			}
			canvas.Resize(1, 2)
			var output bytes.Buffer
			err = canvas.Flush(&output)
			if !errors.Is(err, errors.ErrUnsupported) || output.Len() != 0 {
				t.Fatalf("unrepresentable output = %q, error %v; want refusal before any write", output.String(), err)
			}
			canvas.Resize(4, 2)
			canvas.Frame().Text(0, 0, ">", grid.Style{})
			screen, err := ptytest.NewScreen(ptytest.Size{Cols: 4, Rows: 3})
			if err != nil {
				t.Fatal(err)
			}
			flushInlineToScreen(t, canvas, screen)
			if got, want := screen.Rows(), []string{"ab界", ">   ", "    "}; !slices.Equal(got, want) {
				t.Fatalf("rows after restoring width = %q, want %q", got, want)
			}
		})
	}
}
