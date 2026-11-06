package text_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/text"
)

func TestClusterBoundaryUsesTheCompleteString(t *testing.T) {
	for _, tc := range []struct {
		name       string
		source     string
		boundaries []int
	}{
		{"empty", "", []int{0}},
		{"ASCII", "ab", []int{0, 1, 2}},
		{"line break", "a\r\nb", []int{0, 1, 3, 4}},
		{"combining", "e\u0301x", []int{0, 3, 4}},
		{"emoji joiner", "👩\u200d💻", []int{0, 11}},
		{"odd regional indicators", "🇦🇦🇧", []int{0, 8, 12}},
		{"even regional indicators", "🇦🇧🇨🇩", []int{0, 8, 16}},
		{"Indic linker", "क्क", []int{0, 9}},
		{"malformed separator", "A\xe2\u0301B", []int{0, 1, 2, 4, 5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for at := -1; at <= len(tc.source)+1; at++ {
				if got, want := text.IsClusterBoundary(tc.source, at), slices.Contains(tc.boundaries, at); got != want {
					t.Errorf("IsClusterBoundary(%q, %d) = %v, want %v", tc.source, at, got, want)
				}
			}
		})
	}
}
