package headless_test

import (
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
)

type callbackClipboard struct{ copy func(string) bool }

func (c callbackClipboard) Copy(text string) bool { return c.copy(text) }
func (callbackClipboard) Paste() bool             { return false }

func TestCutCannotDeleteASelectionChangedDuringCopy(t *testing.T) {
	for _, change := range []string{"content", "selection"} {
		t.Run(change, func(t *testing.T) {
			var e headless.Editor
			e.SetText("first second")
			e.SelectAll()
			copied := ""
			e.Clipboard = callbackClipboard{copy: func(text string) bool {
				copied = text
				if change == "content" {
					e.SetText("replacement")
					e.SelectAll()
				} else {
					e.SetCursor(0, len("first"))
				}
				return true
			}}
			if e.Cut() {
				t.Fatal("cut deleted a selection that was not copied")
			}
			want := "first second"
			if change == "content" {
				want = "replacement"
			}
			if e.Text() != want || copied != "first second" {
				t.Fatalf("text=%q copied=%q", e.Text(), copied)
			}
		})
	}
}

func TestControlledTextSettlesBeforeAReentrantCursorMove(t *testing.T) {
	value := &transformingAccessor[string]{value: "hello"}
	field := headless.NewText(headless.TextConfig{Value: value})
	field.Sync()
	revision := field.Revision()
	value.transform = func(string) string {
		field.SetCursor(0)
		return "hello"
	}
	field.SetText("declined")
	if field.Text() != "hello" || field.Cursor() != 0 || field.Revision() != revision {
		t.Fatalf("text=%q cursor=%d revision=%d, want hello at 0 and revision %d",
			field.Text(), field.Cursor(), field.Revision(), revision)
	}
}

func TestControlledTextAccessorPanicCannotPublishARejectedEdit(t *testing.T) {
	value := &transformingAccessor[string]{value: "seed"}
	field := headless.NewText(headless.TextConfig{Value: value})
	field.Handle(input.Key{Text: "x"})
	field.Do(headless.Undo)
	revision := field.Revision()
	value.transform = func(string) string { panic("write failed") }
	func() {
		defer func() {
			if got := recover(); got != "write failed" {
				t.Fatalf("accessor panic = %v", got)
			}
		}()
		field.Handle(input.Key{Text: "!"})
	}()
	if field.Text() != "seed" || field.Revision() != revision {
		t.Fatalf("aborted edit published text=%q revision=%d, want seed at revision %d", field.Text(), field.Revision(), revision)
	}
	value.transform = nil
	if !field.Do(headless.Redo) || field.Text() != "seedx" {
		t.Fatal("aborted edit discarded the previous redo history")
	}
}

func TestControlledTextAccessorPanicSettlesTheAcceptedValue(t *testing.T) {
	for _, accepted := range []string{"requested", "normalized"} {
		t.Run(accepted, func(t *testing.T) {
			value := &publishingValue[string]{value: "seed"}
			field := headless.NewText(headless.TextConfig{Value: value})
			value.set = func(string) {
				value.value = accepted
				panic("write failed")
			}
			func() {
				defer func() {
					if got := recover(); got != "write failed" {
						t.Fatalf("accessor panic = %v", got)
					}
				}()
				field.SetText("requested")
			}()
			if field.Text() != accepted {
				t.Fatalf("answer = %q, want %q", field.Text(), accepted)
			}
			value.set = nil
			if !field.Do(headless.Undo) || field.Text() != "seed" {
				t.Fatal("accepted edit lost its undo history")
			}
			if !field.Do(headless.Redo) || field.Text() != accepted {
				t.Fatal("redo did not restore the accepted value")
			}
		})
	}
}

