package base_test

import (
	"runtime"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func TestCommittedEditorChangesReachTheSubscribersThatOwnedTheirDelivery(t *testing.T) {
	for _, abnormal := range []string{"panic", "goexit"} {
		t.Run(abnormal, func(t *testing.T) {
			s := base.NewEditSession(base.EditSessionConfig{})
			editor := base.NewEditor(base.EditorConfig{Session: s})
			start := s.Revision()
			first := true
			var ordinary, late []uint64
			s.Subscribe(func(base.ChangeMap) {
				if !first {
					return
				}
				first = false
				editor.Insert("b")
				s.Subscribe(func(change base.ChangeMap) { late = append(late, change.After) })
				if abnormal == "goexit" {
					runtime.Goexit()
				}
				panic("observer")
			})
			s.Subscribe(func(change base.ChangeMap) { ordinary = append(ordinary, change.After) })
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					got := recover()
					if abnormal == "panic" && got != "observer" || abnormal == "goexit" && got != nil {
						t.Errorf("unexpected unwind: %v", got)
					}
				}()
				editor.Insert("a")
				t.Error("failed observer returned")
			}()
			<-done
			if !slices.Equal(ordinary, []uint64{start + 1, start + 2}) || len(late) != 0 {
				t.Fatalf("delivered revisions %v; late subscriber received %v", ordinary, late)
			}
			editor.Insert("c")
			if !slices.Equal(late, []uint64{start + 3}) {
				t.Fatalf("next delivery=%v", late)
			}
		})
	}
}

func TestCommittedEditorChangeSurvivesAnInterruptedSceneWake(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	s := base.NewEditSession(base.EditSessionConfig{})
	editor := base.NewEditor(base.EditorConfig{Session: s})
	var revisions []uint64
	s.Subscribe(func(change base.ChangeMap) { revisions = append(revisions, change.After) })
	peer := base.NewEditor(base.EditorConfig{Session: s})
	after := scenes.Mount(peer.Element())
	defer after.Root.Close()
	wake := after.Changes()
	armed := false
	if err := editor.Element().SetFocusable(func() bool {
		if armed {
			armed = false
			panic("predicate")
		}
		return true
	}); err != nil {
		t.Fatal(err)
	}
	root := scenes.Mount(editor.Element())
	defer root.Root.Close()
	armed = true
	func() {
		defer func() {
			if recover() != "predicate" {
				t.Fatal("unexpected predicate outcome")
			}
		}()
		editor.Insert("a")
	}()
	select {
	case <-wake:
	default:
		t.Error("peer scene missed the edit")
	}
	if !slices.Equal(revisions, []uint64{s.Revision()}) {
		t.Fatalf("committed changes not delivered: %v", revisions)
	}
}
