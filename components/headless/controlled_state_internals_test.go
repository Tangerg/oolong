package headless

import "testing"

type rejectingChoice struct{ value string }

func (r *rejectingChoice) Value() string { return r.value }
func (*rejectingChoice) Set(string)      {}

func TestSelectSettlesItsCursorToTheValueItsOwnerAccepted(t *testing.T) {
	value := &rejectingChoice{value: "a"}
	field := &Select[string]{Value: value}
	field.SetOptions(Options("a", "b"))
	_, _ = field.Chosen()

	if !field.Do(SelectNext) {
		t.Fatal("next choice was not handled")
	}
	if got := field.list.Selected(); got != 0 {
		t.Fatalf("internal cursor = %d, want the accepted owner choice at 0", got)
	}
}
