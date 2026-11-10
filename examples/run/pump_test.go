package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/examples/internal/drawn"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestOutputRefusalPreservesTheAcceptedPrefixDeliveryFailure(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprintf("partial=%v", partial), func(t *testing.T) {
			host := uitest.New(t, uitest.Config{Width: 40, Height: 4})
			ready := make(chan struct{})
			done := make(chan error, 1)
			var r *runner
			go func() {
				app, err := program.NewInline(program.InlineConfig{Host: host, Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
					out, buildErr := text.NewDecoder(text.DecoderConfig{MaxLineBytes: 4})
					if buildErr != nil {
						return scene.Element{}, buildErr
					}
					r = &runner{
						runtime: runtime, keys: runnerKeys(), out: out,
						command: []string{os.Args[0], "-test.run=^TestCommandOutputDeliveryFixture$", "--", outputDeliveryFixture},
					}
					r.element = drawn.Element("run", r)
					close(ready)
					return r.Element(), nil
				}})
				if err == nil {
					err = app.Run(t.Context())
				}
				done <- err
			}()
			<-ready
			t.Cleanup(func() {
				if err := <-done; err != nil {
					t.Error(err)
				}
			})
			progress := make(chan struct{}, 1)
			pump := r.pump(progress)
			publicationFailure := errors.New("publication refused the accepted prefix")
			var offered [][]string
			pump.offer = func(lines []text.Line) ([]text.Line, error) {
				batch := make([]string, len(lines))
				for i, line := range lines {
					batch[i] = line.String()
				}
				offered = append(offered, batch)
				if partial && len(offered) == 1 && len(lines) > 0 {
					progress <- struct{}{}
					return lines[1:], publication.ErrCapacity
				}
				return lines, publicationFailure
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			err := pump.run(ctx)
			for _, cause := range []error{text.ErrLineLimit, publicationFailure} {
				if !errors.Is(err, cause) || strings.Count(err.Error(), cause.Error()) != 1 {
					t.Fatalf("outcome = %v, want %v exactly once", err, cause)
				}
			}
			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("source was not stopped by its output refusal: %v", err)
			}
			want := [][]string{{"one", "two"}}
			if partial {
				want = append(want, []string{"two"})
			}
			if !reflect.DeepEqual(offered, want) {
				t.Fatalf("offered prefix = %q, want %q", offered, want)
			}
			if err := r.runtime.Dispatcher().Invoke(t.Context(), func() error {
				if got := r.out.Open().String(); got != "1234" {
					return fmt.Errorf("retained %q, want accepted prefix", got)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

const outputDeliveryFixture = "--output-delivery-fixture"

func TestCommandOutputDeliveryFixture(_ *testing.T) {
	if !slices.Contains(os.Args, outputDeliveryFixture) {
		return
	}
	// One finite pipe write keeps both completed lines and the refusal in a read.
	fmt.Print("one\ntwo\n12345")
	timer := time.NewTimer(30 * time.Second)
	<-timer.C
	os.Exit(0)
}

func TestOutputLimitKeepsTheAcceptedPrefixUnderPublicationBackpressure(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 60, Height: 6})
	ready := make(chan struct{})
	done := make(chan error, 1)
	progress := make(chan struct{}, 1)
	var r *runner
	go func() {
		app, err := program.NewInline(program.InlineConfig{
			Host: host, Publication: publication.Limits{MaxItems: 1},
			Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
				out, buildErr := text.NewDecoder(text.DecoderConfig{MaxLineBytes: 4, MaxLineSpans: 2})
				if buildErr != nil {
					return scene.Element{}, buildErr
				}
				r = &runner{runtime: runtime, keys: runnerKeys(), out: out}
				r.element = drawn.Element("run", r)
				runtime.Publisher().Subscribe(func() {
					select {
					case progress <- struct{}{}:
					default:
					}
				})
				close(ready)
				return r.Element(), nil
			},
		})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()
	<-ready
	t.Cleanup(func() {
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	host.Shows(t, "ctrl+e: editor")
	pump := r.pump(progress)
	var published []string
	blocked := 0
	pump.offer = func(lines []text.Line) ([]text.Line, error) {
		rest, err := r.offer(lines)
		if errors.Is(err, publication.ErrCapacity) {
			blocked++
		}
		for _, line := range lines[:len(lines)-len(rest)] {
			published = append(published, line.String())
		}
		return rest, err
	}
	reads := 0
	err := pump.deliver(t.Context(), func() ([]text.Line, error) {
		reads++
		return pump.feed("one\ntwo\n12345\nnot accepted")
	})
	if !errors.Is(err, text.ErrLineLimit) {
		t.Fatalf("output admission = %v", err)
	}
	if reads != 1 || blocked == 0 || !slices.Equal(published, []string{"one", "two"}) {
		t.Fatalf("reads=%d, blocked=%d, published=%q", reads, blocked, published)
	}
	host.Shows(t, "1234")
	if err := r.runtime.Dispatcher().Invoke(t.Context(), func() error {
		if got := r.out.Open().String(); got != "1234" {
			return fmt.Errorf("retained %q, want accepted prefix", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

const outputBudgetFixture = "--output-budget-fixture"

func TestCommandOutputLimitsStopTheSourceAndReportFailure(t *testing.T) {
	for _, mode := range []string{"bytes", "exit", "spans", "EOF"} {
		t.Run(mode, func(t *testing.T) {
			host := &observedHost{Host: uitest.New(t, uitest.Config{Width: 80, Height: 6})}
			ready := make(chan struct{})
			done := make(chan error, 1)
			var r *runner
			go func() {
				app, err := program.NewInline(program.InlineConfig{Host: host, Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
					var buildErr error
					r, buildErr = newRunner(runtime, []string{os.Args[0], "-test.run=^TestCommandOutputBudgetFixture$", "--", outputBudgetFixture, mode})
					if buildErr != nil {
						return scene.Element{}, buildErr
					}
					close(ready)
					return r.Element(), nil
				}})
				if err == nil {
					err = app.Run(t.Context())
				}
				done <- err
			}()
			<-ready
			t.Cleanup(func() {
				if err := <-done; err != nil {
					t.Error(err)
				}
			})
			host.Until(t, "the source to stop at its output limit", func() bool {
				rang, notified := host.attention()
				return rang == 1 && notified == 1
			})
			host.Shows(t, "failed: text: decoded line limit exceeded")
			host.Shows(t, "open-prefix")
			if !strings.Contains(host.Frames(), "accepted-before-limit") {
				t.Fatal("completed prefix was lost")
			}
			if err := r.runtime.Dispatcher().Invoke(t.Context(), func() error {
				if r.outcome == nil || !errors.Is(r.outcome.err, text.ErrLineLimit) {
					return fmt.Errorf("final outcome lost admission cause: %v", r.outcome)
				}
				if strings.Count(r.outcome.err.Error(), text.ErrLineLimit.Error()) != 1 {
					return fmt.Errorf("final outcome duplicated admission cause: %w", r.outcome.err)
				}
				line := r.out.Open()
				if len(line.String()) > maxCommandLineBytes || len(line) > maxCommandLineSpans {
					return fmt.Errorf("retained %d bytes in %d spans", len(line.String()), len(line))
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCommandOutputBudgetFixture(_ *testing.T) {
	index := slices.Index(os.Args, outputBudgetFixture)
	if index < 0 {
		return
	}
	mode := os.Args[index+1]
	fmt.Print("accepted-before-limit\nopen-prefix")
	switch mode {
	case "bytes", "exit":
		fmt.Print(strings.Repeat("x", maxCommandLineBytes-len("open-prefix")+1))
		if mode == "exit" {
			os.Exit(0)
		}
	case "spans":
		for index := range maxCommandLineSpans {
			fmt.Printf("\x1b[%dmx", 31+index%2)
		}
	case "EOF":
		fmt.Print(strings.Repeat("x", maxCommandLineBytes-len("open-prefix")-1) + "\xe2\x82")
		os.Exit(0)
	}
	// Finite output followed by a bounded wait distinguishes cancellation from
	// merely noticing a naturally completed command; no unbounded stream is used.
	timer := time.NewTimer(30 * time.Second)
	<-timer.C
	os.Exit(0)
}
