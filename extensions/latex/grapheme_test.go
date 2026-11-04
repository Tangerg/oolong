package latex_test

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/extensions/latex"
)

func TestLiteralGraphemesKeepTheirGeometryInsideFractions(t *testing.T) {
	for _, test := range []struct {
		cluster string
		width   int
	}{
		{cluster: "👩‍💻", width: 2},
		{cluster: "👩🏽‍💻", width: 2},
		{cluster: "🇨🇳", width: 2},
		{cluster: "e\u0301", width: 1},
		{cluster: "α\u0301", width: 1},
	} {
		t.Run(test.cluster, func(t *testing.T) {
			source := `\frac{` + test.cluster + `}{x}`
			formula := latex.Render(source, latex.Look{})
			if err := formula.Err(); err != nil {
				t.Fatal(err)
			}
			want := []string{" " + test.cluster, strings.Repeat("─", test.width+2), " x"}
			if got := rendered(t, source); !slices.Equal(got, want) || formula.Width() != test.width+2 {
				t.Fatalf("fraction = %q width=%d; want %q width=%d", got, formula.Width(), want, test.width+2)
			}
		})
	}
}

func TestUnbracedScriptsConsumeOneCompleteGrapheme(t *testing.T) {
	for _, cluster := range []string{"👩‍💻", "👩🏽‍💻", "🇨🇳", "e\u0301", "α\u0301"} {
		for _, template := range []string{`x^%sa`, `x_%sa`, `x^%s_2`, `\frac{x}{y}^%sa`} {
			source := fmt.Sprintf(template, cluster)
			grouped := fmt.Sprintf(template, "{"+cluster+"}")
			if got, want := rendered(t, source), rendered(t, grouped); !slices.Equal(got, want) {
				t.Errorf("Render(%q) = %q; grouped atom = %q", source, got, want)
			}
		}
	}
	if got, want := rendered(t, "x^🇨🇳🇯"), rendered(t, "x^{🇨🇳}🇯"); !slices.Equal(got, want) {
		t.Fatalf("regional-indicator script = %q; want %q", got, want)
	}
}

func TestGraphemeJoiningCannotConsumeFormulaSyntax(t *testing.T) {
	for _, source := range []string{"{\u0600}", "{x^\u0600}", "\\sqrt[x^\u0600]{x}"} {
		if err := latex.Render(source, latex.Look{}).Err(); err != nil {
			t.Errorf("syntax after Prepend in %q: %v", source, err)
		}
	}
	for _, ending := range []string{"}", "$", "#", "&", "\\", "^", "_"} {
		source := "\u0600" + ending
		if err := latex.Render(source, latex.Look{}).Err(); err == nil {
			t.Errorf("grapheme joining hid invalid syntax in %q", source)
		}
	}
	if got, want := rendered(t, "{\u0600α}"), []string{"\u0600α"}; !slices.Equal(got, want) {
		t.Fatalf("literal Prepend cluster = %q; want %q", got, want)
	}
}

func TestCrossAtomGraphemesReportUnrepresentableLayout(t *testing.T) {
	for _, source := range []string{
		"👩{\u200d💻}^2",
		"👩\\textbf{\u200d💻}^2",
		"\\frac{👩{\u200d💻}}{x}",
		"{\u0600\\alpha}",
		"\\sqrt[\u0600]{x}",
		"🇨{🇳}^2",
		"🇨{🇺🇸ा}x^1",
		"x^1{\u0600}{界}",
		// Opposite width changes can preserve the row width while moving a script's base.
		"🇨{🇺🇸ा}x^1{\u0600}{界}",
	} {
		formula := latex.Render(source, latex.Look{})
		if err := formula.Err(); err == nil || !strings.Contains(err.Error(), "grapheme") {
			t.Fatalf("Render(%q) = %v; want an unsupported geometry diagnostic", source, err)
		}
		if formula.Source() != source {
			t.Fatalf("source = %q, want %q", formula.Source(), source)
		}
		if lines := formula.Lines(); len(lines) != 1 || lines[0].String() != source {
			t.Fatalf("fallback = %v, want complete source %q", lines, source)
		}
	}
	for _, source := range []string{"", "{}", "x{}", "{\u0301}", "\\frac{}{}", "👩‍💻^2", "\\frac{e{\u0301}}{x}", "e{\u0301}{\u0301}^2"} {
		if err := latex.Render(source, latex.Look{}).Err(); err != nil {
			t.Fatalf("representable Render(%q): %v", source, err)
		}
	}
}

func BenchmarkCrossAtomCombiningMarks(b *testing.B) {
	for _, count := range []int{256, 1024, 4096} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			source := "e" + strings.Repeat("{\u0301}", count) + "^2"
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				if err := latex.Render(source, latex.Look{}).Err(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLiteralGraphemeRuns(b *testing.B) {
	for _, count := range []int{256, 1024, 4096} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			source := strings.Repeat("👩‍💻", count)
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				if err := latex.Render(source, latex.Look{}).Err(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkUnbracedGraphemeScripts(b *testing.B) {
	for _, count := range []int{256, 1024, 4096} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			source := strings.Repeat("x^👩‍💻+", count)
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				if err := latex.Render(source, latex.Look{}).Err(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
