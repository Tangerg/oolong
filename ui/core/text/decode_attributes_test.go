package text_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestSGRCanDisableBaseAttributes(t *testing.T) {
	for _, test := range []struct {
		name string
		attr grid.Attr
		off  string
		on   string
	}{
		{"bold", grid.Bold, "22", "1"},
		{"dim", grid.Dim, "22", "2"},
		{"intensity", grid.Bold | grid.Dim, "22", "1;2"},
		{"italic", grid.Italic, "23", "3"},
		{"underline", grid.Underline, "24", "4"},
		{"underline shape", grid.Underline, "4:0", "4:3"},
		{"reverse", grid.Reverse, "27", "7"},
		{"strike", grid.Strike, "29", "9"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := "A\x1b[" + test.off + "mB\x1b[" + test.on + "mC\x1b[0mD"
			for _, chunked := range []bool{false, true} {
				decoder := text.Decoder{Base: grid.Style{Attr: test.attr}}
				if chunked {
					for _, b := range []byte(input) {
						feedDecoder(t, &decoder, string([]byte{b}))
					}
				} else {
					feedDecoder(t, &decoder, input)
				}
				for _, span := range flushDecoder(t, &decoder)[0] {
					for _, r := range span.Text {
						want := test.attr
						if r == 'B' {
							want = 0
						}
						if span.Style.Attr != want {
							t.Errorf("chunked %v, %c attributes = %v, want %v", chunked, r, span.Style.Attr, want)
						}
					}
				}
			}
		})
	}
}

func TestBaseChangesOnlyAffectUnspecifiedRendition(t *testing.T) {
	decoder := text.Decoder{Base: grid.Style{Attr: grid.Bold}}
	feedDecoder(t, &decoder, "A\x1b[22m")
	decoder.Base = grid.Style{FG: grid.RGBColor(12, 34, 56), Attr: grid.Bold | grid.Italic}
	feedDecoder(t, &decoder, "B\x1b[31mC\x1b[39mD")
	line := flushDecoder(t, &decoder)[0]
	for _, span := range line {
		if span.Text == "A" {
			if span.Style.Attr != grid.Bold || !span.Style.FG.Default() {
				t.Fatalf("published prefix changed: %#v", span)
			}
			continue
		}
		if span.Style.Attr != grid.Italic {
			t.Errorf("explicit attribute reset lost after Base change: %#v", span)
		}
		if !strings.Contains(span.Text, "C") && span.Style.FG != decoder.Base.FG {
			t.Errorf("default foreground did not follow Base: %#v", span)
		}
	}
	decoder.Reset()
	feedDecoder(t, &decoder, "E")
	if got := flushDecoder(t, &decoder)[0][0].Style; got != decoder.Base {
		t.Errorf("Reset style = %#v, want Base %#v", got, decoder.Base)
	}
}
