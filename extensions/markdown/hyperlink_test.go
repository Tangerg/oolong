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
		{"[ht&#1;tps://example.com](https://example.com/next)", "https://example.com", "https://example.com/next", "https://example.com"},
		{"[https://example.com/a\u200db](https://example.com/next)", "https://example.com/a\u200db", "https://example.com/next", "https://example.com/a\u200db"},
		{"[https://example.com/a\u2066b\u2069](https://example.com/next)", "https://example.com/a\u2066b\u2069", "https://example.com/next", "https://example.com/a\u2066b\u2069"},
		{"[https://例子.测试/a](https://xn--fsqu00a.xn--0zwm56d/b)", "https://例子.测试/a", "https://xn--fsqu00a.xn--0zwm56d/b", "https://例子.测试/a"},
		{"[https://BÜCHER.example](https://xn--bcher-kva.example)", "https://BÜCHER.example", "https://xn--bcher-kva.example", "https://BÜCHER.example"},
		{"[https://例子.测试/#label](https://xn--fsqu00a.xn--0zwm56d/next#target)", "https://例子.测试/#label", "https://xn--fsqu00a.xn--0zwm56d/next#target", "https://例子.测试/#label"},
		{"[https://%E4%BE%8B%E5%AD%90.%E6%B5%8B%E8%AF%95](https://例子.测试)", "https://%E4%BE%8B%E5%AD%90.%E6%B5%8B%E8%AF%95", "https://例子.测试", "https://%E4%BE%8B%E5%AD%90.%E6%B5%8B%E8%AF%95"},
		{"[https://[2001:0db8:0:0:0:0:0:1]/a](https://[2001:db8::1]/b)", "https://[2001:0db8:0:0:0:0:0:1]/a", "https://[2001:db8::1]/b", "https://[2001:0db8:0:0:0:0:0:1]/a"},
		{"[alice@EXAMPLE.com](mailto:alice@example.com)", "alice@EXAMPLE.com", "mailto:alice@example.com", "alice@EXAMPLE.com"},
		{"[alice@例子.测试](mailto:alice@xn--fsqu00a.xn--0zwm56d)", "alice@例子.测试", "mailto:alice@xn--fsqu00a.xn--0zwm56d", "alice@例子.测试"},
		{"[alice+tag@example.com](mailto:alice%2Btag@example.com?subject=Hello)", "alice+tag@example.com", "mailto:alice%2Btag@example.com?subject=Hello", "alice+tag@example.com"},
		{"[alice#tag@example.com](mailto:alice%23tag@example.com)", "alice#tag@example.com", "mailto:alice%23tag@example.com", "alice#tag@example.com"},
		{"[mailto:alice@example.com](mailto:alice@example.com?subject=Hello)", "mailto:alice@example.com", "mailto:alice@example.com?subject=Hello", "mailto:alice@example.com"},
		{"[\"alice smith\"@example.com](mailto:%22alice%20smith%22@example.com)", "\"alice smith\"@example.com", "mailto:%22alice%20smith%22@example.com", "\"alice smith\"@example.com"},
		{"[\"alice:smith\"@example.com](mailto:%22alice%3Asmith%22@example.com)", "\"alice:smith\"@example.com", "mailto:%22alice%3Asmith%22@example.com", "\"alice:smith\"@example.com"},
		{"[alice@[IPv6:2001:db8::1]](mailto:alice@[IPv6:2001:db8::1])", "alice@[IPv6:2001:db8::1]", "mailto:alice@[IPv6:2001:db8::1]", "alice@[IPv6:2001:db8::1]"},
		{"[alice@example.com,bob@example.com](mailto:bob@example.com,alice@example.com)", "alice@example.com,bob@example.com", "mailto:bob@example.com,alice@example.com", "alice@example.com,bob@example.com"},
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
		{"[alice@trusted.example](mailto:bob@evil.example)", "alice@trusted.example (mailto:bob@evil.example)", "mailto:bob@evil.example"},
		{"[alice@example.com](mailto:bob@example.com)", "alice@example.com (mailto:bob@example.com)", "mailto:bob@example.com"},
		{"[Alice@example.com](mailto:alice@example.com)", "Alice@example.com (mailto:alice@example.com)", "mailto:alice@example.com"},
		{"[alice@例子.测试](mailto:alice@evil.example)", "alice@例子.测试 (mailto:alice@evil.example)", "mailto:alice@evil.example"},
		{"[https://例子.测试](https://evil.example)", "https://例子.测试 (https://evil.example)", "https://evil.example"},
		{"[https://trusted.example](https://例子.测试)", "https://trusted.example (https://例子.测试)", "https://例子.测试"},
		{"[mailto:alice@trusted.example](mailto:bob@evil.example)", "mailto:alice@trusted.example (mailto:bob@evil.example)", "mailto:bob@evil.example"},
		{"[alice@example.com](mailto:alice@example.com?cc=bob@example.com)", "alice@example.com (mailto:alice@example.com?cc=bob@example.com)", "mailto:alice@example.com?cc=bob@example.com"},
		{"[alice@example.com](mailto:alice@example.com?BCC=bob+tag@example.com)", "alice@example.com (mailto:alice@example.com?BCC=bob+tag@example.com)", "mailto:alice@example.com?BCC=bob+tag@example.com"},
		{"[alice@example.com](mailto:alice@example.com?%74o=bob@example.com)", "alice@example.com (mailto:alice@example.com?%74o=bob@example.com)", "mailto:alice@example.com?%74o=bob@example.com"},
		{"[alice@example.com](mailto:alice@example.com#bob@example.com)", "alice@example.com (mailto:alice@example.com#bob@example.com)", "mailto:alice@example.com#bob@example.com"},
		{"[alice@example.com](https://example.com)", "alice@example.com (https://example.com)", "https://example.com"},
		{"[https://example.com](mailto:alice@example.com)", "https://example.com (mailto:alice@example.com)", "mailto:alice@example.com"},
		{"[**https://例子.测试**](https://evil.example)", "https://例子.测试 (https://evil.example)", "https://evil.example"},
		{"![alice@trusted.example](https://evil.example)", "[alice@trusted.example] (https://evil.example)", "https://evil.example"},
		{"[![alice@trusted.example](https://image.example)](https://evil.example)", "[alice@trusted.example] (https://evil.example)", "https://evil.example"},
		{"[write to alice@trusted.example today](mailto:bob@evil.example)", "write to alice@trusted.example today (mailto:bob@evil.example)", "mailto:bob@evil.example"},
		{"[https://](https://example.com)", "https:// (https://example.com)", "https://example.com"},
		{"[https://xn--.example](https://example.com)", "https://xn--.example (https://example.com)", "https://example.com"},
		{"[https://%zz.example](https://example.com)", "https://%zz.example (https://example.com)", "https://example.com"},
		{"[alice@@trusted.example](mailto:bob@evil.example)", "alice@@trusted.example (mailto:bob@evil.example)", "mailto:bob@evil.example"},
		{"[write to !alice@example.com](mailto:alice@example.com)", "write to !alice@example.com (mailto:alice@example.com)", "mailto:alice@example.com"},
		{"[\"https://trusted.example\" <alice@example.com>](mailto:alice@example.com)", "\"https://trusted.example\" alice@example.com (mailto:alice@example.com)", "mailto:alice@example.com"},
		{"[\"alice@trusted.example\" &lt;bob@evil.example&gt;](mailto:bob@evil.example)", "\"alice@trusted.example\" <bob@evil.example> (mailto:bob@evil.example)", "mailto:bob@evil.example"},
		{"[\"alice@trusted.example\": bob@evil.example;](mailto:bob@evil.example)", "\"alice@trusted.example\": bob@evil.example; (mailto:bob@evil.example)", "mailto:bob@evil.example"},
		{"[bob@evil.example (alice@trusted.example)](mailto:bob@evil.example)", "bob@evil.example (alice@trusted.example) (mailto:bob@evil.example)", "mailto:bob@evil.example"},
	} {
		for _, look := range []markdown.Look{{}, {Target: grid.Style{Attr: grid.Dim}}} {
			shown, links := linkedText(t, tc.source, look)
			want := " (" + tc.destination + ")"
			if shown != tc.shown || len(links) != 1 || links[tc.destination] != want {
				t.Errorf("%s rendered %q with %v, want %q where only %q links", tc.source, shown, links, tc.shown, want)
			}
		}
	}
}

