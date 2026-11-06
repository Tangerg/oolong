package link_test

import (
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/link"
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
		{"edit view/main.storyboard", "view/main.storyboard"},
		{"an encoded one /tmp/%2Fa/b.txt", "/tmp/%2Fa/b.txt"},
		{"a directory /var/log/", "/var/log/"},
	} {
		got := link.Detect(tc.in, nil)
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
		if got := link.Detect(in, nil); len(got) != 0 {
			t.Errorf("%q found %v", in, targets(got))
		}
	}
}

// TestABareNameNeedsConfirming. "node.js" is a file in one sentence and a runtime in
// the next, and nothing in the text says which.
func TestABareNameNeedsConfirming(t *testing.T) {
	const in = "I edited main.go and node.js runs it, version 1.2.3"

	if got := link.Detect(in, nil); len(got) != 0 {
		t.Errorf("without a filesystem it found %v, want nothing", targets(got))
	}

	got := link.Detect(in, files("main.go"))
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
	got := link.Detect(in, files("src/main.go"))
	if len(got) != 1 || got[0].Target != "src/main.go" {
		t.Errorf("found %v, want only the one that is there", targets(got))
	}
}

// TestARootedPathIsNotAskedAbout: it says where it is, and a reference to a file that
// has not been written yet is still a reference to that file.
func TestARootedPathIsNotAskedAbout(t *testing.T) {
	got := link.Detect("will write /tmp/output.txt", files())
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
		got := link.Detect(tc.in, nil)
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
		got := link.Detect(tc.in, nil)
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

func TestFilePositionsConsumeEveryDigitBeforeValidation(t *testing.T) {
	for _, tc := range []struct {
		suffix       string
		line, column int
	}{
		{suffix: ":0000000012", line: 12},
		{suffix: ":42:0000000034", line: 42, column: 34},
		{suffix: ":1000000000:7", line: 1_000_000_000, column: 7},
		{suffix: ":" + strings.Repeat("0", 100) + "12:00034", line: 12, column: 34},
		{suffix: ":" + strconv.Itoa(math.MaxInt) + ":" + strconv.Itoa(math.MaxInt), line: math.MaxInt, column: math.MaxInt},
	} {
		const path = "src/main.go"
		const prefix = "at "
		source := prefix + path + tc.suffix + " next"
		got := link.Detect(source, nil)
		want := link.Link{Start: len(prefix), End: len(prefix + path + tc.suffix), Kind: link.File, Target: path, Line: tc.line, Column: tc.column}
		if len(got) != 1 || got[0] != want {
			t.Errorf("suffix %q = %+v, want %+v", tc.suffix, got, want)
		}
	}
}

func TestInvalidFilePositionsDoNotPublishAPartialLocation(t *testing.T) {
	overflow := strconv.FormatUint(uint64(math.MaxInt)+1, 10)
	for _, suffix := range []string{
		":0", ":000", ":0:7", ":42:0", ":42:000",
		":" + overflow, ":" + overflow + ":7", ":42:" + overflow,
		":" + strings.Repeat("9", 100), ":42:" + strings.Repeat("9", 100),
	} {
		const path = "src/main.go"
		got := link.Detect(path+suffix, nil)
		want := link.Link{End: len(path), Kind: link.File, Target: path}
		if len(got) != 1 || got[0] != want {
			t.Errorf("invalid suffix %q = %+v, want the path without a location", suffix, got)
		}
	}
}

func TestFilePositionsPreserveQuotedPathsAndAdjacentPunctuation(t *testing.T) {
	for _, tc := range []struct {
		source, text, target string
		line, column         int
	}{
		{source: `"my notes/main.go":0000000012:0000000034)`, text: `"my notes/main.go":0000000012:0000000034`, target: "my notes/main.go", line: 12, column: 34},
		{source: "src/main.go:42: error", text: "src/main.go:42", target: "src/main.go", line: 42},
		{source: "src/main.go:42,", text: "src/main.go:42", target: "src/main.go", line: 42},
		{source: "src/main.go: error", text: "src/main.go", target: "src/main.go"},
		{source: "main.go:0000000012:0000000034", text: "main.go:0000000012:0000000034", target: "main.go", line: 12, column: 34},
	} {
		got := link.Detect(tc.source, files(tc.target))
		if len(got) != 1 || got[0].Target != tc.target || got[0].Line != tc.line || got[0].Column != tc.column || got[0].Text(tc.source) != tc.text {
			t.Errorf("%q = %+v, want %q at %d:%d", tc.source, got, tc.text, tc.line, tc.column)
		}
	}
}

// TestAURLIsNotAlsoAPath, or a link would be stamped twice and whichever ran last
// would win.
func TestAURLIsNotAlsoAPath(t *testing.T) {
	got := link.Detect("see https://example.com/a/b.html for it", nil)
	if len(got) != 1 {
		t.Fatalf("found %v, want one link", targets(got))
	}
	if got[0].Kind != link.URL {
		t.Errorf("found a %v, want a url", got[0].Kind)
	}
}

func TestLinksComeBackInOrderAndDoNotOverlap(t *testing.T) {
	got := link.Detect("see /a/b.go then https://x.test/y then ~/c/d.txt", nil)
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
		if got := link.Detect(in, nil); len(got) != 0 {
			t.Errorf("%q found %v", in, targets(got))
		}
	}
}

