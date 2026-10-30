package main

import (
	"errors"
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/kit"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestOwnedDraftsValidateBeforePublishingANormalizedSubmission(t *testing.T) {
	var values answers
	form, local := fields(&values)
	host := uitest.New(t, uitest.Config{Width: 70, Height: 18})
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return dress(runtime, form), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()

	host.Shows(t, "Who owns this text?")
	host.Type("draft")
	host.Press(input.Tab)
	host.Type("owner")
	host.Press(input.Tab)
	host.Type("mixed")
	host.Press(input.Tab)
	host.Type("123456789")
	host.Press(input.Enter)
	host.Shows(t, "at most eight characters")
	host.Shows(t, "123456789")
	host.Press(input.Backspace)
	host.Press(input.Enter)

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := local.Text(); got != "draft" {
		t.Fatalf("local value = %q, want draft", got)
	}
	if values.exact != "owner" || values.normalized != "MIXED" || values.guarded != "12345678" {
		t.Fatalf("submitted values = %+v", values)
	}
}

func TestControlledFieldAndExternalButtonShareOneDomainVersion(t *testing.T) {
	form, _ := fields(new(answers))
	host := uitest.New(t, uitest.Config{Width: 70, Height: 18})
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
			return dress(runtime, form), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()
	host.Shows(t, "domain-owned notifications")
	for range 4 {
		host.Press(input.Tab)
	}
	host.Type(" ")
	host.Shows(t, "● on")
	for _, x := range []int{3, 21} {
		host.Send(input.Mouse{Pos: image.Pt(x, 17), Action: input.MouseDown, Button: input.ButtonLeft})
		host.Send(input.Mouse{Pos: image.Pt(x, 17), Action: input.MouseUp, Button: input.ButtonLeft})
		if x == 3 {
			host.Shows(t, "○ on")
		}
	}
	host.Press(input.Esc)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if snapshot := form.notifications.Snapshot(); snapshot.Value || snapshot.Revision != 3 || form.confirm.Answer() {
		t.Fatalf("domain and field disagree after external reset: %+v", snapshot)
	}
	if len(form.notifications.subscribers) != 0 {
		t.Fatal("detached controller retained its domain subscription")
	}
	if got := form.theme.Scope().Snapshot(); got.Palette != kit.Light() || got.Revision != 3 {
		t.Fatalf("theme action did not publish one application palette: %+v", got)
	}
}

func TestDomainRejectsAStaleVersionWithoutOverwritingTheLiveValue(t *testing.T) {
	domain := newNotificationSettings()
	before := domain.Snapshot()
	applied, err := domain.Change(t.Context(), base.ConfirmRequest{ExpectedRevision: before.Revision, Value: true})
	if err != nil || applied.State != scene.Applied || applied.Value.Revision != before.Revision+1 {
		t.Fatalf("change = %+v, %v", applied, err)
	}
	if _, err := domain.Change(t.Context(), base.ConfirmRequest{ExpectedRevision: before.Revision, Value: false}); !errors.Is(err, base.ErrConfirmStale) {
		t.Fatalf("stale request = %v", err)
	}
	if domain.Snapshot() != applied.Value {
		t.Fatal("stale request advanced the domain")
	}
}
