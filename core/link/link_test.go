package link_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/link"
)

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
			got := link.Detect(tc.in)
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
	got := link.Detect(doc)
	if len(got) != 1 {
		t.Fatalf("found %d links, want 1", len(got))
	}
	if text := got[0].Text(doc); text != "https://example.com" {
		t.Fatalf("matched %q, want the url without the prose after it", text)
	}
}

func TestTextButtedDirectlyAgainstAUrlWithNoSpace(t *testing.T) {
	const doc = "自见https://example.com页面"
	got := link.Detect(doc)
	if len(got) != 1 {
		t.Fatalf("found %d links, want 1", len(got))
	}
	if text := got[0].Text(doc); text != "https://example.com" {
		t.Fatalf("matched %q, want the url alone", text)
	}
}

func TestTextReportsWhatWasWritten(t *testing.T) {
	const doc = "go to www.example.com now"
	got := link.Detect(doc)
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
	links := link.Detect(s)
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
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		links := link.DetectIn(s, func(string) bool { return true })
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
