package base_test

import (
	"context"
	"errors"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestSettingsRequestsPreserveAcceptedEffectsAlongsideErrors(t *testing.T) {
	physical := errors.New("physical request failed")
	problem := errors.New("request has a later failure")
	for _, state := range []scene.ActionState{scene.Unchanged, scene.Applied, scene.Started} {
		settings := base.NewSettings(base.SettingsConfig[string]{
			Change: func(context.Context, int, string, keymap.Action) (scene.ActionResult[bool], error) {
				return scene.ActionResult[bool]{Value: true}, physical
			},
		})
		settings.List().SetItems([]string{"volume"})
		root := semanticOwner(t, settings.Element())
		if !root.Handle(input.Key{Code: input.Right}) || !errors.Is(settings.ChangeError(), physical) {
			t.Fatal("physical failure did not retain its consumed input and feedback")
		}
		settings.SetChange(func(ctx context.Context, index int, item string, action keymap.Action) (scene.ActionResult[bool], error) {
			if ctx != t.Context() || index != 0 || item != "volume" || action != base.Increase {
				t.Fatal("request did not preserve its context or address")
			}
			return scene.ActionResult[bool]{State: state, Value: true}, problem
		})
		before, changed := root.Progress().ModelRevision, root.Changes()
		result, err := settings.Request(t.Context(), 0, base.Increase)
		if result.State != state || !result.Value || !errors.Is(err, problem) || !errors.Is(settings.ChangeError(), physical) {
			t.Fatalf("program request = %+v, %v, feedback=%v", result, err, settings.ChangeError())
		}
		assertModelPublication(t, root, before, changed, state != scene.Unchanged)
		address := base.CollectionIndex{ContentRevision: semanticRevision(t, root, settings.Element())}
		invoked, err := root.Invoke(t.Context(), settings.Element().Target(), scene.ActionIncrement, address)
		if invoked.State != state || invoked.Value != true || !errors.Is(err, problem) || !errors.Is(settings.ChangeError(), physical) {
			t.Fatalf("semantic request = %+v, %v, feedback=%v", invoked, err, settings.ChangeError())
		}
		node := observedNode(t, root, settings.Element())
		if !node.State.Has(scene.StateInvalid) || node.Description != physical.Error() {
			t.Fatalf("physical semantic feedback = %+v", node.Semantic)
		}
	}
}

func TestSettingsRequestRejectsInvalidBoundariesBeforeCallingTheDomain(t *testing.T) {
	calls := 0
	settings := base.NewSettings(base.SettingsConfig[string]{
		Change: func(context.Context, int, string, keymap.Action) (scene.ActionResult[bool], error) {
			calls++
			return scene.ActionResult[bool]{State: scene.Applied, Value: true}, nil
		},
	})
	settings.List().SetItems([]string{"volume"})
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("caller stopped")
	cancel(cause)
	if result, err := settings.Request(ctx, 0, base.Increase); result.State != scene.Unchanged || !errors.Is(err, cause) {
		t.Fatalf("cancelled request = %+v, %v", result, err)
	}
	for _, index := range []int{-1, 1} {
		if result, err := settings.Request(t.Context(), index, base.Increase); result.State != scene.Unchanged || !errors.Is(err, scene.ErrUnavailable) {
			t.Fatalf("invalid row request = %+v, %v", result, err)
		}
	}
	settings.SetChange(nil)
	if result, err := settings.Request(t.Context(), 0, base.Increase); result.State != scene.Unchanged || !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("read-only request = %+v, %v", result, err)
	}
	if calls != 0 || settings.ChangeError() != nil {
		t.Fatalf("boundary refusal reached the domain or physical feedback: calls=%d, error=%v", calls, settings.ChangeError())
	}
}

func TestSettingsKeepsFeedbackFromTheNewestReentrantPhysicalRequest(t *testing.T) {
	older, newer := errors.New("older request failed"), errors.New("newer request failed")
	for _, exit := range []string{"return", "panic", "Goexit"} {
		for _, latest := range []error{nil, newer} {
			var settings *base.Settings[string]
			nested := false
			settings = base.NewSettings(base.SettingsConfig[string]{
				Change: func(context.Context, int, string, keymap.Action) (scene.ActionResult[bool], error) {
					if nested {
						return scene.ActionResult[bool]{Value: true}, latest
					}
					nested = true
					settings.Do(base.Increase)
					if exit == "Goexit" {
						runtime.Goexit()
					}
					if exit == "panic" {
						panic("interrupted callback")
					}
					return scene.ActionResult[bool]{Value: true}, older
				},
			})
			settings.List().SetItems([]string{"volume"})
			if exit == "return" {
				settings.Do(base.Increase)
			} else {
				runInterruptedCallback(t, exit, func() { settings.Do(base.Increase) })
			}
			if !nested || !errors.Is(settings.ChangeError(), latest) {
				t.Fatalf("%s: older request replaced feedback: got %v, want %v", exit, settings.ChangeError(), latest)
			}
		}
	}
}

func TestSettingsErrorsDoNotInventInputConsumption(t *testing.T) {
	problem := errors.New("action refused")
	settings := base.NewSettings(base.SettingsConfig[string]{
		Change: func(context.Context, int, string, keymap.Action) (scene.ActionResult[bool], error) {
			return scene.ActionResult[bool]{}, problem
		},
	})
	settings.List().SetItems([]string{"volume"})
	if settings.Do(base.Increase) || !errors.Is(settings.ChangeError(), problem) {
		t.Fatalf("failure changed input consumption: %v", settings.ChangeError())
	}
}

func TestSettingsSuccessfulRetryPublishesClearedPhysicalFeedback(t *testing.T) {
	problem := errors.New("setting failed")
	settings := base.NewSettings(base.SettingsConfig[string]{
		Change: func(context.Context, int, string, keymap.Action) (scene.ActionResult[bool], error) {
			return scene.ActionResult[bool]{Value: true}, problem
		},
	})
	settings.List().SetItems([]string{"volume"})
	root := semanticOwner(t, settings.Element())
	for range 2 {
		before, changed := root.Progress().ModelRevision, root.Changes()
		if !root.Handle(input.Key{Code: input.Right}) || !errors.Is(settings.ChangeError(), problem) {
			t.Fatalf("physical feedback = %v, want %v", settings.ChangeError(), problem)
		}
		assertModelPublication(t, root, before, changed, true)
		problem = nil
	}
}

func TestSettingsRequestRejectsReadOnlyAndPendingFrameMutation(t *testing.T) {
	settings := base.NewSettings(base.SettingsConfig[string]{
		Change: func(context.Context, int, string, keymap.Action) (scene.ActionResult[bool], error) {
			t.Error("read-only request reached the domain")
			return scene.ActionResult[bool]{}, nil
		},
	})
	settings.List().SetItems([]string{"volume"})
	root := semanticOwner(t, settings.Element())
	request := func() { _, _ = settings.Request(t.Context(), 0, base.Increase) }
	failure, _ := configurationFailure(func() { settings.Element().ReadOnly(request) }).(error)
	if !errors.Is(failure, scene.ErrBusy) {
		t.Fatalf("read-only request = %v", failure)
	}
	frame, err := root.Prepare(grid.NewSurface(12, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	failure, _ = configurationFailure(request).(error)
	frame.Abort()
	if !errors.Is(failure, scene.ErrBusy) {
		t.Fatalf("pending frame request = %v", failure)
	}
}
