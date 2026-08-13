package text_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/text"
)

// The property everything else rests on: a mark shifted over an edit is over the same
// words it was over before. Stating it as "the same words" rather than as arithmetic
// is what makes the test survive a change to how the arithmetic is done.
func covered(document string, m text.Mark) string {
	if m.Start < 0 || m.End > len(document) || m.Start >= m.End {
		return ""
	}
	return document[m.Start:m.End]
}

func TestAMarkStillCoversTheSameWordsAfterAnEditElsewhere(t *testing.T) {
	const document = "the quick brown fox"
	brown := text.Mark{ID: 1, Start: 10, End: 15}
	if got := covered(document, brown); got != "brown" {
		t.Fatalf("the mark covers %q to begin with", got)
	}

	for _, tc := range []struct {
		name string
		edit text.Edit
	}{
		{"an insertion before it", text.Edit{Start: 0, End: 0, Text: "so "}},
		{"an insertion after it", text.Edit{Start: 19, End: 19, Text: " ran"}},
		{"a deletion before it", text.Edit{Start: 0, End: 4}},
		{"a replacement before it", text.Edit{Start: 4, End: 9, Text: "slow"}},
		{"a longer replacement before it", text.Edit{Start: 4, End: 9, Text: "extremely quick"}},
		{"an insertion at its start", text.Edit{Start: 10, End: 10, Text: "dark "}},
		{"an insertion at its end", text.Edit{Start: 15, End: 15, Text: "ish"}},
		{"a deletion right before it", text.Edit{Start: 9, End: 10, Text: ""}},
	} {
		marks := tc.edit.Shift([]text.Mark{brown}, len(document))
		if len(marks) != 1 {
			t.Errorf("%s: the mark was dropped", tc.name)
			continue
		}
		if got := covered(tc.edit.Apply(document), marks[0]); got != "brown" {
			t.Errorf("%s: the mark now covers %q", tc.name, got)
		}
	}
}

// TestTextTypedAgainstAMarkStaysOutsideIt is the rule that lets a user type up
// against a chip in a prompt. The other answer swallows whatever they write next.
func TestTextTypedAgainstAMarkStaysOutsideIt(t *testing.T) {
	const document = "see file.go now"
	mark := text.Mark{ID: 1, Start: 4, End: 11, Atomic: true}
	if got := covered(document, mark); got != "file.go" {
		t.Fatalf("the mark covers %q to begin with", got)
	}

	for _, tc := range []struct {
		name string
		edit text.Edit
	}{
		{"typed in front of it", text.Edit{Start: 4, End: 4, Text: "X"}},
		{"typed behind it", text.Edit{Start: 11, End: 11, Text: "X"}},
	} {
		marks := tc.edit.Shift([]text.Mark{mark}, len(document))
		if len(marks) != 1 {
			t.Fatalf("%s: the mark was dropped", tc.name)
		}
		if got := covered(tc.edit.Apply(document), marks[0]); got != "file.go" {
			t.Errorf("%s: the mark now covers %q", tc.name, got)
		}
	}
}

func TestAnEditInsideAnAtomicMarkDestroysIt(t *testing.T) {
	// A chip that still looks like a file and points at half of one is worse than no
	// chip.
	mark := text.Mark{ID: 1, Start: 4, End: 11, Atomic: true}
	for _, tc := range []struct {
		name string
		edit text.Edit
	}{
		{"typed into the middle", text.Edit{Start: 7, End: 7, Text: "X"}},
		{"a letter taken out of it", text.Edit{Start: 6, End: 7}},
		{"a cut that overlaps its start", text.Edit{Start: 2, End: 6}},
		{"a cut that overlaps its end", text.Edit{Start: 9, End: 14}},
		{"a cut that swallows it", text.Edit{Start: 0, End: 15}},
	} {
		if got := tc.edit.Shift([]text.Mark{mark}, len("see file.go now")); len(got) != 0 {
			t.Errorf("%s: the mark survived as %+v", tc.name, got[0])
		}
	}
}

func TestAnEmptyEditDoesNotShiftAnyMark(t *testing.T) {
	const document = "see file.go now"
	for _, markCase := range []struct {
		name string
		mark text.Mark
	}{
		{"ordinary", text.Mark{ID: 1, Kind: 7, Start: 4, End: 11}},
		{"atomic", text.Mark{ID: 1, Kind: 7, Start: 4, End: 11, Atomic: true}},
	} {
		for _, tc := range []struct {
			name string
			at   int
		}{
			{"before the document", -20},
			{"before", 0},
			{"at the start", markCase.mark.Start},
			{"inside", 7},
			{"at the end", markCase.mark.End},
			{"after", len(document)},
			{"after the document", len(document) + 20},
		} {
			t.Run(markCase.name+"/"+tc.name, func(t *testing.T) {
				edit := text.Edit{Start: tc.at, End: tc.at}
				if got := edit.Apply(document); got != document {
					t.Fatalf("empty edit changed the document to %q", got)
				}
				if got := edit.Delta(len(document)); got != 0 {
					t.Fatalf("empty edit delta = %d, want 0", got)
				}
				want := []text.Mark{markCase.mark}
				if got := edit.Shift(slices.Clone(want), len(document)); !slices.Equal(got, want) {
					t.Fatalf("empty edit shifted the mark from %+v to %+v", want, got)
				}
			})
		}
	}
}

