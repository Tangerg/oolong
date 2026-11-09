package latex_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/extensions/latex"
	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestTextArgumentsKeepLiteralPunctuation(t *testing.T) {
	for _, command := range []string{
		`\textbf`, `\textit`, `\texttt`, `\textsf`, `\textcal`, `\textdefault`,
		`\textbb`, `\textfrak`, `\textscr`, `\textregular`,
	} {
		t.Run(command, func(t *testing.T) {
			for _, body := range []string{"file-name.txt", "x+y=z", "a - b", "one  two", "a*b/c:d.'"} {
				for _, look := range []latex.Look{{}, {Glyphs: latex.ASCII()}} {
					source := command + "{" + body + "}"
					formula := latex.Render(source, look)
					if err := formula.Err(); err != nil {
						t.Fatalf("Render(%q): %v", source, err)
					}
					if got := formulaRows(t, formula, 80); !slices.Equal(got, []string{body}) {
						t.Fatalf("Render(%q) plain=%t: %q, want %q", source, look.Glyphs.Plain, got, body)
					}
					if got := formula.Lines(); len(got) != 1 || got[0].String() != body {
						t.Fatalf("text projection disagrees with drawing: %v", got)
					}
				}
			}
		})
	}
}

func TestTextArgumentsKeepTheirGrammarAndNestedEmphasis(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{`\texttt{file\_name\{v1\}\%}`, "file_name{v1}%"},
		{`\texttt{a{b-c}+d}`, "ab-c+d"},
		{`\textbf{file-\textit{name+value}.txt}`, "file-name+value.txt"},
		{"\\texttt{a% hidden + -\nb}", "ab"},
		{"\\texttt{a% hidden + -\r\nb}", "ab"},
		{`\textbf{label \mathbf{x-y} label-z}`, "label x − y label-z"},
		{`\mathbf{x-\texttt{file-name}+y}`, "x − file-name + y"},
	} {
		if got := rendered(t, test.source); !slices.Equal(got, []string{test.want}) {
			t.Errorf("Render(%q) = %q, want %q", test.source, got, test.want)
		}
	}
	formula := latex.Render(`\textbf{a-\textit{b+c}}`, latex.Look{})
	surface := grid.NewSurface(12, 1)
	formula.Draw(surface.View())
	for x, want := range []grid.Attr{grid.Bold, grid.Bold, grid.Bold | grid.Italic, grid.Bold | grid.Italic, grid.Bold | grid.Italic} {
		cell, ok := surface.CellAt(x, 0)
		if !ok || cell.Style.Attr != want {
			t.Fatalf("column %d attr=%v, want %v", x, cell.Style.Attr, want)
		}
	}
}

func TestTextArgumentsRejectUnescapedScripts(t *testing.T) {
	for _, source := range []string{`\texttt{file_name}`, `\textbf{x^2}`, `\textit{a{b_c}}`} {
		formula := latex.Render(source, latex.Look{})
		if err := formula.Err(); err == nil || !strings.Contains(err.Error(), "scripts require a mathematical argument") {
			t.Fatalf("Render(%q): %v", source, err)
		}
		if lines := formula.Lines(); len(lines) != 1 || lines[0].String() != source {
			t.Fatalf("unsupported source was silently changed: %v", lines)
		}
	}
	if got, want := rendered(t, `\texttt{file-name}^2`), []string{"         2", "file-name"}; !slices.Equal(got, want) {
		t.Fatalf("a script outside the text argument lost its mathematical context: %q", got)
	}
}

func TestOperatorArgumentsKeepMathematicsAndTextPunctuation(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{`\operatorname{foo-bar}`, "foo-bar"},
		{`\operatorname{a*b/c:d.'}`, "a*b/c:d.'"},
		{`\operatorname{x+y=z}`, "x + y = z"},
		{`\operatorname{lim sup}`, "limsup"},
		{`\operatorname{lim\ sup}`, "lim sup"},
		{`\operatorname{a\mathbf{-b}}`, "a-b"},
		{`\operatorname{a\texttt{+b}-c}`, "a+b-c"},
	} {
		for _, look := range []latex.Look{{}, {Glyphs: latex.ASCII()}} {
			formula := latex.Render(test.source, look)
			if err := formula.Err(); err != nil {
				t.Fatalf("Render(%q): %v", test.source, err)
			}
			if got := formulaRows(t, formula, 80); !slices.Equal(got, []string{test.want}) {
				t.Errorf("Render(%q) = %q, want %q", test.source, got, test.want)
			}
		}
	}
	if got, want := rendered(t, `\operatorname{foo_1^2}`), rendered(t, `foo_1^2`); !slices.Equal(got, want) {
		t.Fatalf("operator scripts = %q, want %q", got, want)
	}
}
