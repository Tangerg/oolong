package text_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestDecoderRejectsInvalidLimits(t *testing.T) {
	for _, config := range []text.DecoderConfig{{MaxLineBytes: -1}, {MaxLineSpans: -1}} {
		if decoder, err := text.NewDecoder(config); decoder != nil || err == nil {
			t.Fatalf("NewDecoder(%+v) = %v, %v", config, decoder, err)
		}
	}
}

func TestDecoderLineAdmissionDoesNotDependOnReadSplits(t *testing.T) {
	for _, test := range []struct {
		name   string
		config text.DecoderConfig
		source string
		prefix string
	}{
		{"bytes", text.DecoderConfig{MaxLineBytes: 4}, "12345", "1234"},
		{"UTF8", text.DecoderConfig{MaxLineBytes: 4}, "好ab", "好a"},
		{"malformed UTF8", text.DecoderConfig{MaxLineBytes: 4}, "x\xff\xfe", "x\ufffd"},
		{"carriage returns", text.DecoderConfig{MaxLineBytes: 4}, "12\r34\r5", "1234"},
		{"style spans", text.DecoderConfig{MaxLineSpans: 2}, "a\x1b[31mb\x1b[32mc", "a\x1b[31mb"},
		{"link bytes", text.DecoderConfig{MaxLineBytes: 10}, "\x1b]8;;https://a\x07ab", "\x1b]8;;https://a\x07a"},
		{"new span link", text.DecoderConfig{MaxLineBytes: 19}, "\x1b]8;;https://a\x07a\x1b[31mb", "\x1b]8;;https://a\x07a"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := "ok\n" + test.source + "\nnot accepted"
			want := text.Decode(test.prefix, grid.Style{})[0]
			for split := 0; split <= len(source); split++ {
				decoder := newLimitedDecoder(t, test.config)
				var complete []text.Line
				var failure error
				for _, chunk := range []string{source[:split], source[split:]} {
					lines, err := decoder.Feed(chunk)
					complete = append(complete, lines...)
					if err != nil {
						if !errors.Is(err, text.ErrLineLimit) {
							t.Fatalf("split %d: %v", split, err)
						}
						failure = err
					}
				}
				if failure == nil || len(complete) != 1 || complete[0].String() != "ok" {
					t.Fatalf("split %d: completed=%v, failure=%v", split, complete, failure)
				}
				if got := decoder.Open(); styled(got) != styled(want) {
					t.Fatalf("split %d: open=%s, want %s", split, styled(got), styled(want))
				}
				for range 2 {
					lines, err := decoder.Flush()
					if len(lines) != 0 || !errors.Is(err, text.ErrLineLimit) || styled(decoder.Open()) != styled(want) {
						t.Fatalf("split %d: Flush erased refusal: %v, %v, open %v", split, lines, err, decoder.Open())
					}
				}
			}
		})
	}
}

func TestDecoderLimitsReleaseAtNewlineAndMergeEquivalentStyles(t *testing.T) {
	decoder := newLimitedDecoder(t, text.DecoderConfig{MaxLineBytes: 4, MaxLineSpans: 2})
	for range 3 {
		lines := feedDecoder(t, decoder, "a\x1b[31mb\x1b[31mc\rd\x00\n")
		if len(lines) != 1 || lines[0].String() != "abcd" || len(lines[0]) > 2 {
			t.Fatalf("completed %v", lines)
		}
		if open := decoder.Open(); len(open) != 0 {
			t.Fatalf("completed line retained %v", open)
		}
	}
	if lines := flushDecoder(t, decoder); len(lines) != 0 {
		t.Fatalf("EOF invented a line: %v", lines)
	}
	feedDecoder(t, decoder, "four")
	if lines := flushDecoder(t, decoder); len(lines) != 1 || lines[0].String() != "four" {
		t.Fatalf("EOF lost the accepted line: %v", lines)
	}
	if lines := flushDecoder(t, decoder); len(lines) != 0 {
		t.Fatalf("repeated EOF duplicated output: %v", lines)
	}
}

