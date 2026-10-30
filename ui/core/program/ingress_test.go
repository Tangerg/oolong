package program_test

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestByteIngressPreservesDataAndFinalResultOrder(t *testing.T) {
	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })

	cause := errors.New("source failed")
	var batches []program.ByteBatch
	ingress, err := program.NewByteIngress(program.ByteIngressConfig{Dispatcher: r.root.runtime.Dispatcher(), Limit: 8, Consume: func(batch program.ByteBatch) {
		batches = append(batches, batch)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := ingress.Write([]byte("abc")); n != 3 || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if n, err := ingress.Write([]byte("def")); n != 3 || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if err := ingress.CloseWithError(cause); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ingress.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("ingress did not deliver its final batch")
	}

	var data strings.Builder
	finals := 0
	for _, batch := range batches {
		data.Write(batch.Data)
		if batch.Final {
			finals++
			if !errors.Is(batch.Err, cause) {
				t.Errorf("final error = %v, want source cause", batch.Err)
			}
		}
	}
	if got := data.String(); got != "abcdef" {
		t.Fatalf("data = %q, want abcdef", got)
	}
	if finals != 1 || !batches[len(batches)-1].Final {
		t.Fatalf("final batches = %d, batches = %+v", finals, batches)
	}

	r.quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}

func TestByteIngressNormalizesOnlyBareEOF(t *testing.T) {
	cause := errors.New("source failed")
	wrapped := fmt.Errorf("source: %w", io.EOF)
	joined := errors.Join(io.EOF, cause)
	for _, test := range []struct {
		name   string
		result error
		want   error
	}{
		{name: "bare", result: io.EOF},
		{name: "wrapped", result: wrapped, want: wrapped},
		{name: "joined", result: joined, want: joined},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := start(t, nil)
			r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
			final := make(chan error, 1)
			ingress, err := program.NewByteIngress(program.ByteIngressConfig{
				Dispatcher: r.root.runtime.Dispatcher(), Limit: 8,
				Consume: func(batch program.ByteBatch) {
					if batch.Final {
						final <- batch.Err
					}
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := ingress.CloseWithError(test.result); err != nil {
				t.Fatal(err)
			}
			select {
			case <-ingress.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("ingress did not deliver its terminal result")
			}
			r.quit()
			if err := r.wait(); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-final:
				if !errors.Is(got, test.want) {
					t.Fatalf("final result = %v, want %v", got, test.want)
				}
			default:
				t.Fatal("ingress completed without delivering its terminal result")
			}
		})
	}
}

func TestByteIngressBackpressuresTheProducer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHost(t)
		root := &blockingComponent{entered: make(chan struct{}), release: make(chan struct{})}
		done := make(chan error, 1)
		ready := make(chan struct{})
		go func() {
			done <- runFixture(t.Context(), program.ScreenConfig{
				Host: h, Build: func(runtime *program.Runtime) (scene.Element, error) {
					root.runtime = runtime
					close(ready)
					return fixtureElement(root), nil
				},
			})
		}()
		<-ready

		var received atomic.Int64
		ingress, err := program.NewByteIngress(program.ByteIngressConfig{Dispatcher: root.runtime.Dispatcher(), Limit: 4, Consume: func(batch program.ByteBatch) {
			received.Add(int64(len(batch.Data)))
		}})
		if err != nil {
			t.Fatal(err)
		}
		h.send(input.Key{Code: input.Character, Rune: 'x'})
		<-root.entered
		if _, err := ingress.Write([]byte("abcd")); err != nil {
			t.Fatal(err)
		}

		written := make(chan error, 1)
		go func() {
			_, err := ingress.Write([]byte("e"))
			written <- err
		}()
		synctest.Wait()
		select {
		case err := <-written:
			t.Fatalf("write escaped backpressure with %v", err)
		default:
		}

		close(root.release)
		select {
		case err := <-written:
			if err != nil {
				t.Fatalf("write after room opened: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("write remained blocked after the owner drained data")
		}
		if err := ingress.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-ingress.Done():
		case <-time.After(5 * time.Second):
			t.Fatal("ingress did not finish")
		}
		if got := received.Load(); got != 5 {
			t.Fatalf("received %d bytes, want 5", got)
		}

		onLoop(t, root.runtime, root.runtime.Quit)
		if err := <-done; err != nil {
			t.Fatalf("program: %v", err)
		}
	})
}

func TestByteIngressCancellationUnblocksAndReleasesTheProducer(t *testing.T) {
	h := newHost(t)
	root := &blockingComponent{entered: make(chan struct{}), release: make(chan struct{}), quit: true}
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		done <- runFixture(t.Context(), program.ScreenConfig{
			Host: h, Build: func(runtime *program.Runtime) (scene.Element, error) {
				root.runtime = runtime
				close(ready)
				return fixtureElement(root), nil
			},
		})
	}()
	<-ready

	var consumed atomic.Bool
	ingress, err := program.NewByteIngress(program.ByteIngressConfig{Dispatcher: root.runtime.Dispatcher(), Limit: 4, Consume: func(program.ByteBatch) {
		consumed.Store(true)
	}})
	if err != nil {
		t.Fatal(err)
	}
	h.send(input.Key{Code: input.Character, Rune: 'x'})
	<-root.entered
	if _, err := ingress.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}

	written := make(chan error, 1)
	go func() {
		_, err := ingress.Write([]byte("e"))
		written <- err
	}()
	close(root.release)
	if err := <-done; err != nil {
		t.Fatalf("program: %v", err)
	}
	select {
	case err := <-written:
		if !errors.Is(err, program.ErrStopped) {
			t.Fatalf("blocked Write error = %v, want ErrStopped", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked Write did not stop with its owner")
	}
	if consumed.Load() {
		t.Fatal("pending data was consumed after the owner stopped")
	}
	if _, err := ingress.Write([]byte("later")); !errors.Is(err, program.ErrStopped) {
		t.Fatalf("Write after stop = %v, want ErrStopped", err)
	}
}

func TestByteIngressRefusesInvalidConstructionAndRepeatedClose(t *testing.T) {
	var dispatch program.Dispatcher
	if _, err := program.NewByteIngress(program.ByteIngressConfig{Dispatcher: dispatch, Limit: 1, Consume: func(program.ByteBatch) {}}); !errors.Is(err, program.ErrStopped) {
		t.Fatalf("zero dispatcher error = %v, want ErrStopped", err)
	}

	r := start(t, nil)
	r.until("the opening frame", func() bool { return r.host.frames.size() > 0 })
	if _, err := program.NewByteIngress(program.ByteIngressConfig{Dispatcher: r.root.runtime.Dispatcher(), Limit: 0, Consume: func(program.ByteBatch) {}}); err == nil {
		t.Fatal("a zero byte limit was accepted")
	}
	if _, err := program.NewByteIngress(program.ByteIngressConfig{Dispatcher: r.root.runtime.Dispatcher(), Limit: 1, Consume: nil}); err == nil {
		t.Fatal("a nil consumer was accepted")
	}
	ingress, err := program.NewByteIngress(program.ByteIngressConfig{Dispatcher: r.root.runtime.Dispatcher(), Limit: 1, Consume: func(program.ByteBatch) {}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ingress.CloseWithError(io.EOF); err != nil {
		t.Fatal(err)
	}
	<-ingress.Done()
	if err := ingress.Close(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("second Close = %v, want io.ErrClosedPipe", err)
	}

	r.quit()
	if err := r.wait(); err != nil {
		t.Fatalf("program: %v", err)
	}
}
