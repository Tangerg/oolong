package base_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/fuzzy"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

type borrowedCandidate struct{ offsets []int }

func (r *borrowedCandidate) Width(candidate base.Candidate) int {
	r.offsets = candidate.Matched
	return 5
}

func (r *borrowedCandidate) DrawRow(_ grid.View, candidate base.Candidate, _ bool, _ base.Look) {
	r.offsets = candidate.Matched
}

func TestAppearanceCallbacksCannotRewriteMatchOffsetsAfterTheyReturn(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	r := new(borrowedCandidate)
	c := &base.Completion{Renderer: r}
	c.Offer(base.Token{}, []base.Candidate{{Text: "abc", Matched: []int{1}}})
	root := scenes.Mount(c.Element())
	for _, project := range []func(){func() { c.Width() }, func() { root.Draw(grid.NewSurface(8, 1).View()) }} {
		project()
		r.offsets[0] = 9
		if got, _ := c.Current(); !slices.Equal(got.Matched, []int{1}) {
			t.Errorf("appearance changed candidate: %+v", got)
		}
	}

	f := &base.Filter[string]{}
	var saved []int
	f.Row = func(_ grid.View, _ int, _ string, match fuzzy.Match, _ bool) { saved = match.At }
	f.SetItems([]string{"ab"}, func(s string) string { return s })
	f.SetPattern("b")
	filterRoot := scenes.Mount(f.Element())
	filterRoot.Draw(grid.NewSurface(8, 1).View())
	saved[0] = 9
	filterRoot.Draw(grid.NewSurface(8, 1).View())
	if !slices.Equal(saved, []int{1}) {
		t.Errorf("retained match offsets changed a later frame: %v", saved)
	}
}
