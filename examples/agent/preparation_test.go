package main

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestMarkdownPreparationLeavesTheOwnerResponsive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestConversation(t, 8, nil)
		release := make(chan struct{})
		defer close(release)
		entered := make(chan struct{}, 1)
		returned := make(chan error, 1)
		go func() {
			returned <- c.runtime.Dispatcher().Invoke(t.Context(), func() error {
				c.conversation.render = func(context.Context, string) (grid.Drawable, error) {
					entered <- struct{}{}
					<-release
					return preparedAnswer{}, nil
				}
				return c.conversation.Markdown(t.Context(), "a complete source snapshot")
			})
		}()
		synctest.Wait()
		select {
		case err := <-returned:
			if err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("Markdown kept the UI owner inside the blocked renderer")
		}
		select {
		case <-entered:
		default:
			t.Fatal("the renderer never started")
		}
		if err := c.runtime.Dispatcher().Invoke(t.Context(), func() error {
			c.conversation.content.Scroll().ToTop()
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}

type preparedAnswer struct{}

func (preparedAnswer) HeightForWidth(int) int { return 1 }
func (preparedAnswer) Draw(view grid.View)    { view.Text(0, 0, "prepared answer", grid.Style{}) }

type gatedLayout struct {
	width   atomic.Int64
	resize  chan int
	release <-chan struct{}
}

func (g *gatedLayout) PrepareWidth(ctx context.Context, width int) error {
	if previous := g.width.Load(); previous != 0 && previous != int64(width) {
		g.resize <- width
		select {
		case <-g.release:
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	g.width.Store(int64(width))
	return nil
}

func (g *gatedLayout) PreparedLayout(width int) (grid.Drawable, bool) {
	if g.width.Load() != int64(width) {
		return nil, false
	}
	return preparedAnswer{}, true
}

func (g *gatedLayout) HeightForWidth(width int) int {
	if g.width.Load() != int64(width) {
		panic("cold layout reached the UI owner")
	}
	return 1
}

func (g *gatedLayout) Draw(view grid.View) {
	width, _ := view.Size()
	g.HeightForWidth(width)
	preparedAnswer{}.Draw(view)
}

func TestResizePreparesTheNewWidthBeforePainting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestConversation(t, 8, nil)
		release := make(chan struct{})
		var released bool
		defer func() {
			if !released {
				close(release)
			}
		}()
		body := &gatedLayout{resize: make(chan int, 1), release: release}
		c.apply(t, func(c *conversation) error {
			c.render = func(context.Context, string) (grid.Drawable, error) { return body, nil }
			return c.Markdown(t.Context(), "retained answer")
		})
		c.host.Resize(20, 8)
		synctest.Wait()
		select {
		case width := <-body.resize:
			if width != 20 {
				t.Errorf("layout requested width %d", width)
			}
		default:
			t.Fatal("resize did not schedule preparation")
		}
		if err := c.runtime.Dispatcher().Invoke(t.Context(), func() error {
			if width, _ := c.runtime.Size(); width != 20 {
				t.Errorf("owner did not receive resize: %d", width)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		close(release)
		released = true
		synctest.Wait()
		if _, err := c.host.Repaint(t.Context()); err != nil {
			t.Fatal(err)
		}
		c.host.Shows(t, "prepared answer")
	})
}
