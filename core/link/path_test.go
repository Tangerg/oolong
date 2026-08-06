package link_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/link"
)

// files answers as a filesystem holding exactly these paths would.
func files(paths ...string) func(string) bool {
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[p] = true
	}
	return func(p string) bool { return set[p] }
}

func targets(links []link.Link) []string {
	out := make([]string, len(links))
	for i, l := range links {
		out[i] = l.Target
	}
	return out
}

// TestPathsWhoseShapeIsEvidenceEnough are the ones found without asking anything: a
// leading slash, a home marker, or a directory part with an extension after it.
func TestPathsWhoseShapeIsEvidenceEnough(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"see /usr/local/bin/go for it", "/usr/local/bin/go"},
		{"in ~/.config/oolong/settings.toml", "~/.config/oolong/settings.toml"},
		{"edited src/main.go today", "src/main.go"},
		{"under .grok/sessions/x.json", ".grok/sessions/x.json"},
		{"the file images/1.png", "images/1.png"},
		{"an encoded one /tmp/%2Fa/b.txt", "/tmp/%2Fa/b.txt"},
		{"a directory /var/log/", "/var/log/"},
	} {
		got := link.Detect(tc.in)
		if len(got) != 1 {
			t.Errorf("%q found %v, want one path", tc.in, targets(got))
			continue
		}
		if got[0].Kind != link.File {
			t.Errorf("%q found a %v, want a file", tc.in, got[0].Kind)
		}
		if got[0].Target != tc.want {
			t.Errorf("%q found %q, want %q", tc.in, got[0].Target, tc.want)
		}
		if text := got[0].Text(tc.in); !strings.Contains(tc.in, text) {
			t.Errorf("%q reported the range %q, which is not in it", tc.in, text)
		}
	}
}

// TestSlashedProseIsNotAPath. The extension is what keeps it out, which is the whole
// reason a relative path needs one.
func TestSlashedProseIsNotAPath(t *testing.T) {
	for _, in := range []string{
		"it works and/or it does not",
		"over TCP/IP",
		"open 24/7",
		"either his/her copy",
		"a fraction 1/2 of it",
	} {
		if got := link.Detect(in); len(got) != 0 {
			t.Errorf("%q found %v", in, targets(got))
		}
	}
}

// TestABareNameNeedsConfirming. "node.js" is a file in one sentence and a runtime in
// the next, and nothing in the text says which.
func TestABareNameNeedsConfirming(t *testing.T) {
	const in = "I edited main.go and node.js runs it, version 1.2.3"

	if got := link.Detect(in); len(got) != 0 {
		t.Errorf("without a filesystem it found %v, want nothing", targets(got))
	}

	got := link.DetectIn(in, files("main.go"))
	if len(got) != 1 {
		t.Fatalf("found %v, want only the file that is there", targets(got))
	}
	if got[0].Target != "main.go" || got[0].Kind != link.File {
		t.Errorf("found %+v", got[0])
	}
}

// TestARelativePathIsConfirmedWhenThereIsAnythingToConfirmItWith, so an agent's own
// output does not underline a path from somebody else's machine.
func TestARelativePathIsConfirmedWhenThereIsAnythingToConfirmItWith(t *testing.T) {
	const in = "changed src/main.go and their/other.go"
	got := link.DetectIn(in, files("src/main.go"))
	if len(got) != 1 || got[0].Target != "src/main.go" {
		t.Errorf("found %v, want only the one that is there", targets(got))
	}
}

// TestARootedPathIsNotAskedAbout: it says where it is, and a reference to a file that
// has not been written yet is still a reference to that file.
func TestARootedPathIsNotAskedAbout(t *testing.T) {
	got := link.DetectIn("will write /tmp/output.txt", files())
	if len(got) != 1 || got[0].Target != "/tmp/output.txt" {
		t.Errorf("found %v, want the rooted path", targets(got))
	}
}

// TestAQuotedPathKeepsItsSpaces, which is the only form where a space may appear
// anywhere and what makes an application bundle one path rather than two words.
func TestAQuotedPathKeepsItsSpaces(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`open "/Applications/Demo App.app"`, "/Applications/Demo App.app"},
		{`see 'my notes/todo list.md'`, "my notes/todo list.md"},
	} {
		got := link.Detect(tc.in)
		if len(got) != 1 {
			t.Errorf("%q found %v", tc.in, targets(got))
			continue
		}
		if got[0].Target != tc.want {
			t.Errorf("%q found %q, want %q", tc.in, got[0].Target, tc.want)
		}
		// The quotes are part of what was matched, so the underline covers them.
		if text := got[0].Text(tc.in); !strings.HasPrefix(text, `"`) && !strings.HasPrefix(text, `'`) {
			t.Errorf("%q underlined %q, which leaves the quote out", tc.in, text)
		}
	}
}

