package scene_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestReadOnlyBorrowsOneCompositionAcrossAttachmentLifetimes(t *testing.T) {
	for _, attachment := range []string{"detached", "mounted"} {
		for _, exit := range []string{"return", "panic", "Goexit"} {
			t.Run(attachment+"/"+exit, func(t *testing.T) {
				a, b := scene.NewElement(scene.ElementConfig{}), scene.NewElement(scene.ElementConfig{})
				group := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", a), item("b", b)}})
				var mounted *scene.Root
				if attachment == "mounted" {
					mounted = root(t, group.Element())
				}
				returned := false
				done := make(chan struct{})
				go func() {
					defer close(done)
					defer func() {
						got := recover()
						if exit == "panic" && got != "projection interrupted" || exit != "panic" && got != nil {
							t.Errorf("projection exit=%v", got)
						}
					}()
					a.ReadOnly(func() {
						for _, element := range []scene.Element{a, b, group.Element()} {
							if err := element.CheckMutation(); !errors.Is(err, scene.ErrBusy) {
								t.Errorf("composition mutation=%v", err)
							}
						}
						if err := group.Set(item("b", b)); !errors.Is(err, scene.ErrBusy) {
							t.Errorf("borrow changed its parent relation: %v", err)
						}
						for _, err := range []error{a.SetPaint(nil), a.SetMeasure(nil), a.SetFocusable(nil)} {
							if !errors.Is(err, scene.ErrBusy) {
								t.Errorf("borrow changed an element capability: %v", err)
							}
						}
						if mounted == nil {
							if _, err := scene.NewRoot(scene.RootConfig{Element: group.Element()}); !errors.Is(err, scene.ErrBusy) {
								t.Errorf("mount moved a borrowed composition to another phase owner: %v", err)
							}
						} else if err := mounted.SetContent(scene.Element{}); !errors.Is(err, scene.ErrBusy) {
							t.Errorf("borrow detached its scene: %v", err)
						}
						b.ReadOnly(func() {
							if err := a.CheckMutation(); !errors.Is(err, scene.ErrBusy) {
								t.Errorf("nested borrow lost protection: %v", err)
							}
						})
						if err := b.CheckMutation(); !errors.Is(err, scene.ErrBusy) {
							t.Errorf("nested release ended the enclosing borrow: %v", err)
						}
						switch exit {
						case "panic":
							panic("projection interrupted")
						case "Goexit":
							runtime.Goexit()
						}
					})
					returned = true
				}()
				<-done
				if returned != (exit == "return") {
					t.Fatal("read-only borrow changed abnormal control flow")
				}
				if err := a.CheckMutation(); err != nil {
					t.Fatalf("borrow remained active: %v", err)
				}
				if err := group.Move(0, 1); err != nil {
					t.Fatalf("borrow prevented a later valid change: %v", err)
				}
				if mounted == nil {
					root(t, group.Element())
				}
			})
		}
	}
}

func TestElementMeasurementUsesTheSameReadOnlyOwner(t *testing.T) {
	for _, attachment := range []string{"detached", "mounted"} {
		t.Run(attachment, func(t *testing.T) {
			var element scene.Element
			element = scene.NewElement(scene.ElementConfig{Measure: func(layout.Axis, int) int {
				if err := element.CheckMutation(); !errors.Is(err, scene.ErrBusy) {
					t.Fatalf("measurement allowed owner mutation: %v", err)
				}
				return 3
			}})
			if attachment == "mounted" {
				root(t, element)
			}
			if extent := element.Measure(layout.Down, 20); extent != 3 {
				t.Fatalf("extent=%d", extent)
			}
			if err := element.CheckMutation(); err != nil {
				t.Fatalf("measurement retained its borrow: %v", err)
			}
		})
	}
}

func TestReadOnlyRequiresAnElementAndCallback(t *testing.T) {
	for _, call := range []func(){
		func() { scene.Element{}.ReadOnly(func() { t.Error("empty element invoked callback") }) },
		func() { scene.NewElement(scene.ElementConfig{}).ReadOnly(nil) },
	} {
		func() {
			defer func() {
				if got, ok := recover().(error); !ok || !errors.Is(got, scene.ErrInvalid) {
					t.Errorf("invalid borrow panic=%v", got)
				}
			}()
			call()
		}()
	}
}

