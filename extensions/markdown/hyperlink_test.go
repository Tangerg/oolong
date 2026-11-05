package markdown_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/grid"
)

// linkedText reads back what one rendered line shows and which of its words
// carry which destination.
func linkedText(t *testing.T, source string, look markdown.Look) (shown string, links map[string]string) {
	t.Helper()
	var doc markdown.Doc
	doc.SetBlocks(mustRender(t, source, look))
	const width = 120
	surface := grid.NewSurface(width, doc.HeightForWidth(width))
	doc.Draw(surface.View())
	links = map[string]string{}
	var all strings.Builder
	for y := range doc.HeightForWidth(width) {
		for x := range width {
			cell := cellAt(surface, x, y)
			all.WriteString(cell.Content())
			if cell.Link != "" {
				links[cell.Link] += cell.Content()
			}
		}
	}
	return strings.TrimSpace(all.String()), links
}

func TestOnlyBrowserAndMailDestinationsBecomeHyperlinks(t *testing.T) {
	addressed := markdown.Look{Target: grid.Style{Attr: grid.Dim}}
	for _, source := range []string{
		"[open](file:///etc/passwd)",
		"[open](FILE:///Applications/Calculator.app)",
		"[run](javascript:alert(1))",
		"[run](JavaScript:alert(1))",
		"[edit](vscode://file/etc/hosts)",
		"[chat](slack://open)",
		"[read](docs/guide.md)",
		"[jump](#install)",
		"[web](http:no-host)",
		"[mail](mailto:)",
		"<file:///etc/passwd>",
		"![diagram](file:///tmp/diagram.png)",
	} {
		for _, look := range []markdown.Look{{}, addressed} {
			if shown, links := linkedText(t, source, look); len(links) != 0 {
				t.Errorf("%s rendered %q with hyperlinks %v, want none", source, shown, links)
			}
		}
	}
	shown, _ := linkedText(t, "[open](file:///etc/passwd)", addressed)
	if shown != "open (file:///etc/passwd)" {
		t.Errorf("a configured address look shows %q, want the refused destination as text", shown)
	}
}

func TestBrowserAndMailDestinationsStayHyperlinks(t *testing.T) {
	for _, tc := range []struct {
		source, shown, destination, words string
	}{
		{"[docs](https://example.com/docs)", "docs", "https://example.com/docs", "docs"},
		{"[docs](HTTPS://example.com/docs)", "docs", "HTTPS://example.com/docs", "docs"},
		{"[plain](http://example.com)", "plain", "http://example.com", "plain"},
		{"[write](mailto:ada@example.test)", "write", "mailto:ada@example.test", "write"},
		{"<https://example.com/x>", "https://example.com/x", "https://example.com/x", "https://example.com/x"},
		{"<ada@example.test>", "ada@example.test", "mailto:ada@example.test", "ada@example.test"},
		{"[https://example.com/a](https://EXAMPLE.com/b)", "https://example.com/a", "https://EXAMPLE.com/b", "https://example.com/a"},
	} {
		shown, links := linkedText(t, tc.source, markdown.Look{})
		if shown != tc.shown || len(links) != 1 || links[tc.destination] != tc.words {
			t.Errorf("%s rendered %q with %v, want %q linking %q to %q", tc.source, shown, links, tc.shown, tc.words, tc.destination)
		}
	}
}

func TestALabelNamingAnotherAddressCannotHideTheDestination(t *testing.T) {
	for _, tc := range []struct{ source, shown, destination string }{
		{"[https://github.com](https://evil.example/x)", "https://github.com (https://evil.example/x)", "https://evil.example/x"},
		{"[see www.github.com now](https://evil.example)", "see www.github.com now (https://evil.example)", "https://evil.example"},
		{"![https://github.com/logo.png](https://evil.example/logo.png)", "[https://github.com/logo.png] (https://evil.example/logo.png)", "https://evil.example/logo.png"},
	} {
		shown, links := linkedText(t, tc.source, markdown.Look{})
		want := " (" + tc.destination + ")"
		if shown != tc.shown || len(links) != 1 || links[tc.destination] != want {
			t.Errorf("%s rendered %q with %v, want %q where only %q links", tc.source, shown, links, tc.shown, want)
		}
	}
}
