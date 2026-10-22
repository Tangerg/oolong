package grid_test

import (
	"bytes"
	"errors"
	"image"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
)

func rejectsPublicationReentry(t *testing.T, mutate func()) {
	t.Helper()
	rejected := false
	func() {
		defer func() { rejected = recover() != nil }()
		mutate()
	}()
	if !rejected {
		t.Error("a callback advanced the publication owner during another operation")
	}
}

func TestInlineAppendOwnsItsScratchAndPendingOutput(t *testing.T) {
	i := grid.NewInline(20, 2)
	if err := i.Append(func(v grid.View) bool {
		v.Text(0, 0, "outer", grid.Style{})
		rejectsPublicationReentry(t, func() {
			if err := i.Append(func(v grid.View) bool {
				v.Text(0, 0, "inner", grid.Style{})
				return false
			}); err != nil {
				t.Error(err)
			}
		})

		return false
	}); err != nil {
		t.Error(err)
	}
	var out bytes.Buffer
	if err := i.Flush(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "outer") || strings.Contains(out.String(), "inner") {
		t.Fatalf("callback replaced the outer publication: %q", out.String())
	}
}

func TestFailedDrawingReleasesPublicationWithoutPublishingIt(t *testing.T) {
	i := grid.NewInline(20, 2)
	if err := i.Append(appendLines("before")); err != nil {
		t.Error(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("drawing panic was suppressed")
			}
		}()
		if err := i.Append(func(v grid.View) bool {
			v.Text(0, 0, "failed", grid.Style{})
			panic("drawing failed")
		}); err != nil {
			t.Error(err)
		}
	}()
	if err := i.Append(appendLines("after")); err != nil {
		t.Error(err)
	}
	var out bytes.Buffer
	if err := i.Flush(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "before") || !strings.Contains(out.String(), "after") || strings.Contains(out.String(), "failed") {
		t.Fatalf("drawing failure changed pending publication: %q", out.String())
	}
}

func TestInterruptedDestinationWritePermanentlyEndsPublication(t *testing.T) {
	for name, canvas := range map[string]interface {
		regionCanvas
		Invalidate()
	}{"screen": grid.NewScreen(10, 2), "inline": grid.NewInline(10, 2)} {
		t.Run(name, func(t *testing.T) {
			canvas.Frame().Text(0, 0, "initial", grid.Style{})
			writer := &interruptedPublicationWriter{}
			func() {
				defer func() {
					if recover() == nil {
						t.Error("writer panic was suppressed")
					}
				}()
				_ = canvas.Flush(writer)
			}()
			if writer.Len() != 1 {
				t.Fatal("writer did not publish its prefix before interruption")
			}
			canvas.Invalidate()
			canvas.Frame().Text(0, 0, "replacement", grid.Style{})
			var out bytes.Buffer
			first := canvas.Flush(&out)
			if first == nil || out.Len() != 0 {
				t.Fatalf("interrupted delivery was replayed: %v %q", first, out.String())
			}
			if err := canvas.Flush(&out); !errors.Is(err, first) || out.Len() != 0 {
				t.Fatalf("interruption receipt changed: %v %q", err, out.String())
			}
		})
	}
}

func TestInterruptedInlineFinishPermanentlyEndsPublication(t *testing.T) {
	i := grid.NewInline(10, 2)
	i.Frame().Text(0, 0, "initial", grid.Style{})
	if err := i.Flush(io.Discard); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("writer panic was suppressed")
			}
		}()
		_ = i.Finish(&interruptedPublicationWriter{})
	}()
	var out bytes.Buffer
	if err := i.Finish(&out); err == nil || out.Len() != 0 {
		t.Fatalf("interrupted finish was replayed: %v %q", err, out.String())
	}
}