func TestControlledTextAccessorPanicCannotSettleANewerOwner(t *testing.T) {
	old := &publishingValue[string]{value: "old"}
	latest := &publishingValue[string]{value: "new"}
	field := headless.NewText(headless.TextConfig{Value: old})
	old.set = func(string) {
		field.SetBinding(latest)
		field.SetText("new answer")
		field.SetCursor(0)
		panic("old write failed")
	}
	func() {
		defer func() {
			if got := recover(); got != "old write failed" {
				t.Fatalf("accessor panic = %v", got)
			}
		}()
		field.SetText("old request")
	}()
	if field.Text() != "new answer" || field.Cursor() != 0 {
		t.Fatalf("old operation changed new owner's text=%q cursor=%d", field.Text(), field.Cursor())
	}
	if !field.Do(headless.Undo) || field.Text() != "new" {
		t.Fatal("old settlement erased the new owner's history")
	}
}

func TestControlledTextAccessorGoexitCannotPublishARejectedEdit(t *testing.T) {
	value := &transformingAccessor[string]{value: "seed"}
	field := headless.NewText(headless.TextConfig{Value: value})
	field.Handle(input.Key{Text: "x"})
	field.Do(headless.Undo)
	revision := field.Revision()
	value.transform = func(string) string {
		runtime.Goexit()
		return "unreachable"
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		field.Handle(input.Key{Text: "!"})
	}()
	<-done
	if field.Text() != "seed" || field.Revision() != revision {
		t.Fatalf("aborted edit published text=%q revision=%d, want seed at revision %d", field.Text(), field.Revision(), revision)
	}
	value.transform = nil
	if !field.Do(headless.Redo) || field.Text() != "seedx" {
		t.Fatal("aborted edit discarded the previous redo history")
	}
}

type publishingValue[T any] struct {
	value  T
	writes int
	set    func(T)
}

func (v *publishingValue[T]) Value() T { return v.value }
func (v *publishingValue[T]) Set(value T) {
	v.value = value
	v.writes++
	if v.set != nil {
		v.set(value)
	}
}

func TestControlledTextCannotAdvanceAfterSyncStartedANewerEdit(t *testing.T) {
	value := &publishingValue[string]{value: "one\ntwo"}
	field := headless.NewText(headless.TextConfig{Value: value})
	value.set = func(string) {
		value.set = nil
		field.SetText("new answer")
	}
	field.SetText("obsolete answer")
	if field.Text() != "new answer" || value.writes != 2 {
		t.Fatalf("answer=%q writes=%d, want new answer after two writes", field.Text(), value.writes)
	}
}

func TestControlledTextCannotOfferAnEditAlreadyOwnedByAReentrantCall(t *testing.T) {
	value := &publishingValue[string]{value: "first"}
	field := headless.NewText(headless.TextConfig{Value: value})
	field.Do(headless.SelectAll)
	field.Clipboard = callbackClipboard{copy: func(string) bool {
		field.SetText("new answer")
		return true
	}}
	if !field.Do(headless.Cut) {
		t.Fatal("cut action was not recognized")
	}
	if field.Text() != "new answer" || value.writes != 1 {
		t.Fatalf("answer=%q writes=%d, want one accepted edit", field.Text(), value.writes)
	}
}

func TestControlledTextCannotReconcileANewerOwnerFromTheOldAccessor(t *testing.T) {
	old := &publishingValue[string]{value: "old"}
	latest := &publishingValue[string]{value: "new"}
	field := headless.NewText(headless.TextConfig{Value: old})
	field.Sync()
	old.set = func(string) {
		field.SetBinding(latest)
		field.SetText("new answer")
		field.SetCursor(0)
	}
	field.SetText("old request")
	if field.Text() != "new answer" || field.Cursor() != 0 || old.writes != 1 || latest.writes != 1 {
		t.Fatalf("answer=%q cursor=%d old writes=%d new writes=%d",
			field.Text(), field.Cursor(), old.writes, latest.writes)
	}
	if !field.Do(headless.Undo) || field.Text() != "new" {
		t.Fatal("old settlement erased the new owner's edit history")
	}
	field.Do(headless.Undo)
	if field.Text() != "new" {
		t.Fatal("new owner inherited the old owner's undo history")
	}
}

