package ptytest_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ptytest"
)

func TestScreenGraphemesAreIndependentOfEveryByteBoundary(t *testing.T) {
	for _, source := range []string{"é", "👩‍💻", "🇦🇧", "ab☺️", "abc👩‍💻", "é\x1b[2;1Hend"} {
		full, _ := ptytest.NewScreen(ptytest.Size{Cols: 4, Rows: 3})
		if err := full.Apply([]byte(source)); err != nil {
			t.Fatal(err)
		}
		if err := full.Flush(); err != nil {
			t.Fatal(err)
		}
		for at := 0; at <= len(source); at++ {
			screen, _ := ptytest.NewScreen(ptytest.Size{Cols: 4, Rows: 3})
			for _, part := range []string{source[:at], source[at:]} {
				if err := screen.Apply([]byte(part)); err != nil {
					t.Fatal(err)
				}
			}
			if err := screen.Flush(); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(screen.Rows(), full.Rows()) {
				t.Fatalf("%q split %d: %q != %q", source, at, screen.Rows(), full.Rows())
			}
		}
	}
}
