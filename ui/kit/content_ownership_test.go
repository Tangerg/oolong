package kit_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

type singleChildContainer interface {
	Element() scene.Element
	Content() scene.Element
	SetContent(child scene.Element)
}

func TestChildQueriesFollowTheActualRelationDuringFocusSettlement(t *testing.T) {
	for _, kind := range []string{"panel", "viewport", "dialog"} {
		t.Run(kind, func(t *testing.T) {
			first := scene.NewElement(scene.ElementConfig{})
			next := scene.NewElement(scene.ElementConfig{})
			var owner singleChildContainer
			switch kind {
			case "panel":
				owner = kit.NewPanel(kit.PanelConfig{Content: first})
			case "viewport":
				owner = base.NewViewport(base.ViewportConfig{Content: first})
			case "dialog":
				owner = base.NewDialog(base.DialogConfig{Initial: true, Content: first})
			}
			var inspect func()
			outer := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: owner.Element()}}, Element: scene.ElementConfig{Enabled: func() bool {
				if inspect != nil {
					inspect()
				}
				return true
			}}})
			root, err := scene.NewRoot(scene.RootConfig{Element: outer.Element()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(root.Close)
			before, wake := root.Progress().ModelRevision, root.Changes()
			seen := false
			inspect = func() {
				if !next.Target().IsZero() {
					seen = true
					if owner.Content() != next {
						t.Error("scene predicate saw the new attachment and the old Content")
					}
					panic("stop after attachment")
				}
			}
			func() {
				defer func() {
					if got := recover(); got != "stop after attachment" {
						t.Fatalf("unexpected settlement outcome %v", got)
					}
				}()
				owner.SetContent(next)
			}()
			inspect = nil
			if root.Progress().ModelRevision <= before {
				t.Error("installed child kept the previous model revision")
			}
			select {
			case <-wake:
			default:
				t.Error("installed child did not wake observers")
			}
			if !seen || !first.Target().IsZero() || next.Target().IsZero() || owner.Content() != next {
				t.Fatal("component and scene disagree after interrupted focus settlement")
			}
		})
	}
}
