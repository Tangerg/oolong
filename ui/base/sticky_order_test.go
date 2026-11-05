package base_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestStickyRejectsInvalidIdentityOrderBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name    string
		replace bool
		blocks  []base.BlockID
	}{
		{name: "replace zero", replace: true, blocks: []base.BlockID{0, 1}},
		{name: "replace duplicate", replace: true, blocks: []base.BlockID{1, 1}},
		{name: "replace descending", replace: true, blocks: []base.BlockID{3, 1}},
		{name: "append zero", blocks: []base.BlockID{0}},
		{name: "append previous identity", blocks: []base.BlockID{3}},
		{name: "append descending batch", blocks: []base.BlockID{4, 2}},
		{name: "append duplicate batch", blocks: []base.BlockID{4, 4}},
	} {
		t.Run(test.name, func(t *testing.T) {
			transcript := transcriptModel(t, base.TranscriptConfig{})
			sticky := transcript.Sticky()
			previous := []base.BlockID{1, 3}
			sticky.SetBlocks(previous)
			root := transcriptScene(t, transcript)
			changed := root.Changes()
			func() {
				defer func() {
					if recover() == nil {
						t.Error("invalid identity order entered the sticky owner")
					}
				}()
				if test.replace {
					sticky.SetBlocks(test.blocks)
				} else {
					sticky.Add(test.blocks...)
				}
			}()
			if got := sticky.Blocks(); !slices.Equal(got, previous) {
				t.Fatalf("rejected identity batch changed blocks to %v, want %v", got, previous)
			}
			select {
			case <-changed:
				t.Fatal("rejected identity batch published a scene change")
			default:
			}
		})
	}
}
