package ptytest_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Tangerg/oolong/ui/uitest/ptytest"
)

func TestVerticalCursorMovementRespectsApplicableScrollMargins(t *testing.T) {
	for _, test := range []struct {
		name   string
		stream string
		row    int
	}{
		{"up inside", "\x1b[2;4r\x1b[3;1H\x1b[99AX", 1},
		{"down inside", "\x1b[2;4r\x1b[3;1H\x1b[99BX", 3},
		{"up below", "\x1b[2;4r\x1b[5;1H\x1b[99AX", 1},
		{"down above", "\x1b[2;4r\x1b[1;1H\x1b[99BX", 3},
		{"up above", "\x1b[3;4r\x1b[2;1H\x1b[99AX", 0},
		{"down below", "\x1b[1;2r\x1b[3;1H\x1b[99BX", 4},
		{"default up", "\x1b[2;4r\x1b[3;1H\x1b[AX", 1},
		{"zero down", "\x1b[2;4r\x1b[3;1H\x1b[0BX", 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			for chunkSize := 1; chunkSize <= len(test.stream); chunkSize++ {
				shown, err := ptytest.NewScreen(ptytest.Size{Cols: 5, Rows: 5})
				if err != nil {
					t.Fatal(err)
				}
				for start := 0; start < len(test.stream); start += chunkSize {
					if err := shown.Apply([]byte(test.stream[start:min(start+chunkSize, len(test.stream))])); err != nil {
						t.Fatal(err)
					}
				}
				if err := shown.Flush(); err != nil {
					t.Fatal(err)
				}
				want := make([]string, 5)
				want[test.row] = "X"
				assertTrimmedScreenRows(t, shown, want)
			}
		})
	}
}

func TestLineFeedControlsShareScrollingAndPendingWrapSemantics(t *testing.T) {
	for _, control := range []byte{'\n', '\v', '\f'} {
		t.Run(fmt.Sprintf("%U", control), func(t *testing.T) {
			for _, test := range []struct {
				prefix string
				want   []string
			}{
				{"X", []string{"X", " Y", ""}},
				{"ABCDE", []string{"ABCDE", "    Y", ""}},
				{"\x1b[2;3r\x1b[3;1HX", []string{"", "X", " Y"}},
			} {
				shown, err := ptytest.NewScreen(ptytest.Size{Cols: 5, Rows: 3})
				if err != nil {
					t.Fatal(err)
				}
				for _, chunk := range []string{test.prefix, string(control), "Y"} {
					if err := shown.Apply([]byte(chunk)); err != nil {
						t.Fatal(err)
					}
				}
				assertTrimmedScreenRows(t, shown, test.want)
			}
		})
	}
}

func TestUnmodelledControlsAreRejectedBeforeLaterText(t *testing.T) {
	for _, control := range []rune{'\x0e', '\x0f', '\x18', '\x1a', '\u0085'} {
		t.Run(fmt.Sprintf("%U", control), func(t *testing.T) {
			shown, err := ptytest.NewScreen(ptytest.Size{Cols: 5, Rows: 2})
			if err != nil {
				t.Fatal(err)
			}
			if err := shown.Apply([]byte("X" + string(control) + "Y")); !errors.Is(err, ptytest.ErrUnsupportedOutput) {
				t.Fatalf("control returned %v, want ErrUnsupportedOutput", err)
			}
			assertTrimmedScreenRows(t, shown, []string{"X", ""})
		})
	}
}

func TestNonCellControlsPreserveGraphemeAndWrap(t *testing.T) {
	for _, control := range []byte{'\x00', '\a', '\x7f'} {
		t.Run(fmt.Sprintf("%U", control), func(t *testing.T) {
			shown, err := ptytest.NewScreen(ptytest.Size{Cols: 2, Rows: 2})
			if err != nil {
				t.Fatal(err)
			}
			if err := shown.Apply([]byte("ab" + string(control) + "\u0301X")); err != nil {
				t.Fatal(err)
			}
			assertTrimmedScreenRows(t, shown, []string{"ab́", "X"})
		})
	}
}

func TestTabAtTheRightMarginPreservesPendingWrap(t *testing.T) {
	for _, test := range []struct {
		name, stream string
		rows         []string
	}{
		{"ordinary row", "ABCDE\tX", []string{"ABCDE", "X", ""}},
		{"scroll margin", "\x1b[2;3r\x1b[3;1HABCDE\tX", []string{"", "ABCDE", "X"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for chunkSize := 1; chunkSize <= len(test.stream); chunkSize++ {
				shown, err := ptytest.NewScreen(ptytest.Size{Cols: 5, Rows: 3})
				if err != nil {
					t.Fatal(err)
				}
				for start := 0; start < len(test.stream); start += chunkSize {
					if err := shown.Apply([]byte(test.stream[start:min(start+chunkSize, len(test.stream))])); err != nil {
						t.Fatal(err)
					}
				}
				if err := shown.Flush(); err != nil {
					t.Fatal(err)
				}
				assertTrimmedScreenRows(t, shown, test.rows)
			}
		})
	}
}