func TestAMarkThatIsNotAtomicStretchesOverWhatWasTypedInIt(t *testing.T) {
	// A highlight is about the text it covers, and text inserted in the middle of it
	// is still covered.
	const document = "the quick brown fox"
	mark := text.Mark{ID: 1, Start: 4, End: 9}
	edit := text.Edit{Start: 6, End: 6, Text: "XY"}

	marks := edit.Shift([]text.Mark{mark}, len(document))
	if len(marks) != 1 {
		t.Fatal("a highlight was destroyed by text typed inside it")
	}
	if got := covered(edit.Apply(document), marks[0]); got != "quXYick" {
		t.Errorf("the mark covers %q, want it stretched over what was typed", got)
	}
}

func TestAMarkTheEditTookEntirelyIsGone(t *testing.T) {
	// Whatever kind it is: a range covering nothing says nothing about the text, and
	// a caller keying a record off it would keep the record for ever.
	for _, atomic := range []bool{false, true} {
		mark := text.Mark{ID: 1, Start: 4, End: 9, Atomic: atomic}
		if got := (text.Edit{Start: 4, End: 9}).Shift([]text.Mark{mark}, len("the quick brown fox")); len(got) != 0 {
			t.Errorf("atomic=%v: the mark survived as %+v", atomic, got[0])
		}
	}
}

func TestShiftKeepsTheOrderAndTheIdentities(t *testing.T) {
	marks := []text.Mark{
		{ID: 1, Kind: 7, Start: 0, End: 3},
		{ID: 2, Start: 4, End: 9, Atomic: true},
		{ID: 3, Start: 10, End: 15},
	}
	got := (text.Edit{Start: 5, End: 6, Text: "X"}).Shift(marks, len("the quick brown fox"))
	if len(got) != 2 {
		t.Fatalf("got %d marks, want the two the edit left alone", len(got))
	}
	if got[0].ID != 1 || got[1].ID != 3 {
		t.Errorf("the survivors are %d and %d", got[0].ID, got[1].ID)
	}
	if got[0].Kind != 7 {
		t.Errorf("a mark's label became %d", got[0].Kind)
	}
}

func TestAnEditOutsideTheDocumentDoesNotPanic(t *testing.T) {
	// An edit worked out from a position that has since moved has to be survivable:
	// the alternative is a program that dies because two things raced.
	for _, e := range []text.Edit{
		{Start: -5, End: 2, Text: "x"},
		{Start: 2, End: 900, Text: "x"},
		{Start: 9, End: 2, Text: "x"},
		{Start: 900, End: 900, Text: "x"},
	} {
		if got := e.Apply("abcdef"); !strings.Contains(got, "x") {
			t.Errorf("edit %+v applied to give %q", e, got)
		}
	}
}

func TestApplyIsTheThreeShapesOfAChange(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit text.Edit
		want string
	}{
		{"an insertion", text.Edit{Start: 3, End: 3, Text: "-"}, "abc-def"},
		{"a deletion", text.Edit{Start: 2, End: 4}, "abef"},
		{"a replacement", text.Edit{Start: 1, End: 5, Text: "X"}, "aXf"},
	} {
		if got := tc.edit.Apply("abcdef"); got != tc.want {
			t.Errorf("%s gave %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestAnOutOfRangeEditMovesMarksByTheRangeItActuallyReplaced(t *testing.T) {
	const document = "abcdef"
	mark := text.Mark{ID: 1, Start: 3, End: 6}
	edit := text.Edit{Start: -5, End: 2, Text: "x"}

	marks := edit.Shift([]text.Mark{mark}, len(document))
	if len(marks) != 1 {
		t.Fatal("an edit before the document destroyed an untouched mark")
	}
	if got := covered(edit.Apply(document), marks[0]); got != "def" {
		t.Fatalf("the mark covers %q after the clamped edit, want %q", got, "def")
	}
	if got := edit.Delta(len(document)); got != -1 {
		t.Fatalf("clamped edit delta = %d, want -1", got)
	}
}