// TestALineAndColumnBelongToTheLink. "src/main.go:42" is what a compiler, a stack
// trace and a model all write, and opening the file at the top throws away the useful
// half of it.
func TestALineAndColumnBelongToTheLink(t *testing.T) {
	for _, tc := range []struct {
		in           string
		target       string
		line, column int
	}{
		{"at src/main.go:42", "src/main.go", 42, 0},
		{"at src/main.go:42:7", "src/main.go", 42, 7},
		{"at /a/b.go:1:1 exactly", "/a/b.go", 1, 1},
		{"no line here src/main.go", "src/main.go", 0, 0},
	} {
		got := link.Detect(tc.in)
		if len(got) != 1 {
			t.Errorf("%q found %v", tc.in, targets(got))
			continue
		}
		if got[0].Target != tc.target || got[0].Line != tc.line || got[0].Column != tc.column {
			t.Errorf("%q found %+v, want %q at %d:%d", tc.in, got[0], tc.target, tc.line, tc.column)
		}
		// And the underline covers the line number, because it is part of what the
		// reader would click.
		if tc.line != 0 && !strings.Contains(got[0].Text(tc.in), ":") {
			t.Errorf("%q underlined %q, which leaves the line out", tc.in, got[0].Text(tc.in))
		}
	}
}

// TestAURLIsNotAlsoAPath, or a link would be stamped twice and whichever ran last
// would win.
func TestAURLIsNotAlsoAPath(t *testing.T) {
	got := link.Detect("see https://example.com/a/b.html for it")
	if len(got) != 1 {
		t.Fatalf("found %v, want one link", targets(got))
	}
	if got[0].Kind != link.URL {
		t.Errorf("found a %v, want a url", got[0].Kind)
	}
}

func TestLinksComeBackInOrderAndDoNotOverlap(t *testing.T) {
	got := link.Detect("see /a/b.go then https://x.test/y then ~/c/d.txt")
	if len(got) != 3 {
		t.Fatalf("found %v, want three", targets(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Start < got[i-1].End {
			t.Errorf("link %d starts at %d, inside the one ending at %d", i, got[i].Start, got[i-1].End)
		}
	}
}

func TestKindNames(t *testing.T) {
	if got := link.URL.String(); got != "url" {
		t.Errorf("= %q", got)
	}
	if got := link.File.String(); got != "file" {
		t.Errorf("= %q", got)
	}
}

func TestDetectFindsNothingInNothing(t *testing.T) {
	for _, in := range []string{"", "   ", "/", "~/", "just words"} {
		if got := link.Detect(in); len(got) != 0 {
			t.Errorf("%q found %v", in, targets(got))
		}
	}
}

// TestPathsThatAreTheTailOfALongerWord. A slash appears in the middle of prose and a
// regular expression cannot look backwards, so the check is made by hand — and getting
// it wrong is what turned "and/or" into the path "/or".
func TestPathsThatAreTheTailOfALongerWord(t *testing.T) {
	for _, in := range []string{
		"and/or",
		"x/tmp/a.go",
		"foo~/bar.go",
		"a.b/c.go",
	} {
		for _, l := range link.Detect(in) {
			if l.Start != 0 {
				t.Errorf("%q found %q starting at %d, in the middle of a word", in, l.Target, l.Start)
			}
		}
	}
	// At the very start of the text there is nothing before it, which is the case the
	// check has to allow.
	if got := link.Detect("/usr/bin/go is there"); len(got) != 1 {
		t.Errorf("a path at the start of the text found %v", targets(got))
	}
}

func TestABareNameIsNotFoundInTheMiddleOfOne(t *testing.T) {
	// The same rule, on the branch that only the filesystem can decide.
	got := link.DetectIn("nonsense.go", files("sense.go"))
	if len(got) != 0 {
		t.Errorf("found %v inside a longer word", targets(got))
	}
}

func TestTrailingPunctuationLeavesThePath(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"look at /tmp/a.go.", "/tmp/a.go"},
		{"look at /tmp/a.go,", "/tmp/a.go"},
		{"(see /tmp/a.go)", "/tmp/a.go"},
	} {
		got := link.Detect(tc.in)
		if len(got) != 1 || got[0].Target != tc.want {
			t.Errorf("%q found %v, want %q", tc.in, targets(got), tc.want)
		}
	}
}

func TestAPathThatIsOnlyPunctuationIsNotAPath(t *testing.T) {
	for _, in := range []string{"a / b", "~/", "// a comment"} {
		for _, l := range link.Detect(in) {
			if l.Target == "/" || l.Target == "~/" || l.Target == "" {
				t.Errorf("%q found the empty path %q", in, l.Target)
			}
		}
	}
}

// TestOverlappingShapesKeepTheFirst, because two links over the same cells would stamp
// one target on top of the other.
func TestOverlappingShapesKeepTheFirst(t *testing.T) {
	// The bare branch would find "b.go" inside the relative path the other branch
	// already found.
	got := link.DetectIn("in src/b.go now", files("src/b.go", "b.go"))
	if len(got) != 1 || got[0].Target != "src/b.go" {
		t.Errorf("found %v, want only the whole path", targets(got))
	}
}