func TestProjectionRequiresItsMountedElementAndActiveFrame(t *testing.T) {
	detached := scene.NewElement(scene.ElementConfig{})
	foreign := scene.NewElement(scene.ElementConfig{})
	root(t, foreign)
	var owner scene.Element
	var saved scene.Frame
	owner = scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		saved = frame
		if err := owner.CheckProjection(frame); err != nil {
			t.Fatalf("own Prepare refused projection: %v", err)
		}
		if err := detached.CheckProjection(frame); !errors.Is(err, scene.ErrDetached) {
			t.Fatalf("detached projection=%v", err)
		}
		if err := foreign.CheckProjection(frame); !errors.Is(err, scene.ErrInvalid) {
			t.Fatalf("foreign projection=%v", err)
		}
		owner.ReadOnly(func() {
			if err := owner.CheckProjection(frame); !errors.Is(err, scene.ErrBusy) {
				t.Fatalf("read-only projection=%v", err)
			}
		})
	}})
	r := root(t, owner)
	draw(t, r, 10, 1, 0)
	if err := owner.CheckProjection(saved); !errors.Is(err, scene.ErrInvalid) {
		t.Fatalf("expired projection=%v", err)
	}
	if err := (scene.Element{}).CheckProjection(saved); !errors.Is(err, scene.ErrDetached) {
		t.Fatalf("empty owner projection=%v", err)
	}
}

func TestPendingCandidateRefusesProjectionBeforeCallingApplication(t *testing.T) {
	for _, operation := range []string{"ReadOnly", "Measure", "Observe", "RequestFocusFirst"} {
		t.Run(operation, func(t *testing.T) {
			var snapshot scene.Snapshot[int]
			var candidate *scene.Prepared
			value, armed, called := 1, false, false
			callback := func() {
				if armed {
					called = true
					candidate.Commit(2)
				}
			}
			element := scene.NewElement(scene.ElementConfig{
				Paint:     func(frame scene.Frame) { snapshot.Stage(frame, value) },
				Measure:   func(layout.Axis, int) int { callback(); return 1 },
				Focusable: func() bool { callback(); return true },
				Semantics: func() scene.Semantic { callback(); return scene.Semantic{Role: scene.RoleStatus} },
			})
			r := root(t, element)
			draw(t, r, 10, 1, 1)
			value = 2
			var err error
			candidate, err = r.Prepare(grid.NewSurface(5, 1).View())
			if err != nil {
				t.Fatal(err)
			}
			defer candidate.Abort()
			armed = true
			if operation == "ReadOnly" || operation == "Measure" {
				func() {
					defer func() {
						got, _ := recover().(error)
						if !errors.Is(got, scene.ErrBusy) {
							t.Errorf("pending projection panic=%v", got)
						}
					}()
					if operation == "ReadOnly" {
						element.ReadOnly(callback)
					} else {
						element.Measure(layout.Down, 10)
					}
				}()
			} else {
				if operation == "Observe" {
					_, err = r.Observe(scene.Query{})
				} else {
					err = element.RequestFocusFirst()
				}
				if !errors.Is(err, scene.ErrBusy) {
					t.Errorf("pending projection error=%v", err)
				}
			}
			if called || snapshot.Value() != 1 {
				t.Fatal("pending projection called application code or committed borrowed geometry")
			}
			candidate.Commit(2)
			if snapshot.Value() != 2 {
				t.Fatal("refusing a new borrow prevented the candidate's valid commit")
			}
		})
	}
}

func TestNestedReadOnlyCannotStageOrRefineActiveGeometry(t *testing.T) {
	for _, operation := range []string{"Stage", "Set"} {
		t.Run(operation, func(t *testing.T) {
			var snapshot, other scene.Snapshot[int]
			var element scene.Element
			element = scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
				staged := snapshot.Stage(frame, 1)
				element.ReadOnly(func() {
					element.ReadOnly(func() {
						defer func() {
							got, _ := recover().(error)
							if !errors.Is(got, scene.ErrBusy) {
								t.Errorf("read-only geometry panic=%v", got)
							}
						}()
						if operation == "Stage" {
							other.Stage(frame, 2)
						} else {
							staged.Set(2)
						}
					})
					if err := element.CheckMutation(); !errors.Is(err, scene.ErrBusy) {
						t.Fatal("inner projection released its outer borrow")
					}
				})
				if staged.Value() != 1 {
					t.Fatal("refused refinement changed the candidate")
				}
				staged.Set(3)
				other.Stage(frame, 4)
			}})
			r := root(t, element)
			draw(t, r, 10, 1, 0)
			if snapshot.Value() != 3 || other.Value() != 4 {
				t.Fatal("read-only refusal prevented later valid preparation and commit")
			}
		})
	}
}