func TestMailtoQueryRecipientsPreserveTheOpeningPolicy(t *testing.T) {
	for _, tc := range []struct{ source, shown string }{
		{"[alice@example.com](mailto:?to=alice@example.com)", "alice@example.com"},
		{"[alice+tag@example.com](mailto:?to=alice+tag@example.com)", "alice+tag@example.com"},
		{"[alice@example.com](mailto:?to=bob@example.com)", "alice@example.com (mailto:?to=bob@example.com)"},
	} {
		shown, links := linkedText(t, tc.source, markdown.Look{})
		if shown != tc.shown || len(links) != 0 {
			t.Errorf("%s rendered %q with %v, want %q without clickable destinations", tc.source, shown, links, tc.shown)
		}
	}
}

func TestLinkPolicyUsesTheVisibleLabel(t *testing.T) {
	const destination = "https://evil.example"
	for _, tc := range []struct{ name, source, shown string }{
		{"control", "[ht\x01tps://trusted.example](https://evil.example)", "https://trusted.example"},
		{"entity", "[ht&#1;tps://trusted.example](https://evil.example)", "https://trusted.example"},
		{"C1 entity", "[ww&#128;w.trusted.example](https://evil.example)", "www.trusted.example"},
		{"spacing control", "[ht&#x85;tps://trusted.example](https://evil.example)", "https://trusted.example"},
		{"styled", "[h**t&#x7f;tps://trusted.example**](https://evil.example)", "https://trusted.example"},
		{"discarded joiner", "[ht\x01\u200dtps://trusted.example](https://evil.example)", "https://trusted.example"},
		{"joiner across style", "[ht&#1;*\u200dtps://trusted.example*](https://evil.example)", "https://trusted.example"},
		{"retained joiner", "[https://trus\u200dted.example](https://evil.example)", "https://trus\u200dted.example"},
		{"retained bidi", "[https://trus\u2066ted.example\u2069](https://evil.example)", "https://trus\u2066ted.example\u2069"},
		{"image", "![ht&#1;tps://trusted.example](https://evil.example)", "[https://trusted.example]"},
		{"code", "[`ht\x01tps://trusted.example`](https://evil.example)", "https://trusted.example"},
		{"reference", "[ht&#1;tps://trusted.example][ref]\n\n[ref]: https://evil.example", "https://trusted.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, look := range []markdown.Look{{}, {Target: grid.Style{Attr: grid.Dim}}, look()} {
				shown, links := linkedText(t, tc.source, look)
				address := " (" + destination + ")"
				if shown != tc.shown+address || len(links) != 1 || links[destination] != address {
					t.Errorf("rendered %q with %v, want %q where only %q links", shown, links, tc.shown+address, address)
				}
			}
		})
	}
}