func TestPathsThatAreTheTailOfALongerWord(t *testing.T) {
	for _, in := range []string{
		"and/or",
		"x/tmp/a.go",
		"foo~/bar.go",
		"a.b/c.go",
	} {
		for _, l := range link.Detect(in, nil) {
			if l.Start != 0 {
				t.Errorf("%q found %q starting at %d, in the middle of a word", in, l.Target, l.Start)
			}
		}
	}
	// At the very start of the text there is nothing before it, which is the case the
	// check has to allow.
	if got := link.Detect("/usr/bin/go is there", nil); len(got) != 1 {
		t.Errorf("a path at the start of the text found %v", targets(got))
	}
}

func TestABareNameIsNotFoundInTheMiddleOfOne(t *testing.T) {
	// The same rule, on the branch that only the filesystem can decide.
	got := link.Detect("nonsense.go", files("sense.go"))
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
		got := link.Detect(tc.in, nil)
		if len(got) != 1 || got[0].Target != tc.want {
			t.Errorf("%q found %v, want %q", tc.in, targets(got), tc.want)
		}
	}
}

func TestAPathThatIsOnlyPunctuationIsNotAPath(t *testing.T) {
	for _, in := range []string{"a / b", "~/", "// a comment"} {
		for _, l := range link.Detect(in, nil) {
			if l.Target == "/" || l.Target == "~/" || l.Target == "" {
				t.Errorf("%q found the empty path %q", in, l.Target)
			}
		}
	}
}

func TestARelativePathDoesNotAlsoPublishItsBareFilename(t *testing.T) {
	got := link.Detect("in src/b.go now", files("src/b.go", "b.go"))
	if len(got) != 1 || got[0].Target != "src/b.go" {
		t.Errorf("found %v, want only the whole path", targets(got))
	}
}

func TestURLAndItsPathNeverTriggerFilesystemQueries(t *testing.T) {
	input := "www.example.com/src/main.go?file=secret.go then local.go"
	var queried []string
	found := link.Detect(input, func(path string) bool {
		queried = append(queried, path)
		return true
	})
	if len(found) != 2 || found[0].Kind != link.URL || found[1].Target != "local.go" {
		t.Fatalf("links = %+v", found)
	}
	if len(queried) != 1 || queried[0] != "local.go" {
		t.Fatalf("filesystem queried for %q", queried)
	}
}

func TestBareLinkChecksCompleteExtension(t *testing.T) {
	const name = "settings.entitlements"
	got := link.Detect(name, files(name))
	if len(got) != 1 || got[0].Target != name || got[0].Text(name) != name {
		t.Fatalf("links = %+v, want %q", got, name)
	}
}

func TestUnicodePathsKeepTheirWholeIdentityAndOriginalByteRange(t *testing.T) {
	for _, target := range []string{
		"/tmp/报告.go", "/tmp/目录/file.go", "/tmp/café.go", "/tmp/cafe\u0301.go",
		"/tmp/👩‍💻.go", "/tmp/🇯🇵/报告.go", "/tmp/中·文.go", "~/目录/报告.go", "目录/报告.go",
		"/tmp/🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F.go", "/tmp/\ue000.go",
	} {
		for _, quoted := range []bool{false, true} {
			reference := target
			if quoted {
				reference = `"` + target + `"`
			}
			reference += ":42:7"
			prefix := "见 "
			source := prefix + reference + "。"
			found := link.Detect(source, nil)
			if len(found) != 1 || found[0].Kind != link.File || found[0].Target != target || found[0].Line != 42 || found[0].Column != 7 {
				t.Fatalf("Detect(%q) = %+v; want complete file %q at 42:7", source, found, target)
			}
			if found[0].Start != len(prefix) || found[0].End != len(prefix)+len(reference) || found[0].Text(source) != reference {
				t.Fatalf("Detect(%q) lost original byte range: %+v", source, found[0])
			}
		}
	}
}

