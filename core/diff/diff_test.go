package diff_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/diff"
)

// The tests assert on what a diff reads as, not on which internal path produced it.

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func TestWhatChangedReadsAsTheChange(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after string
		want          string
	}{
		{
			name:   "nothing",
			before: "a\nb", after: "a\nb",
			want: " a\n b\n",
		},
		{
			name:   "a line in the middle",
			before: "a\nb\nc", after: "a\nB\nc",
			want: " a\n-b\n+B\n c\n",
		},
		{
			name:   "an insertion",
			before: "a\nc", after: "a\nb\nc",
			want: " a\n+b\n c\n",
		},
		{
			name:   "a deletion",
			before: "a\nb\nc", after: "a\nc",
			want: " a\n-b\n c\n",
		},
		{
			name:   "from nothing",
			before: "", after: "a\nb",
			want: "+a\n+b\n",
		},
		{
			name:   "to nothing",
			before: "a\nb", after: "",
			want: "-a\n-b\n",
		},
		{
			name:   "nothing in common",
			before: "a\nb", after: "x\ny",
			want: "-a\n-b\n+x\n+y\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := diff.Between(split(tc.before), split(tc.after)).String(); got != tc.want {
				t.Fatalf("got:\n%swant:\n%s", got, tc.want)
			}
		})
	}
}

func TestEveryLineOfBothTextsAppearsExactlyOnce(t *testing.T) {
	// The property that makes a diff a diff, checked against inputs nobody chose. A
	// script that dropped a line or showed one twice would still look plausible on the
	// cases somebody thought to write down.
	// A fixed seed, so a failure is one anybody can reproduce. Nothing here is a
	// secret; the point is inputs nobody chose, not inputs nobody can predict.
	r := rand.New(rand.NewSource(1)) //nolint:gosec // reproducibility is the point
	letters := []string{"a", "b", "c", "d", "e", "f"}
	pick := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = letters[r.Intn(len(letters))]
		}
		return out
	}
	for range 300 {
		before, after := pick(r.Intn(12)), pick(r.Intn(12))
		lines := diff.Between(before, after)

		var kept, added []string
		for _, line := range lines {
			switch line.Kind {
			case diff.Removed:
				kept = append(kept, line.Text)
			case diff.Added:
				added = append(added, line.Text)
			default:
				kept, added = append(kept, line.Text), append(added, line.Text)
			}
		}
		if strings.Join(kept, "") != strings.Join(before, "") {
			t.Fatalf("the removals and context do not rebuild the first text:\n%v\n%v", kept, before)
		}
		if strings.Join(added, "") != strings.Join(after, "") {
			t.Fatalf("the additions and context do not rebuild the second text:\n%v\n%v", added, after)
		}
	}
}

func TestALineKnowsWhereItIsInBothTexts(t *testing.T) {
	// A reader looking for what to open needs the number in the text that still
	// exists; one reading the change needs both.
	lines := diff.Between(split("a\nb\nc"), split("a\nB\nc"))
	want := []diff.Line{
		{Text: "a", Old: 1, New: 1},
		{Kind: diff.Removed, Text: "b", Old: 2},
		{Kind: diff.Added, Text: "B", New: 2},
		{Text: "c", Old: 3, New: 3},
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(lines), len(want), lines)
	}
	for i, line := range lines {
		if line != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, line, want[i])
		}
	}
}

func TestHunksLeaveOutWhatNobodyIsReading(t *testing.T) {
	before := strings.Split("1\n2\n3\n4\n5\n6\n7\n8\n9", "\n")
	after := strings.Split("1\n2\n3\nX\n5\n6\n7\n8\n9", "\n")
	hunks := diff.Between(before, after).Hunks(1)
	if len(hunks) != 1 {
		t.Fatalf("got %d hunks, want one", len(hunks))
	}
	if got := hunks[0].String(); got != " 3\n-4\n+X\n 5\n" {
		t.Fatalf("hunk:\n%s", got)
	}
	if hunks[0].Old != 3 || hunks[0].New != 3 {
		t.Fatalf("hunk starts at old %d new %d, want the first line it shows", hunks[0].Old, hunks[0].New)
	}
}

func TestTwoChangesCloseTogetherAreOneHunk(t *testing.T) {
	// A gap of one unchanged line is not a gap worth drawing a break across.
	before := strings.Split("1\n2\n3\n4\n5", "\n")
	after := strings.Split("1\nX\n3\nY\n5", "\n")
	if got := len(diff.Between(before, after).Hunks(1)); got != 1 {
		t.Fatalf("got %d hunks, want the overlapping context to join them", got)
	}
}

func TestNothingChangedIsNoHunks(t *testing.T) {
	same := strings.Split("a\nb\nc", "\n")
	if hunks := diff.Between(same, same).Hunks(3); hunks != nil {
		t.Fatalf("got %d hunks for two texts that are the same", len(hunks))
	}
}

func TestTwoTextsWithTooLittleInCommonAreSaidToBeReplaced(t *testing.T) {
	// Past a point the two have nothing much to line up, and the honest answer is
	// cheaper than the exact one nobody would read.
	before := make([]string, 2000)
	after := make([]string, 2000)
	for i := range before {
		before[i] = "old " + string(rune('a'+i%26))
		after[i] = "new " + string(rune('A'+i%26))
	}
	lines := diff.Between(before, after)
	if len(lines) != len(before)+len(after) {
		t.Fatalf("got %d lines, want every line of both", len(lines))
	}
	if lines[0].Kind != diff.Removed || lines[len(lines)-1].Kind != diff.Added {
		t.Fatal("want all of one and then all of the other")
	}
}
