package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
	"github.com/Tangerg/oolong/ui/uitest"
)

type conversationTest struct {
	conversation *conversation
	runtime      *program.Runtime
	host         *uitest.Host
}

func newTestConversation(t *testing.T, height int, publisher base.TranscriptPublisher) *conversationTest {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	host := uitest.New(t, uitest.Config{Width: 40, Height: height})
	started := make(chan *conversationTest, 1)
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) { //nolint:contextcheck // Runtime scopes inherit ctx from Run; Build does not create another parent lifetime.
			scope, err := runtime.NewScope(program.ScopeConfig{})
			if err != nil {
				return scene.Element{}, err
			}
			c, err := newConversation(conversationConfig{Runtime: runtime, Scope: scope, Glyphs: kit.GlyphsFor(""), Publisher: publisher})
			if err != nil {
				return scene.Element{}, err
			}
			root := scene.NewGroup(scene.GroupConfig{Overlay: true, Element: scene.ElementConfig{Detached: func() { scope.Close(); c.Close() }}, Items: []scene.Item{{Element: c.Element()}}})
			started <- &conversationTest{conversation: c, runtime: runtime, host: host}
			return root.Element(), nil
		}})
		if err == nil {
			err = app.Run(ctx)
		}
		done <- err
	}()
	select {
	case test := <-started:
		t.Cleanup(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("conversation stopped: %v", err)
			}
		})
		return test
	case err := <-done:
		cancel()
		t.Fatalf("conversation did not start: %v", err)
	case <-t.Context().Done():
		cancel()
		t.Fatal(t.Context().Err())
	}
	return nil
}

func (c *conversationTest) apply(t *testing.T, fn func(*conversation) error) {
	t.Helper()
	settled := make(chan error, 1)
	if err := c.runtime.Dispatcher().Invoke(t.Context(), func() error {
		if err := fn(c.conversation); err != nil {
			return err
		}
		return c.conversation.Settled(func(err error) { settled <- err })
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-settled:
		if err != nil {
			t.Fatal(err)
		}
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	if _, err := c.host.Repaint(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestAnAnswerThatEndsWithNothingLeftStillEnds(t *testing.T) {
	publisher, err := publication.New(publication.Config{Width: 40})
	if err != nil {
		t.Fatal(err)
	}
	c := newTestConversation(t, 10, publisher)
	var first *base.TranscriptItem
	c.apply(t, func(c *conversation) error {
		var err error
		first, err = c.beginMarkdown()
		if err != nil {
			return err
		}
		if err := c.Markdown(t.Context(), "<"); err != nil {
			return err
		}
		if err := c.Markdown(t.Context(), "!-- hidden -->"); err != nil {
			return err
		}
		return c.FinishMarkdown(t.Context(), nil)
	})
	c.apply(t, func(_ *conversation) error {
		if first.State() == base.ContentLive {
			t.Error("the answer remained live after EOF preparation")
		}
		return nil
	})
	for range retainedCompletedItems + 1 {
		c.apply(t, func(c *conversation) error {
			if err := c.Markdown(t.Context(), "a line\n\n"); err != nil {
				return err
			}
			return c.FinishMarkdown(t.Context(), nil)
		})
	}
	c.apply(t, func(c *conversation) error {
		if c.content.Len() != retainedCompletedItems || first.State() != base.ContentRetired || first.Receipt().Snapshot().ID == 0 {
			t.Error("settled content did not transfer to the publisher")
		}
		return nil
	})
}

func TestMoreOfTheSameAnswerDoesNotTakeTheReaderBackToIt(t *testing.T) {
	c := newTestConversation(t, 6, nil)
	c.apply(t, func(c *conversation) error {
		if err := c.User(t.Context(), strings.Repeat("a long prompt that fills the window\n", 20)); err != nil {
			return err
		}
		return c.Markdown(t.Context(), "first\n\n<!-- separator -->\n\n")
	})
	c.apply(t, func(c *conversation) error {
		c.content.Scroll().ToTop()
		if c.content.Scroll().FollowingEnd() {
			t.Error("the reader is at the end after scrolling to the top")
		}
		return c.Markdown(t.Context(), "second\n\nthird\n")
	})
	c.apply(t, func(c *conversation) error {
		if c.content.Scroll().FollowingEnd() {
			t.Error("more of the same answer took the reader back to the end of it")
		}
		return nil
	})
}

func TestTextThatTurnedOutToBeACommentStopsBeingOnTheScreen(t *testing.T) {
	for _, settle := range []string{"open", "flushed"} {
		t.Run(settle, func(t *testing.T) {
			c := newTestConversation(t, 8, nil)
			c.apply(t, func(c *conversation) error { return c.Markdown(t.Context(), "<") })
			c.host.Shows(t, "<")
			c.apply(t, func(c *conversation) error {
				if err := c.Markdown(t.Context(), "!-- hidden -->"); err != nil {
					return err
				}
				if settle == "flushed" {
					return c.FinishMarkdown(t.Context(), nil)
				}
				return nil
			})
			c.host.Hides(t, "<")
		})
	}
}

func TestFinalRendererFailureCannotAnnounceRunSuccess(t *testing.T) {
	started := make(chan *agentBridge, 1)
	host, stop := runAgent(t, captureRuns{started})
	defer stop()
	host.Type("verify final document")
	host.Press(input.Enter)
	bridge := <-started
	failure := errors.New("final projection failed")
	settled := make(chan struct{})
	if err := bridge.dispatch.Invoke(t.Context(), func() error {
		a := bridge.owner
		a.conversation.render = func(context.Context, string) (grid.Drawable, error) {
			return nil, failure
		}
		if err := a.conversation.Markdown(t.Context(), "retained source"); err != nil {
			return err
		}
		a.finishRun(nil)
		return a.conversation.Settled(func(error) {
			label := a.outcome.label(a.theme, a.glyphs)
			if a.outcome.kind != outcomeFailed || !strings.Contains(label.Text, failure.Error()) || a.status.Doing != "" {
				t.Errorf("failed final renderer produced outcome %+v and activity %q", a.outcome, a.status.Doing)
			}
			close(settled)
		})
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-settled:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
}

func TestSourceErrorDoesNotPoisonTheNextRun(t *testing.T) {
	started := make(chan *agentBridge, 2)
	host, stop := runAgent(t, captureRuns{started})
	defer stop()
	host.Type("first")
	host.Press(input.Enter)
	first := <-started
	settled := make(chan struct{})
	if err := first.dispatch.Invoke(t.Context(), func() error {
		a := first.owner
		a.accept(program.ByteBatch{Data: make([]byte, (1<<20)+1)})
		return a.conversation.Settled(func(error) {
			if a.run != nil || a.outcome.kind != outcomeFailed || a.status.Doing != "" {
				t.Errorf("oversized first run: outcome=%+v activity=%q", a.outcome, a.status.Doing)
			}
			a.startRun("independent next run")
			if a.run == nil || a.outcome != (sessionOutcome{}) {
				t.Errorf("previous failure leaked to next run: outcome=%+v activity=%q", a.outcome, a.status.Doing)
			}
			close(settled)
		})
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-settled:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
}
