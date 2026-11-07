package docprep_test

import (
	"context"
	"errors"
	"io"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/examples/internal/docprep"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
)

type queueFixture struct {
	queue     *docprep.Queue
	runtime   *program.Runtime
	scope     *program.Scope
	host      *queueHost
	failures  []error
	onFailure func(error)
}

type queueHost struct {
	events chan input.Event
	writer *term.Writer
}

func (h *queueHost) Input() program.EventSource  { return h }
func (h *queueHost) Events() <-chan input.Event  { return h.events }
func (h *queueHost) Err() error                  { return nil }
func (h *queueHost) Writer() program.FrameWriter { return h.writer }
func (h *queueHost) Size() (int, int, error)     { return 40, 8, nil }

func newQueueFixture(t *testing.T, resources int) *queueFixture {
	t.Helper()
	fixture := &queueFixture{host: &queueHost{
		events: make(chan input.Event, 1), writer: term.NewWriter(term.WriterConfig{Destination: io.Discard}),
	}}
	ready, done := make(chan struct{}), make(chan error, 1)
	app, err := program.NewScreen(program.ScreenConfig{
		Host: fixture.host, Resources: resources,
		Build: func(rt *program.Runtime) (scene.Element, error) {
			fixture.runtime = rt
			var scopeErr error
			fixture.scope, scopeErr = rt.NewScope(program.ScopeConfig{})
			if scopeErr != nil {
				return scene.Element{}, scopeErr
			}
			fixture.queue = docprep.New(docprep.Config{Runtime: rt, Scope: fixture.scope, Failed: func(err error) {
				fixture.failures = append(fixture.failures, err)
				if fixture.onFailure != nil {
					fixture.onFailure(err)
				}
			}})
			close(ready)
			return scene.NewElement(scene.ElementConfig{}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { done <- app.Run(ctx) }()
	select {
	case <-ready:
	case err := <-done:
		cancel()
		t.Fatalf("startup: %v", err)
	}
	t.Cleanup(func() {
		fixture.apply(t, func() error {
			fixture.queue.Close()
			fixture.scope.Close()
			return nil
		})
		cancel()
		if err := <-done; err != nil {
			t.Errorf("shutdown: %v", err)
		}
		if err := fixture.host.writer.Close(); err != nil {
			t.Errorf("writer shutdown: %v", err)
		}
	})
	return fixture
}

func (f *queueFixture) apply(t *testing.T, fn func() error) {
	t.Helper()
	if err := f.runtime.Dispatcher().Invoke(context.Background(), fn); err != nil {
		t.Fatal(err)
	}
}

func frameReady(t *testing.T, queue *docprep.Queue, width int, transcript *base.Transcript) bool {
	t.Helper()
	ready, err := queue.Ready(width, transcript)
	if err != nil {
		t.Error(err)
	}
	return ready
}

func newDocument(t *testing.T, source string, final bool) *content.StreamDocument {
	t.Helper()
	document, err := content.NewStreamDocument(content.StreamConfig{Format: "text"})
	if err == nil {
		err = document.Append([]byte(source))
	}
	if err == nil && final {
		err = document.End(nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return document
}

type line string

func (line) HeightForWidth(int) int   { return 1 }
func (body line) Draw(view grid.View) { view.Text(0, 0, string(body), grid.Style{}) }

func plain(_ context.Context, source string) (grid.Drawable, error) { return line(source), nil }

func newGate(t *testing.T) (<-chan struct{}, func()) {
	t.Helper()
	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	return gate, release
}

func TestQueueCoalescesSourceRevisionsAndPreservesEOF(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newQueueFixture(t, 0)
		gate, release := newGate(t)
		sources := make(chan string, 4)
		var running, peak atomic.Int32
		var render content.Renderer = func(_ context.Context, source string) (grid.Drawable, error) {
			active := running.Add(1)
			defer running.Add(-1)
			if active > peak.Load() {
				peak.Store(active)
			}
			sources <- source
			if source == "a" {
				<-gate
			}
			return line(source), nil
		}
		document := newDocument(t, "a", false)
		fixture.apply(t, func() error { return fixture.queue.Prepare(document, render, nil) })
		if got := <-sources; got != "a" {
			t.Fatal(got)
		}
		finished, after := 0, false
		fixture.apply(t, func() error {
			if err := document.Append([]byte("b")); err != nil {
				return err
			}
			if err := fixture.queue.Prepare(document, render, nil); err != nil {
				return err
			}
			if err := document.Append([]byte("c")); err != nil {
				return err
			}
			if err := document.End(nil); err != nil {
				return err
			}
			if err := fixture.queue.Prepare(document, render, func(err error) {
				finished++
				if err != nil {
					t.Error(err)
				}
			}); err != nil {
				return err
			}
			if err := fixture.queue.Prepare(document, render, nil); err != nil {
				return err
			}
			if err := fixture.queue.After(func() { after = true }); err != nil {
				return err
			}
			return nil
		})
		synctest.Wait()
		if len(sources) != 0 || running.Load() != 1 || after {
			t.Fatal("queued work overlapped the first renderer")
		}
		release()
		synctest.Wait()
		if got := <-sources; got != "abc" {
			t.Fatal(got)
		}
		fixture.apply(t, func() error {
			change := document.Current()
			if change.State != content.Final || change.PreparedRevision != change.Revision || finished != 1 || !after || peak.Load() != 1 || len(sources) != 0 {
				t.Errorf("change=%+v finished=%d after=%v peak=%d extra=%d", change, finished, after, peak.Load(), len(sources))
			}
			return nil
		})
	})
}

func TestClearWaitsForNoncooperativeWorkerBeforeReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newQueueFixture(t, 0)
		gate, release := newGate(t)
		started := make(chan string, 2)
		oldFinished, oldAfter, newFinished := false, false, false
		old := newDocument(t, "old", true)
		fresh := newDocument(t, "fresh", true)
		var render content.Renderer = func(_ context.Context, source string) (grid.Drawable, error) {
			started <- source
			if source == "old" {
				<-gate
			}
			return line(source), nil
		}
		fixture.apply(t, func() error {
			if err := fixture.queue.Prepare(old, render, func(error) { oldFinished = true }); err != nil {
				return err
			}
			if err := fixture.queue.After(func() { oldAfter = true }); err != nil {
				return err
			}
			return nil
		})
		<-started
		fixture.apply(t, func() error {
			fixture.queue.Clear()
			return fixture.queue.Prepare(fresh, render, func(error) { newFinished = true })
		})
		synctest.Wait()
		if len(started) != 0 {
			t.Fatal("Clear admitted a second running renderer")
		}
		release()
		synctest.Wait()
		if got := <-started; got != "fresh" {
			t.Fatal(got)
		}
		fixture.apply(t, func() error {
			if old.Current().PreparedRevision != 0 || oldFinished || oldAfter || !newFinished || fresh.Current().State != content.Final {
				t.Errorf("old=%+v fresh=%+v callbacks=%v/%v/%v", old.Current(), fresh.Current(), oldFinished, oldAfter, newFinished)
			}
			return nil
		})
	})
}

func TestFinalPreparationFailuresHaveOneCompletionOwner(t *testing.T) {
	cause := errors.New("renderer failed")
	for _, test := range []struct {
		name   string
		render content.Renderer
		want   error
	}{
		{"returned error", func(context.Context, string) (grid.Drawable, error) { return nil, cause }, cause},
		{"panic", func(context.Context, string) (grid.Drawable, error) { panic(cause) }, program.ErrTaskAborted},
		{"Goexit", func(context.Context, string) (grid.Drawable, error) {
			runtime.Goexit()
			return nil, program.ErrTaskAborted
		}, program.ErrTaskAborted},
		{"cancellation outcome", func(context.Context, string) (grid.Drawable, error) { return nil, context.Canceled }, context.Canceled},
		{"missing renderer", nil, content.ErrUnknownFormat},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fixture := newQueueFixture(t, 0)
				document := newDocument(t, "final", false)
				sourceErr := errors.New("source failed")
				finished, after := 0, false
				fixture.apply(t, func() error {
					if err := document.End(sourceErr); err != nil {
						return err
					}
					if err := fixture.queue.Prepare(document, test.render, func(err error) {
						finished++
						if !errors.Is(err, sourceErr) || !errors.Is(err, test.want) {
							t.Errorf("completion error=%v", err)
						}
					}); err != nil {
						return err
					}
					if err := fixture.queue.After(func() { after = true }); err != nil {
						return err
					}
					return nil
				})
				synctest.Wait()
				fixture.apply(t, func() error {
					if change := document.Current(); change.State != content.Failed || !errors.Is(change.PrepareError, test.want) || finished != 1 || !after || len(fixture.failures) != 0 {
						t.Errorf("change=%+v finished=%d after=%v failures=%v", change, finished, after, fixture.failures)
					}
					return nil
				})
			})
		})
	}
}

