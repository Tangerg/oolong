package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestElementTextStaysWithItsEditSnapshot(t *testing.T) {
	a := base.NewEditor(base.EditorConfig{})
	mark := a.InsertElement(fileChip, "file")
	a.SetText("trap")
	if got := mark.Text(); got != "file" {
		t.Fatalf("saved mark read replacement document: %q", got)
	}
}

func TestElementRemovalAuthorityBelongsToTheSharedSession(t *testing.T) {
	session := base.NewEditSession(base.EditSessionConfig{})
	a := base.NewEditor(base.EditorConfig{Session: session})
	sibling := base.NewEditor(base.EditorConfig{Session: session})
	b := base.NewEditor(base.EditorConfig{})
	id := a.InsertElement(fileChip, "first").ID
	b.InsertElement(fileChip, "second")
	if b.RemoveElement(id) || b.Text() != "second " {
		t.Fatal("foreign editor removed an unrelated chip")
	}
	if !sibling.RemoveElement(id) {
		t.Fatal("peer view cannot remove its shared chip")
	}
}
