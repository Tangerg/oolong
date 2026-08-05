package link_test

import (
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
			want: []link.Link{{Start: 4, End: 23, URL: "https://example.com"}},
		},
		{
			name: "a bare host is given the scheme it was written without",
			in:   "www.example.com",
			want: []link.Link{{Start: 0, End: 15, URL: "https://www.example.com"}},
		},
		{
			name: "www inside a longer word is not a host",
			in:   "awww.example.com",
			want: nil,
		},
		{
			name: "the sentence's full stop is not part of the url",
			in:   "read https://example.com/docs.",
			want: []link.Link{{Start: 5, End: 29, URL: "https://example.com/docs"}},
		},
		{
			name: "a run of punctuation is trimmed",
			in:   "wow https://example.com/a?q=1!?;,",
			want: []link.Link{{Start: 4, End: 29, URL: "https://example.com/a?q=1"}},
		},
		{
			name: "prose parentheses are not part of the url",
			in:   "(see https://example.com/path)",
			want: []link.Link{{Start: 5, End: 29, URL: "https://example.com/path"}},
		},
		{
			name: "a parenthesis the path opened is part of the url",
			in:   "https://en.wikipedia.org/wiki/Go_(language)",
			want: []link.Link{{Start: 0, End: 43, URL: "https://en.wikipedia.org/wiki/Go_(language)"}},
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
				{Start: 0, End: 16, URL: "http://a.example"},
				{Start: 21, End: 37, URL: "http://b.example"},
			},
		},
		{
			name: "a www host inside a full url is not reported twice",
			in:   "https://www.example.com/x",
			want: []link.Link{{Start: 0, End: 25, URL: "https://www.example.com/x"}},
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
	if got[0].URL != "https://www.example.com" {
		t.Fatalf("url = %q, want the scheme filled in", got[0].URL)
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
		{"the first byte of the first", links[0].Start, links[0].URL},
		{"inside the first", links[0].Start + 5, links[0].URL},
		{"the last byte of the first", links[0].End - 1, links[0].URL},
		// End is exclusive: the space after a URL is not part of it, or a click
		// beside a link opens it.
		{"just past the first", links[0].End, ""},
		{"inside the second", links[1].Start + 2, links[1].URL},
		{"past everything", len(s) - 1, ""},
		{"negative", -1, ""},
	} {
		got, ok := link.At(links, tc.offset)
		switch {
		case tc.want == "" && ok:
			t.Errorf("%s: found %q", tc.name, got.URL)
		case tc.want != "" && !ok:
			t.Errorf("%s: found nothing, want %q", tc.name, tc.want)
		case tc.want != "" && got.URL != tc.want:
			t.Errorf("%s: found %q, want %q", tc.name, got.URL, tc.want)
		}
	}
}

func TestMapAnswersAClick(t *testing.T) {
	var m link.Map
	m.Add(4, 1, 19, "https://example.com")
	m.Add(2, 3, 5, "https://other.test")

	for _, tc := range []struct {
		name string
		x, y int
		want string
	}{
		{"the first column of a run", 4, 1, "https://example.com"},
		{"the middle of a run", 10, 1, "https://example.com"},
		{"the last column of a run", 22, 1, "https://example.com"},
		{"one column past a run", 23, 1, ""},
		{"one column before a run", 3, 1, ""},
		{"the right column on the wrong row", 4, 0, ""},
		{"the other run", 3, 3, "https://other.test"},
		{"negative", -1, -1, ""},
	} {
		got, ok := m.At(tc.x, tc.y)
		if tc.want == "" && ok {
			t.Errorf("%s: found %q", tc.name, got)
		}
		if tc.want != "" && got != tc.want {
			t.Errorf("%s: found %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestMapIgnoresRunsWithNothingInThem(t *testing.T) {
	// What a link scrolled off the edge comes to, and what a caller passes when it
	// has no target.
	var m link.Map
	m.Add(0, 0, 0, "https://example.com")
	m.Add(0, 0, -3, "https://example.com")
	m.Add(0, 0, 5, "")
	if m.Len() != 0 {
		t.Errorf("the map holds %d runs, want none", m.Len())
	}
}

func TestMapPrefersWhatWasDrawnLast(t *testing.T) {
	// Overlapping draws mean something was drawn over something else, and a click
	// lands on what is in front.
	var m link.Map
	m.Add(0, 0, 10, "https://behind.test")
	m.Add(3, 0, 4, "https://in-front.test")
	if got, _ := m.At(4, 0); got != "https://in-front.test" {
		t.Errorf("got %q, want the run drawn last", got)
	}
	if got, _ := m.At(8, 0); got != "https://behind.test" {
		t.Errorf("outside the overlap: got %q", got)
	}
}

func TestMapResetKeepsItsRoom(t *testing.T) {
	var m link.Map
	for i := range 10 {
		m.Add(0, i, 3, "https://example.com")
	}
	m.Reset()
	if m.Len() != 0 {
		t.Errorf("the map holds %d runs after a reset", m.Len())
	}
	if _, ok := m.At(1, 1); ok {
		t.Error("a click was answered from a map that was reset")
	}
}