func TestStartRejectionSettlesAnAcceptedFinalRequest(t *testing.T) {
	for _, queued := range []bool{false, true} {
		name := "immediate"
		if queued {
			name = "after another Apply"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				resources := 1
				if queued {
					resources = 2
				}
				fixture := newQueueFixture(t, resources)
				gate, release := newGate(t)
				if queued {
					started := make(chan struct{})
					previous := newDocument(t, "old", false)
					fixture.apply(t, func() error {
						return fixture.queue.Prepare(previous, func(context.Context, string) (grid.Drawable, error) { close(started); <-gate; return line("old"), nil }, nil)
					})
					<-started
				}
				document := newDocument(t, "final", true)
				finished, after := 0, false
				fixture.apply(t, func() error {
					if err := fixture.queue.Prepare(document, plain, func(err error) {
						finished++
						if !errors.Is(err, program.ErrCapacity) {
							t.Errorf("completion=%v", err)
						}
					}); err != nil {
						return err
					}
					if err := fixture.queue.After(func() { after = true }); err != nil {
						return err
					}
					return nil
				})
				release()
				synctest.Wait()
				fixture.apply(t, func() error {
					if change := document.Current(); change.State != content.Failed || !errors.Is(change.PrepareError, program.ErrCapacity) || finished != 1 || !after || len(fixture.failures) != 0 {
						t.Errorf("change=%+v finished=%d after=%v failures=%v", change, finished, after, fixture.failures)
					}
					return nil
				})
			})
		})
	}
}