type functionalTextAccessor struct {
	read  func() string
	write func(string)
}

func (v functionalTextAccessor) Value() string { return v.read() }
func (v functionalTextAccessor) Set(s string)  { v.write(s) }

func TestTextBindingBoundarySupportsValueAndPointerAccessors(t *testing.T) {
	for _, pointer := range []bool{false, true} {
		t.Run(map[bool]string{false: "value", true: "pointer"}[pointer], func(t *testing.T) {
			value := "old"
			accessor := functionalTextAccessor{read: func() string { return value }, write: func(s string) { value = s }}
			var binding headless.Accessor[string] = accessor
			if pointer {
				binding = &accessor
			}
			field := headless.NewText(headless.TextConfig{Value: binding})
			field.SetText("new")
			if field.Text() != "new" || !field.Do(headless.Undo) || field.Text() != "old" {
				t.Fatal("functional accessor lost ordinary editing or undo semantics")
			}
			field.SetText("edited")
			latest := "edited"
			other := functionalTextAccessor{read: func() string { return latest }, write: func(s string) { latest = s }}
			field.SetBinding(other)
			field.Do(headless.Undo)
			if latest != "edited" {
				t.Fatal("the new binding inherited old undo history")
			}
		})
	}
}

func TestTextOwnerTransferDiscardsHistoryEvenWhenTheValuesAreEqual(t *testing.T) {
	old := &publishingValue[string]{value: "old"}
	field := headless.NewText(headless.TextConfig{Value: old})
	field.SetText("edited")
	latest := &publishingValue[string]{value: "edited"}
	field.SetBinding(latest)
	field.Sync()
	field.Do(headless.Undo)
	if field.Text() != "edited" || latest.writes != 0 {
		t.Fatalf("old history advanced the new owner: value=%q writes=%d", field.Text(), latest.writes)
	}
}

func TestTextSyncReadsTheCurrentOwnerAfterCanonicalization(t *testing.T) {
	old := &publishingValue[string]{value: "one\ntwo"}
	latest := &publishingValue[string]{value: "new answer"}
	field := headless.NewText(headless.TextConfig{Value: old})
	old.set = func(string) { field.SetBinding(latest) }
	field.Sync()
	if field.Text() != "new answer" || field.Cursor() != len("new answer") || latest.writes != 0 {
		t.Fatalf("projection=%q cursor=%d new owner writes=%d", field.Text(), field.Cursor(), latest.writes)
	}
}

func TestTextCannotApplyAnOldCommandToANewerOwner(t *testing.T) {
	old := &publishingValue[string]{value: "one\ntwo"}
	latest := &publishingValue[string]{value: "new answer"}
	field := headless.NewText(headless.TextConfig{Value: old})
	old.set = func(string) { field.SetBinding(latest) }
	field.SetText("obsolete answer")
	if field.Text() != "new answer" || latest.writes != 0 {
		t.Fatalf("new answer=%q writes=%d", field.Text(), latest.writes)
	}
}

func TestTextCutCannotWriteAnEditToANewerOwner(t *testing.T) {
	for _, mode := range []string{"accessor", "local"} {
		t.Run(mode, func(t *testing.T) {
			old := "old answer"
			latest := &publishingValue[string]{value: "new answer"}
			field := headless.NewText(headless.TextConfig{Value: headless.Bind(&old)})
			field.Do(headless.SelectAll)
			field.Clipboard = callbackClipboard{copy: func(string) bool {
				if mode == "accessor" {
					field.SetBinding(latest)
				} else {
					field.SetBinding(nil)
				}
				return true
			}}
			field.Do(headless.Cut)
			want := "new answer"
			if mode == "local" {
				want = "old answer"
			}
			if field.Text() != want || latest.writes != 0 || old != "old answer" {
				t.Fatalf("answer=%q new writes=%d old=%q", field.Text(), latest.writes, old)
			}
		})
	}
}
