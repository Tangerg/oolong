package latex_test

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"
	"weak"

	"github.com/Tangerg/oolong/extensions/latex"
)

func TestFormulaDetachesConstructionGlyphStorage(t *testing.T) {
	for _, test := range []struct {
		name string
		look func(string) latex.Look
		want string
	}{
		{"unused structure", func(glyph string) latex.Look {
			return latex.Look{Glyphs: latex.Glyphs{Horizontal: glyph}}
		}, "…"},
		{"clipping policy", func(glyph string) latex.Look {
			return latex.Look{Glyphs: latex.Glyphs{Ellipsis: glyph}}
		}, "!"},
	} {
		t.Run(test.name, func(t *testing.T) {
			formula, supplied := func() (*latex.Formula, weak.Pointer[byte]) {
				storage := "!" + strings.Repeat("discarded glyph storage", 1024)
				reference := weak.Make(unsafe.StringData(storage)) //nolint:gosec // Observe the allocation without reading through its pointer.
				return latex.Render("wide", test.look(storage[:1])), reference
			}()
			runtime.GC()
			retained := supplied.Value() != nil
			rows := formula.Rows(1)
			if formula.Err() != nil || len(rows) != 1 || rows[0].Content.String() != test.want {
				t.Fatalf("clipped formula = %v, %v; want %q", rows, formula.Err(), test.want)
			}
			runtime.KeepAlive(formula)
			if retained {
				t.Fatal("formula retained the caller's glyph allocation")
			}
		})
	}
}