func TestInterruptedPrivateCompositionRetainsTheFullRepaint(t *testing.T) {
	for name, canvas := range map[string]interface {
		regionCanvas
		Invalidate()
	}{"screen": grid.NewScreen(10, 2), "inline": grid.NewInline(10, 2)} {
		t.Run(name, func(t *testing.T) {
			calls, interrupted := 0, false
			picture := publicationCallbackPainter{callback: func() {
				calls++
				if interrupted {
					panic("private composition failed")
				}
			}}
			canvas.Frame().Paint(grid.Area(0, 0, 2, 1), 1, picture)
			if err := canvas.Flush(io.Discard); err != nil {
				t.Fatal(err)
			}
			canvas.Invalidate()
			canvas.Frame().Paint(grid.Area(0, 0, 2, 1), 1, picture)
			interrupted = true
			func() {
				defer func() {
					if recover() == nil {
						t.Error("painter panic was suppressed")
					}
				}()
				_ = canvas.Flush(io.Discard)
			}()
			interrupted = false
			if err := canvas.Flush(io.Discard); err != nil {
				t.Fatal(err)
			}
			if calls != 4 {
				t.Fatalf("interrupted erase lost the required erase and repaint: calls=%d", calls)
			}
		})
	}
}

type interruptedPublicationWriter struct{ bytes.Buffer }

func (w *interruptedPublicationWriter) Write(p []byte) (int, error) {
	_, _ = w.Buffer.Write(p[:1])
	panic("partial write")
}

type publicationCallbackWriter struct{ callback func() }

func (w publicationCallbackWriter) Write(p []byte) (int, error) {
	w.callback()
	return len(p), nil
}

type publicationCallbackPainter struct{ callback func() }

func (p publicationCallbackPainter) Paint(io.Writer, image.Point) error { p.callback(); return nil }
func (p publicationCallbackPainter) Erase(io.Writer) error              { p.callback(); return nil }

func TestCanvasPublicationRejectsMutationsFromExternalCallbacks(t *testing.T) {
	for _, mode := range []string{"screen", "inline"} {
		for _, source := range []string{"writer", "painter"} {
			for _, operation := range []string{"Resize", "SetDepth", "SetGround", "Invalidate", "Frame", "Flush", "Print", "Append", "Break", "Finish"} {
				if mode == "screen" && (operation == "Print" || operation == "Append" || operation == "Break" || operation == "Finish") {
					continue
				}
				t.Run(mode+"/"+source+"/"+operation, func(t *testing.T) {
					var canvas interface {
						regionCanvas
						Size() (int, int)
						Resize(w, h int)
						SetDepth(d grid.Depth)
						SetGround(g grid.Ground)
						Invalidate()
					}
					var inline *grid.Inline
					if mode == "screen" {
						canvas = grid.NewScreen(10, 2)
					} else {
						inline = grid.NewInline(10, 2)
						canvas = inline
					}
					mutations := map[string]func(){
						"Resize":     func() { canvas.Resize(5, 1) },
						"SetDepth":   func() { canvas.SetDepth(grid.NoColor) },
						"SetGround":  func() { canvas.SetGround(grid.Ground{}) },
						"Invalidate": canvas.Invalidate,
						"Frame":      func() { canvas.Frame() },
						"Flush":      func() { _ = canvas.Flush(io.Discard) },
						"Print": func() {
							if err := inline.Print(content(1, lines("nested"))); err != nil {
								t.Error(err)
							}
						},
						"Append": func() {
							if err := inline.Append(appendLines("nested")); err != nil {
								t.Error(err)
							}
						},
						"Break":  func() { inline.Break() },
						"Finish": func() { _ = inline.Finish(io.Discard) },
					}
					calls := 0
					callback := func() {
						calls++
						if w, h := canvas.Size(); w != 10 || h != 2 {
							t.Fatalf("callback lost publication geometry: %dx%d", w, h)
						}
						rejectsPublicationReentry(t, mutations[operation])
					}
					view := canvas.Frame()
					view.Text(0, 0, "outer", grid.Style{})
					var writer io.Writer = &bytes.Buffer{}
					if source == "painter" {
						view.Paint(grid.Area(6, 0, 1, 1), 1, publicationCallbackPainter{callback: callback})
					} else {
						writer = publicationCallbackWriter{callback: callback}
					}
					if err := canvas.Flush(writer); err != nil {
						t.Fatal(err)
					}
					if calls != 1 {
						t.Fatalf("callback count = %d", calls)
					}
					view = canvas.Frame()
					view.Text(0, 0, "outer", grid.Style{})
					if source == "painter" {
						view.Paint(grid.Area(6, 0, 1, 1), 1, publicationCallbackPainter{callback: callback})
					}
					var second bytes.Buffer
					if err := canvas.Flush(&second); err != nil || second.Len() != 0 {
						t.Fatalf("settled frame changed again: %v %q", err, second.String())
					}
				})
			}
		}
	}
}
