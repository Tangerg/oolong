package program

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

type ambiguousComponent struct {
	keys     keymap.Map
	matcher  keymap.Matcher
	actions  int
	consumed bool
}

func (*ambiguousComponent) Draw(grid.View) {}
func (c *ambiguousComponent) Handle(ev input.Event) bool {
	_, c.consumed = c.matcher.Handle(&c.keys, ev.(input.Key), func(keymap.Action) bool { c.actions++; return true })
	return c.consumed
}

func TestDeclinedInputStillPresentsResolvedPrefixAction(t *testing.T) {
	c := &ambiguousComponent{}
	g := input.Chord{Code: input.Character, Rune: 'g'}
	c.keys.Bind("short", g)
	c.keys.Bind("long", g, g)
	p := &program{root: c}
	present := func() bool {
		t.Helper()
		drawn, err := p.present.Present(time.Now(), func(bool) (uint64, error) { return 0, nil })
		if err != nil {
			t.Fatal(err)
		}
		return drawn
	}
	if err := p.handle(input.Key{Code: input.Character, Rune: 'g'}); err != nil {
		t.Fatal(err)
	}
	if !present() || c.actions != 0 {
		t.Fatal("prefix did not wait for a continuation")
	}
	if err := p.handle(input.Key{Code: input.Character, Rune: 'x'}); err != nil {
		t.Fatal(err)
	}
	if c.actions != 1 || c.consumed {
		t.Fatal("short action must run while x remains unconsumed")
	}
	if !present() {
		t.Fatal("resolved prefix changed state but did not request a frame")
	}
	if present() {
		t.Fatal("one input left repeated redraws pending")
	}
}

func TestFrameTimerResetAndCancellationDiscardExpiredValues(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		timer := newFrameTimer()
		defer timer.stop()
		timer.schedule(time.Now().Add(time.Second), true)
		time.Sleep(2 * time.Second)
		timer.schedule(time.Now().Add(time.Second), true)
		select {
		case <-timer.channel():
			t.Fatal("reset exposed old expiration")
		default:
		}
		time.Sleep(2 * time.Second)
		timer.schedule(time.Time{}, false)
		select {
		case <-timer.channel():
			t.Fatal("cancel exposed old expiration")
		default:
		}
		timer.schedule(time.Now(), true)
		<-timer.channel()
	})
}
