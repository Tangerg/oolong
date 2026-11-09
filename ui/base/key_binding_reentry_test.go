package base_test

import (
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func TestReplacedKeyMapDeliveryDoesNotEraseANewerSequence(t *testing.T) {
	var original, replacement keymap.Map
	x, y := input.Chord{Rune: 'x'}, input.Chord{Rune: 'y'}
	original.Bind(base.SelectNext, x)
	original.Bind(base.SelectLast, x, x)
	replacement.Bind(base.SelectNext, y)
	replacement.Bind(base.SelectLast, y, y)
	var resolveReplacement func()
	replacement.SetResolver(func(_ time.Duration, resolve func()) func() { resolveReplacement = resolve; return func() {} })
	var second *base.List[int]
	original.SetResolver(func(time.Duration, func()) func() {
		return func() {
			second.SetKeys(&replacement)
			second.Handle(input.Key{Rune: 'y'})
		}
	})
	first := base.NewList(base.ListConfig[int]{Items: []int{0, 1, 2}, Keys: &original})
	second = base.NewList(base.ListConfig[int]{Items: []int{0, 1, 2}, Keys: &original})
	scenes := scenetest.New()
	defer scenes.Close()
	scenes.Mount(first)
	scenes.Mount(second)
	scenes.Focus(first, true)
	scenes.Focus(second, true)
	if !first.Handle(input.Key{Rune: 'x'}) {
		t.Fatal("first sequence did not start")
	}
	original.Bind(base.SelectNext, input.Chord{Rune: 'z'})
	if resolveReplacement == nil {
		t.Fatal("replacement sequence did not start")
	}
	resolveReplacement()
	if second.Selected() != 1 {
		t.Fatalf("old map notification erased replacement sequence: selected=%d", second.Selected())
	}
}

func TestKeyMapDeliveryDoesNotEraseSequenceStartedAtCommittedVersion(t *testing.T) {
	var bindings keymap.Map
	x := input.Chord{Rune: 'x'}
	bindings.Bind(base.SelectNext, x)
	bindings.Bind(base.SelectLast, x, x)
	var second *base.List[int]
	var resolveSecond func()
	calls := 0
	bindings.SetResolver(func(_ time.Duration, resolve func()) func() {
		calls++
		if calls == 1 {
			return func() { second.Handle(input.Key{Rune: 'x'}) }
		}
		resolveSecond = resolve
		return func() {}
	})
	first := base.NewList(base.ListConfig[int]{Items: []int{0, 1, 2}, Keys: &bindings})
	second = base.NewList(base.ListConfig[int]{Items: []int{0, 1, 2}, Keys: &bindings})
	scenes := scenetest.New()
	defer scenes.Close()
	scenes.Mount(first)
	scenes.Mount(second)
	scenes.Focus(first, true)
	scenes.Focus(second, true)
	if !first.Handle(input.Key{Rune: 'x'}) {
		t.Fatal("first sequence did not start")
	}
	bindings.Bind(base.SelectNext, input.Chord{Rune: 'z'})
	if resolveSecond == nil {
		t.Fatal("second sequence did not start at the committed map version")
	}
	resolveSecond()
	if second.Selected() != 1 {
		t.Fatalf("committed map notification erased its own new sequence: selected=%d", second.Selected())
	}
}

func TestMapChangePublishesBeforeCancellationCanPrepareAnotherConsumer(t *testing.T) {
	var bindings keymap.Map
	x := input.Chord{Rune: 'x'}
	bindings.Bind(base.SelectNext, x)
	bindings.Bind(base.SelectLast, x, x)
	var other *scenetest.Scene
	var prepared *scene.Prepared
	var revisionAtCancellation uint64
	bindings.SetResolver(func(time.Duration, func()) func() {
		return func() {
			observation, err := other.Observe(scene.Query{})
			if err != nil {
				t.Fatalf("observation from resolver cancellation: %v", err)
			}
			revisionAtCancellation = observation.Progress.ModelRevision
			prepared, err = other.Prepare(grid.NewSurface(10, 2).View())
			if err != nil {
				t.Fatalf("preparation from resolver cancellation: %v", err)
			}
		}
	})
	first := base.NewList(base.ListConfig[int]{Items: []int{0, 1, 2}, Keys: &bindings})
	second := base.NewList(base.ListConfig[int]{Items: []int{0, 1, 2}, Keys: &bindings})
	scenes := scenetest.New()
	defer scenes.Close()
	defer func() { prepared.Abort() }()
	scenes.Mount(first)
	other = scenes.Mount(second)
	scenes.Focus(first, true)
	if !first.Handle(input.Key{Rune: 'x'}) {
		t.Fatal("first sequence did not start")
	}
	before := other.Progress().ModelRevision
	var failure any
	func() {
		defer func() { failure = recover() }()
		bindings.Bind(base.SelectFirst, input.Chord{Rune: 'z'})
	}()
	if failure != nil || revisionAtCancellation <= before {
		action, _ := bindings.Action(input.Chord{Rune: 'z'})
		t.Fatalf("change failed after commit: panic=%v committed=%q other-revision=%d before=%d callback-revision=%d", failure, action, other.Progress().ModelRevision, before, revisionAtCancellation)
	}
}
