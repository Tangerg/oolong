package link_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/link"
)

func TestQuotedURLsKeepTheirSemanticKind(t *testing.T) {
	for _, quote := range []string{"\"", "'"} {
		for _, target := range []string{"https://example.com", "http://example.com/a.go", "www.example.com"} {
			s := quote + target + quote
			for _, exists := range []func(string) bool{nil, func(string) bool { return true }, func(string) bool { return false }} {
				links := link.Detect(s, exists)
				want := target
				if strings.HasPrefix(target, "www.") {
					want = "https://" + target
				}
				if len(links) != 1 || links[0].Kind != link.URL || links[0].Target != want || links[0].Line != 0 || links[0].Column != 0 {
					t.Fatalf("Detect(%q) = %+v", s, links)
				}
			}
		}
	}
}

func TestURLApostrophesRemainInsideTheDestination(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"https://example.com/O'Brien", "https://example.com/O'Brien"},
		{"https://example.com/?q=O'Brien", "https://example.com/?q=O'Brien"},
		{"'https://example.com/O'Brien'", "https://example.com/O'Brien"},
		{"'https://example.com/O'Brien'.", "https://example.com/O'Brien"},
		{"\"https://example.com/O'Brien\"", "https://example.com/O'Brien"},
		{"https://example.com/O'Brien页面", "https://example.com/O'Brien"},
		{"(https://example.com/O'Brien_(name)).", "https://example.com/O'Brien_(name)"},
		{"https://example.com/quote'", "https://example.com/quote'"},
		{"'https://example.com/path?'", "https://example.com/path?"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			found := link.Detect(tc.input, nil)
			if len(found) != 1 || found[0].Kind != link.URL || found[0].Target != tc.want {
				t.Fatalf("Detect(%q) = %+v; want %q", tc.input, found, tc.want)
			}
			if matched := found[0].Text(tc.input); matched != tc.want {
				t.Fatalf("matched %q; want %q", matched, tc.want)
			}
		})
	}
}

func TestQuotedURLBoundariesKeepFollowingTextSeparate(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{"'https://a.test/one','https://b.test/two'", []string{"https://a.test/one", "https://b.test/two"}},
		{"'https://a.test/one';'https://b.test/two'", []string{"https://a.test/one", "https://b.test/two"}},
		{"'https://example.com/O'Brien',next", []string{"https://example.com/O'Brien"}},
		{"'https://example.com/O'Brien';next", []string{"https://example.com/O'Brien"}},
		{"'(https://example.com/O'Brien)'", []string{"https://example.com/O'Brien"}},
		{"'[https://example.com/O'Brien]'", []string{"https://example.com/O'Brien"}},
		{"\"(https://example.com/O'Brien)\"", []string{"https://example.com/O'Brien"}},
		{"'see https://example.com/O'Brien',then 'https://b.test/two'", []string{"https://example.com/O'Brien", "https://b.test/two"}},
		{"O'Brien recommends 'https://example.com/O'Brien',next", []string{"https://example.com/O'Brien"}},
		{"don't truncate https://example.com/quote'", []string{"https://example.com/quote'"}},
		{"users' links include https://example.com/quote'", []string{"https://example.com/quote'"}},
		{"https://example.com/?q='value'&next=2", []string{"https://example.com/?q='value'&next=2"}},
		{"https://example.com/a'/'b", []string{"https://example.com/a'/'b"}},
	} {
		t.Run(tc.input, func(t *testing.T) {
			found := link.Detect(tc.input, nil)
			if len(found) != len(tc.want) {
				t.Fatalf("Detect(%q) = %+v; want %q", tc.input, found, tc.want)
			}
			for i, target := range tc.want {
				if found[i].Kind != link.URL || found[i].Target != target || found[i].Text(tc.input) != target {
					t.Fatalf("link %d = %+v (%q); want %q", i, found[i], found[i].Text(tc.input), target)
				}
			}
		})
	}
}

func TestQuotedURLsPreserveTheirFinalURIPunctuation(t *testing.T) {
	for _, quote := range []string{"'", "\""} {
		for _, punctuation := range []string{"?", "!", ".", ",", ";", ":"} {
			target := "https://example.com/path" + punctuation
			input := quote + target + quote + ",next"
			found := link.Detect(input, nil)
			if len(found) != 1 || found[0].Kind != link.URL || found[0].Target != target || found[0].Text(input) != target {
				t.Errorf("Detect(%q) = %+v; want %q", input, found, target)
			}
		}
	}
}

