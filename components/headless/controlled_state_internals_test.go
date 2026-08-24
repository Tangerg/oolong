package headless

import (
	"testing"
	"unicode/utf8"
)

type rejectingChoice struct{ value string }

func (r *rejectingChoice) Value() string { return r.value }
func (*rejectingChoice) Set(string)      {}

func TestSelectSettlesItsCursorToTheValueItsOwnerAccepted(t *testing.T) {
	value := &rejectingChoice{value: "a"}
	field := &Select[string]{Value: value}
	field.SetOptions(Options("a", "b"))
	_, _ = field.Chosen()

	if !field.Do(SelectNext) {
		t.Fatal("next choice was not handled")
	}
	if got := field.list.Selected(); got != 0 {
		t.Fatalf("internal cursor = %d, want the accepted owner choice at 0", got)
	}
}

func TestReconciledOffsetPreservesPositionAroundNormalization(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after string
		at, want      int
	}{
		{"same-length rewrite", "helloX world", "HELLOX WORLD", 6, 6},
		{"prefix insertion boundary", "world", "hello world", 0, 6},
		{"prefix inserted", "world", "hello world", 3, 9},
		{"suffix insertion boundary", "hello", "hello!", 5, 6},
		{"prefix removed", "hello world", "world", 9, 3},
		{"replacement shrinks", "abXXcd", "abYcd", 4, 3},
		{"combining form composed", "e\u0301x", "éx", len("e\u0301"), len("é")},
		{"changed interval lands inside a rune", "0000", "00˱", 3, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := reconciledOffset(tc.before, tc.after, tc.at); got != tc.want {
				t.Fatalf("reconciled offset = %d, want %d", got, tc.want)
			}
		})
	}
}

func FuzzReconciledOffsetReturnsRuneBoundary(f *testing.F) {
	f.Add("0000", "00˱", 3)
	f.Add("ab", "a👩‍💻b", 1)
	f.Add("e\u0301x", "éx", len("e\u0301"))

	f.Fuzz(func(t *testing.T, before, after string, at int) {
		const limit = 512
		if len(before) > limit {
			before = before[:limit]
		}
		if len(after) > limit {
			after = after[:limit]
		}
		var editor Editor
		before = editor.canonicalText(before)
		after = editor.canonicalText(after)

		got := reconciledOffset(before, after, at)
		if got < 0 || got > len(after) {
			t.Fatalf("reconciled offset = %d, outside [0,%d]", got, len(after))
		}
		if got < len(after) && !utf8.RuneStart(after[got]) {
			t.Fatalf("reconciled offset = %d, inside UTF-8 encoding %q", got, after)
		}
	})
}