func TestDecoderActiveLinkSurvivesNewlinesWithinEachLineBudget(t *testing.T) {
	const target = "https://a"
	decoder := newLimitedDecoder(t, text.DecoderConfig{MaxLineBytes: len(target) + 1, MaxLineSpans: 1})
	feedDecoder(t, decoder, "\x1b]8;;"+target+"\x07")
	for _, value := range []string{"a", "b", "c"} {
		lines := feedDecoder(t, decoder, value+"\n")
		if len(lines) != 1 || len(lines[0]) != 1 || lines[0][0].Text != value || lines[0][0].Link != target {
			t.Fatalf("completed linked line = %v", lines)
		}
	}
	feedDecoder(t, decoder, "d")
	if _, err := decoder.Feed("e"); !errors.Is(err, text.ErrLineLimit) || decoder.Open().String() != "d" {
		t.Fatalf("persistent target escaped the next line's budget: %v, %v", decoder.Open(), err)
	}
}

func TestDecoderBoundsActiveLinksBeforeTextArrives(t *testing.T) {
	decoder := newLimitedDecoder(t, text.DecoderConfig{MaxLineBytes: 10, MaxLineSpans: 1})
	feedDecoder(t, decoder, strings.Repeat("\x1b[31m\x1b[32m\r\x00", 1000))
	feedDecoder(t, decoder, "\x1b]8;;https://a\x07a")
	feedDecoder(t, decoder, strings.Repeat("\x1b]8;;https://a\x07", 10))
	before := decoder.Open()
	if _, err := decoder.Feed("\x1b]8;;https://b\x07"); !errors.Is(err, text.ErrLineLimit) {
		t.Fatalf("unrepresented active target bypassed line budget: %v", err)
	}
	if styled(decoder.Open()) != styled(before) {
		t.Fatal("a refused link changed accepted text")
	}

	decoder.Reset()
	if _, err := decoder.Feed("\x1b]8;;https://too-long\x07"); !errors.Is(err, text.ErrLineLimit) {
		t.Fatalf("target without text was not bounded: %v", err)
	}
	if len(decoder.Open()) != 0 {
		t.Fatal("link control invented a span")
	}
}

func TestDecoderEOFFailureAndResetKeepOneAdmissionOwner(t *testing.T) {
	base := grid.Style{FG: grid.RGBColor(3, 5, 7)}
	decoder := newLimitedDecoder(t, text.DecoderConfig{Base: base, MaxLineBytes: 4, MaxLineSpans: 1})
	feedDecoder(t, decoder, "\x1b[31mx\xe2\x82")
	if lines, err := decoder.Flush(); len(lines) != 0 || !errors.Is(err, text.ErrLineLimit) {
		t.Fatalf("EOF accepted expanded malformed bytes: %v, %v", lines, err)
	}
	if got := decoder.Open().String(); got != "x\ufffd" {
		t.Fatalf("EOF lost accepted prefix: %q", got)
	}
	if lines, err := decoder.Feed("\nnew"); len(lines) != 0 || !errors.Is(err, text.ErrLineLimit) {
		t.Fatalf("refused decoder resumed: %v, %v", lines, err)
	}
	decoder.Reset()
	lines := feedDecoder(t, decoder, "next\n")
	if len(lines) != 1 || len(lines[0]) != 1 || lines[0][0].Text != "next" || lines[0][0].Style != base {
		t.Fatalf("reset lost configured Base or retained old state: %v", lines)
	}
	if _, err := decoder.Feed("again"); !errors.Is(err, text.ErrLineLimit) || decoder.Open().String() != "agai" {
		t.Fatalf("reset lost limits: %v, %v", decoder.Open(), err)
	}
}

func newLimitedDecoder(t *testing.T, config text.DecoderConfig) *text.Decoder {
	t.Helper()
	decoder, err := text.NewDecoder(config)
	if err != nil {
		t.Fatal(err)
	}
	return decoder
}