func TestPendingCapacityStillAcceptsTheLatestEOFForAnAdmittedDocument(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newQueueFixture(t, 0)
		gate, release := newGate(t)
		started := make(chan struct{})
		old := newDocument(t, "active", false)
		fixture.apply(t, func() error {
			return fixture.queue.Prepare(old, func(context.Context, string) (grid.Drawable, error) {
				close(started)
				<-gate
				return line("active"), nil
			}, nil)
		})
		<-started
		var documents []*content.StreamDocument
		finished := false
		fixture.apply(t, func() error {
			for range 256 {
				document := newDocument(t, "queued", false)
				err := fixture.queue.Prepare(document, plain, nil)
				if errors.Is(err, docprep.ErrCapacity) {
					break
				}
				if err != nil {
					return err
				}
				documents = append(documents, document)
			}
			if len(documents) == 0 || len(documents) == 256 {
				return errors.New("pending queue is not bounded")
			}
			final := documents[len(documents)-1]
			if err := final.End(nil); err != nil {
				return err
			}
			return fixture.queue.Prepare(final, plain, func(err error) { finished = err == nil })
		})
		release()
		synctest.Wait()
		fixture.apply(t, func() error {
			if !finished || documents[len(documents)-1].Current().State != content.Final {
				t.Error("admitted EOF was lost at capacity")
			}
			return nil
		})
	})
}

func TestAfterClearRevokesRemainingCallbacksAndPreparedFinalCompletesSynchronously(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newQueueFixture(t, 0)
		gate, release := newGate(t)
		document := newDocument(t, "final", true)
		var callbacks []string
		fixture.apply(t, func() error {
			if err := fixture.queue.Prepare(document, func(context.Context, string) (grid.Drawable, error) { <-gate; return line("final"), nil }, nil); err != nil {
				return err
			}
			if err := fixture.queue.After(func() { callbacks = append(callbacks, "first"); fixture.queue.Clear() }); err != nil {
				return err
			}
			if err := fixture.queue.After(func() { callbacks = append(callbacks, "revoked") }); err != nil {
				return err
			}
			return nil
		})
		release()
		synctest.Wait()
		fixture.apply(t, func() error {
			if !slices.Equal(callbacks, []string{"first"}) {
				t.Errorf("after callbacks=%v", callbacks)
			}
			called := false
			if err := fixture.queue.Prepare(document, plain, func(error) { called = true }); err != nil {
				return err
			}
			if !called {
				t.Error("settled final request lost its new completion callback")
			}
			fixture.queue.Close()
			if err := fixture.queue.After(func() { t.Error("closed queue delivered After") }); !errors.Is(err, program.ErrScopeClosed) {
				t.Errorf("closed After=%v", err)
			}
			if err := fixture.queue.Prepare(document, plain, nil); !errors.Is(err, program.ErrScopeClosed) {
				t.Errorf("closed Prepare=%v", err)
			}
			return nil
		})
	})
}

func TestDocumentNotificationCanRevokeTheRemainingCompletionCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newQueueFixture(t, 0)
		document := newDocument(t, "final", true)
		finished := false
		fixture.apply(t, func() error {
			document.Subscribe(func(change content.DocumentChange) {
				if change.State == content.Final {
					fixture.queue.Clear()
				}
			})
			return fixture.queue.Prepare(document, plain, func(error) { finished = true })
		})
		synctest.Wait()
		fixture.apply(t, func() error {
			if document.Current().State != content.Final || finished {
				t.Errorf("accepted state=%v completion after Clear=%v", document.Current().State, finished)
			}
			return nil
		})
	})
}