func TestDetect(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want []link.Link
	}{
		{
			name: "nothing in it",
			in:   "no links here",
		},
		{
			name: "plain https",
			in:   "see https://example.com for details",
			want: []link.Link{{Start: 4, End: 23, Target: "https://example.com"}},
		},
		{
			name: "a bare host is given the scheme it was written without",
			in:   "www.example.com",
			want: []link.Link{{Start: 0, End: 15, Target: "https://www.example.com"}},
		},
		{
			name: "www inside a longer word is not a host",
			in:   "awww.example.com",
			want: nil,
		},
		{
			name: "the sentence's full stop is not part of the url",
			in:   "read https://example.com/docs.",
			want: []link.Link{{Start: 5, End: 29, Target: "https://example.com/docs"}},
		},
		{
			name: "a run of punctuation is trimmed",
			in:   "wow https://example.com/a?q=1!?;,",
			want: []link.Link{{Start: 4, End: 29, Target: "https://example.com/a?q=1"}},
		},
		{
			name: "prose parentheses are not part of the url",
			in:   "(see https://example.com/path)",
			want: []link.Link{{Start: 5, End: 29, Target: "https://example.com/path"}},
		},
		{
			name: "a parenthesis the path opened is part of the url",
			in:   "https://en.wikipedia.org/wiki/Go_(language)",
			want: []link.Link{{Start: 0, End: 43, Target: "https://en.wikipedia.org/wiki/Go_(language)"}},
		},
		{
			name: "keep the IPv6 host bracket, trim the prose bracket",
			in:   "[http://[::1]].",
			want: []link.Link{{Start: 1, End: 13, Target: "http://[::1]"}},
		},
		{
			name: "a scheme with nothing after it is not a link",
			in:   "the scheme is https://",
			want: nil,
		},
		{
			name: "two of them",
			in:   "http://a.example and http://b.example",
			want: []link.Link{
				{Start: 0, End: 16, Target: "http://a.example"},
				{Start: 21, End: 37, Target: "http://b.example"},
			},
		},
		{
			name: "a www host inside a full url is not reported twice",
			in:   "https://www.example.com/x",
			want: []link.Link{{Start: 0, End: 25, Target: "https://www.example.com/x"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := link.Detect(tc.in, nil)
			if len(got) != len(tc.want) {
				t.Fatalf("found %d links %+v, want %d %+v", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("link %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestTextButtedAgainstAUrlIsNotSwallowed(t *testing.T) {
	// The normal case in Chinese and Japanese prose, and the reason the body is the
	// URI character set rather than "anything that is not a space".
	const doc = "见 https://example.com 页面"
	got := link.Detect(doc, nil)
	if len(got) != 1 {
		t.Fatalf("found %d links, want 1", len(got))
	}
	if text := got[0].Text(doc); text != "https://example.com" {
		t.Fatalf("matched %q, want the url without the prose after it", text)
	}
}

func TestTextButtedDirectlyAgainstAUrlWithNoSpace(t *testing.T) {
	const doc = "自见https://example.com页面"
	got := link.Detect(doc, nil)
	if len(got) != 1 {
		t.Fatalf("found %d links, want 1", len(got))
	}
	if text := got[0].Text(doc); text != "https://example.com" {
		t.Fatalf("matched %q, want the url alone", text)
	}
}

func TestTextReportsWhatWasWritten(t *testing.T) {
	const doc = "go to www.example.com now"
	got := link.Detect(doc, nil)
	if len(got) != 1 {
		t.Fatalf("found %d links, want 1", len(got))
	}
	if text := got[0].Text(doc); text != "www.example.com" {
		t.Fatalf("text = %q, want what was written", text)
	}
	if got[0].Target != "https://www.example.com" {
		t.Fatalf("url = %q, want the scheme filled in", got[0].Target)
	}
}

func TestTextOutsideTheStringIsEmptyRatherThanAPanic(t *testing.T) {
	if got := (link.Link{Start: 0, End: 99}).Text("short"); got != "" {
		t.Fatalf("= %q, want nothing", got)
	}
	if got := (link.Link{Start: 3, End: 1}).Text("short"); got != "" {
		t.Fatalf("a backwards range = %q, want nothing", got)
	}
}

func TestAt(t *testing.T) {
	const s = "see https://example.com and http://other.test now"
	links := link.Detect(s, nil)
	if len(links) != 2 {
		t.Fatalf("found %d links in %q", len(links), s)
	}

	for _, tc := range []struct {
		name   string
		offset int
		want   string
	}{
		{"before anything", 0, ""},
		{"the first byte of the first", links[0].Start, links[0].Target},
		{"inside the first", links[0].Start + 5, links[0].Target},
		{"the last byte of the first", links[0].End - 1, links[0].Target},
		// End is exclusive: the space after a URL is not part of it, or a click
		// beside a link opens it.
		{"just past the first", links[0].End, ""},
		{"inside the second", links[1].Start + 2, links[1].Target},
		{"past everything", len(s) - 1, ""},
		{"negative", -1, ""},
	} {
		got, ok := links.At(tc.offset)
		switch {
		case tc.want == "" && ok:
			t.Errorf("%s: found %q", tc.name, got.Target)
		case tc.want != "" && !ok:
			t.Errorf("%s: found nothing, want %q", tc.name, tc.want)
		case tc.want != "" && got.Target != tc.want:
			t.Errorf("%s: found %q, want %q", tc.name, got.Target, tc.want)
		}
	}
}

func FuzzDetectNeverPanicsAndStaysInside(f *testing.F) {
	// Every one of these came out of a model, a tool, or a terminal, and the ranges go
	// straight onto cells.
	for _, seed := range []string{
		"", "https://example.com", "www.example.com/a", "see /usr/bin/go",
		"src/main.go:42:7", `"Demo App.app"`, "and/or", "~/x/y.txt",
		"https://a.test/b.html and /c/d.go", "node.js", "//////", "~~~~", ".....",
		"a" + strings.Repeat("/b", 200), strings.Repeat(".", 300),
		"'https://a.test/one','https://b.test/two'",
		"'https://example.com/O'Brien',next", "'(https://example.com/O'Brien)'",
		"'https://example.com/path?'", "https://example.com/?q='value'&next=2",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		links := link.Detect(s, func(string) bool { return true })
		end := 0
		for _, l := range links {
			if l.Start < 0 || l.End > len(s) || l.Start >= l.End {
				t.Fatalf("range [%d,%d) in a string of %d bytes", l.Start, l.End, len(s))
			}
			if l.Start < end {
				t.Fatalf("link at %d overlaps the one ending at %d", l.Start, end)
			}
			end = l.End
			if l.Target == "" {
				t.Fatalf("a link at [%d,%d) points nowhere", l.Start, l.End)
			}
			if l.Line < 0 || l.Column < 0 {
				t.Fatalf("a link points at line %d column %d", l.Line, l.Column)
			}
			if l.Kind != link.URL && l.Kind != link.File {
				t.Fatalf("a link of kind %d", l.Kind)
			}
		}
	})
}
