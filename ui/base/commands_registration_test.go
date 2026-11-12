package base_test

import (
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestCommandsRefuseConflictingSpellingsBeforeChangingTheRegistry(t *testing.T) {
	for _, test := range []struct {
		name    string
		command base.Command
	}{
		{"name overlaps alias", base.Command{Name: "shared"}},
		{"alias overlaps name", base.Command{Name: "third", Aliases: []string{"first"}}},
		{"alias overlaps alias", base.Command{Name: "third", Aliases: []string{"shared"}}},
		{"replacement overlaps alias", base.Command{Name: "second", Aliases: []string{"unused", "shared"}}},
		{"empty alias", base.Command{Name: "third", Aliases: []string{""}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var commands base.Commands[int]
			commands.Add(base.Command{Name: "first", Aliases: []string{"shared"}}, 1)
			commands.Add(base.Command{Name: "second", Aliases: []string{"alternate"}}, 2)
			commands.Used("second")
			before := commands.Find("")
			rejected := false
			func() {
				defer func() { rejected = recover() != nil }()
				commands.Add(test.command, 3)
			}()
			if !rejected {
				t.Error("registration admitted a conflicting or empty spelling")
			}
			if !reflect.DeepEqual(commands.Find(""), before) {
				t.Error("refused registration changed descriptors or recency")
			}
			for _, spelling := range []string{"first", "shared", "second", "alternate"} {
				_, value, ok := commands.Lookup(spelling)
				want := 1
				if spelling == "second" || spelling == "alternate" {
					want = 2
				}
				if !ok || value != want {
					t.Errorf("Lookup(%q) = %d, %t; want %d", spelling, value, ok, want)
				}
			}
			for _, spelling := range []string{"third", "unused", ""} {
				if _, _, ok := commands.Lookup(spelling); ok {
					t.Errorf("refused registration exposed %q", spelling)
				}
			}
		})
	}
}

func TestCommandSpellingsTransferAfterTheirOwnerReleasesThem(t *testing.T) {
	var commands base.Commands[int]
	commands.Add(base.Command{Name: "old", Aliases: []string{"current"}}, 1)
	commands.Add(base.Command{Name: "old"}, 1)
	commands.Add(base.Command{Name: "current"}, 2)
	if command, value, ok := commands.Lookup("current"); !ok || command.Name != "current" || value != 2 {
		t.Fatalf("released spelling did not transfer: %+v, %d, %t", command, value, ok)
	}
	commands.Remove("current")
	if _, _, ok := commands.Lookup("current"); ok {
		t.Fatal("removing the new owner revived an older meaning")
	}
	if _, value, ok := commands.Lookup("old"); !ok || value != 1 {
		t.Fatal("transferring a spelling removed its previous command")
	}
}