func TestUnicodePathEvidenceIsAskedOnlyAboutTheWholeRelativeName(t *testing.T) {
	source := "/tmp/报告.go ~/目录/报告.go 目录/报告.go 报告.go"
	var queried []string
	found := link.Detect(source, func(path string) bool {
		queried = append(queried, path)
		return true
	})
	if got := targets(found); !reflect.DeepEqual(got, []string{"/tmp/报告.go", "~/目录/报告.go", "目录/报告.go", "报告.go"}) {
		t.Fatalf("destinations = %q", got)
	}
	if !reflect.DeepEqual(queried, []string{"目录/报告.go", "报告.go"}) {
		t.Fatalf("filesystem evidence requested for %q", queried)
	}
	if found := link.Detect("报告.go", nil); len(found) != 0 {
		t.Fatalf("bare filename accepted without caller evidence: %+v", found)
	}
	queried = nil
	if found := link.Detect("目录/file.go", func(path string) bool {
		queried = append(queried, path)
		return path == "file.go"
	}); len(found) != 0 || !reflect.DeepEqual(queried, []string{"目录/file.go"}) {
		t.Fatalf("rejected whole path donated a different file: %+v, evidence=%q", found, queried)
	}
}

func TestUnicodeSlashedProseCannotDonateARootedSuffix(t *testing.T) {
	for _, source := range []string{"甲/乙", "甲/乙/丙", "café/菜单", "家族👩‍👩‍👧/成员", "证据/目录。"} {
		if found := link.Detect(source, nil); len(found) != 0 {
			t.Fatalf("slashed prose %q produced file identities: %+v", source, found)
		}
	}
}

func TestQuotedUnicodeNamesKeepSpacesAndPunctuation(t *testing.T) {
	for _, target := range []string{"/tmp/目录/报告 副本.go", "目录/报告:版本?.go", "/tmp/有 空格"} {
		for _, quote := range []string{`"`, `'`} {
			source := quote + target + quote + ":42"
			found := link.Detect(source, nil)
			if len(found) != 1 || found[0].Target != target || found[0].Line != 42 || found[0].Text(source) != source {
				t.Fatalf("quoted path %q = %+v", source, found)
			}
		}
	}
}

func TestDriveAndBackslashPathsKeepTheirSourceIdentity(t *testing.T) {
	for _, tc := range []struct {
		target   string
		relative bool
	}{
		{target: `C:/src/main.go`},
		{target: `C:\src\main.go`},
		{target: `d:\目录\报告.go`},
		{target: `\src\main.go`},
		{target: `\\server\share\main.go`},
		{target: `\\?\C:\src\main.go`},
		{target: `\\?\UNC\server\share\main.go`},
		{target: `\\.\pipe\oolong`},
		{target: `src\main.go`, relative: true},
		{target: `C:src\main.go`, relative: true},
		{target: `C:main.go`, relative: true},
	} {
		for _, quoted := range []bool{false, true} {
			reference := tc.target
			if quoted {
				reference = `"` + reference + `"`
			}
			reference += ":42:7"
			const prefix = "见 "
			source := prefix + reference + "。"
			var queried []string
			found := link.Detect(source, func(candidate string) bool {
				queried = append(queried, candidate)
				return candidate == tc.target
			})
			if len(found) != 1 || found[0].Kind != link.File || found[0].Target != tc.target || found[0].Line != 42 || found[0].Column != 7 {
				t.Fatalf("Detect(%q) = %+v; want complete source path %q", source, found, tc.target)
			}
			if found[0].Start != len(prefix) || found[0].End != len(prefix)+len(reference) || found[0].Text(source) != reference {
				t.Fatalf("Detect(%q) changed the source byte range: %+v", source, found[0])
			}
			var wantQueries []string
			if tc.relative {
				wantQueries = []string{tc.target}
			}
			if !reflect.DeepEqual(queried, wantQueries) {
				t.Fatalf("Detect(%q) requested evidence for %q, want %q", source, queried, wantQueries)
			}
		}
	}
}

func TestRejectedBackslashPathsCannotDonateAnotherTarget(t *testing.T) {
	for _, target := range []string{`src\main.go`, `C:src\main.go`, `C:main.go`} {
		var queried []string
		found := link.Detect(target+":42", func(candidate string) bool {
			queried = append(queried, candidate)
			return candidate == "main.go"
		})
		if len(found) != 0 || !reflect.DeepEqual(queried, []string{target}) {
			t.Fatalf("rejected path %q donated a target: %+v, evidence=%q", target, found, queried)
		}
	}
}
