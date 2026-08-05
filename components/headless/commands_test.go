package headless_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

func registry() *headless.Commands {
	var c headless.Commands
	for _, cmd := range []headless.Command{
		{Name: "new-session", Title: "start again"},
		{Name: "clear", Title: "empty the screen", Aliases: []string{"cls"}},
		{Name: "model", Title: "choose a model", Takes: true},
		{Name: "quit", Title: "leave"},
	} {
		c.Add(cmd)
	}
	return &c
}

func names(found []headless.Found) []string {
	out := make([]string, len(found))
	for i, f := range found {
		out[i] = f.Command.Name
	}
	return out
}

// TestCommandsFindWhatWasRemembered. A user who knows a command types its beginning;
// one who does not types what they remember, which is rarely the beginning.
func TestCommandsFindWhatWasRemembered(t *testing.T) {
	c := registry()
	for _, tc := range []struct{ query, want string }{
		{"new", "new-session"},
		{"sess", "new-session"},
		{"ns", "new-session"},
		{"clr", "clear"},
		{"q", "quit"},
	} {
		got := names(c.Find(tc.query))
		if len(got) == 0 || got[0] != tc.want {
			t.Errorf("%q found %v, want %q first", tc.query, got, tc.want)
		}
	}
}

func TestCommandsFindNothingForNonsense(t *testing.T) {
	if got := registry().Find("zzzzq"); len(got) != 0 {
		t.Errorf("found %v", names(got))
	}
}

func TestCommandsMatchAnAliasWithoutClaimingToMatchTheName(t *testing.T) {
	// An alias match has nothing to underline in the name being shown, which is
	// honest: the user typed something else.
	c := registry()
	found := c.Find("cls")
	if len(found) == 0 || found[0].Command.Name != "clear" {
		t.Fatalf("found %v", names(found))
	}
	if len(found[0].At) != 0 {
		t.Errorf("an alias match claimed to match the name at %v", found[0].At)
	}
}

func TestCommandsUnderlineWhereTheNameMatched(t *testing.T) {
	found := registry().Find("clear")
	if len(found) == 0 {
		t.Fatal("nothing found")
	}
	if got := len(found[0].At); got != len("clear") {
		t.Errorf("the match points at %d places, want %d", got, len("clear"))
	}
}

// TestCommandsPutTheRecentOnesFirst. The command somebody ran a moment ago is
// overwhelmingly the one they want next, and no amount of scoring the names finds
// that out.
func TestCommandsPutTheRecentOnesFirst(t *testing.T) {
	c := registry()
	if got := names(c.Find("")); got[0] != "new-session" {
		t.Errorf("with nothing used, the list starts %v, want registration order", got)
	}

	c.Used("quit")
	c.Used("model")
	got := names(c.Find(""))
	if got[0] != "model" || got[1] != "quit" {
		t.Errorf("the list starts %v, want the two most recent first", got[:2])
	}
	// And nothing is listed twice.
	if len(got) != c.Len() {
		t.Errorf("the list has %d entries for %d commands", len(got), c.Len())
	}
}

func TestCommandsIgnoreUseOfSomethingUnregistered(t *testing.T) {
	c := registry()
	c.Used("nonexistent")
	if got := names(c.Find("")); len(got) != c.Len() {
		t.Errorf("the list has %d entries for %d commands: %v", len(got), c.Len(), got)
	}
}

func TestCommandsReplaceByName(t *testing.T) {
	c := registry()
	before := c.Len()
	c.Add(headless.Command{Name: "clear", Title: "something else"})
	if c.Len() != before {
		t.Errorf("adding a command of an existing name made %d of them", c.Len())
	}
	cmd, ok := c.Lookup("clear")
	if !ok || cmd.Title != "something else" {
		t.Errorf("lookup found %+v (%v)", cmd, ok)
	}
	c.Add(headless.Command{})
	if c.Len() != before {
		t.Errorf("a command with no name was registered")
	}
}

func TestCommandsRemove(t *testing.T) {
	c := registry()
	c.Used("quit")
	if !c.Remove("quit") {
		t.Fatal("removing a registered command reported false")
	}
	if _, ok := c.Lookup("quit"); ok {
		t.Error("it is still there")
	}
	// And it is gone from the recent list too, or an empty query would list a
	// command that does not exist.
	for _, name := range names(c.Find("")) {
		if name == "quit" {
			t.Error("the recent list still holds it")
		}
	}
	if c.Remove("quit") {
		t.Error("removing it twice reported true")
	}
}

func TestLookupTakesAnAlias(t *testing.T) {
	if _, ok := registry().Lookup("cls"); !ok {
		t.Error("an alias did not find its command")
	}
	if _, ok := registry().Lookup("nothing"); ok {
		t.Error("a name nobody registered found something")
	}
}

func TestParseSplitsACommandFromItsArgument(t *testing.T) {
	for _, tc := range []struct {
		line      string
		name, arg string
		ok        bool
	}{
		{line: "/clear", name: "clear", ok: true},
		{line: "/model gpt-5", name: "model", arg: "gpt-5", ok: true},
		{line: "/model   spaced  out  ", name: "model", arg: "spaced  out", ok: true},
		{line: "/", ok: true},
		{line: "not a command", ok: false},
		{line: "", ok: false},
		{line: " /clear", ok: false},
	} {
		name, arg, ok := headless.Parse(tc.line)
		if ok != tc.ok || name != tc.name || arg != tc.arg {
			t.Errorf("%q = (%q, %q, %v), want (%q, %q, %v)", tc.line, name, arg, ok, tc.name, tc.arg, tc.ok)
		}
	}
}

func TestFindIgnoresTheSlash(t *testing.T) {
	// A palette is fed the whole line, slash and all.
	if got := names(registry().Find("/clear")); len(got) == 0 || got[0] != "clear" {
		t.Errorf("found %v", got)
	}
}

func TestCommandsRememberOnlyRecently(t *testing.T) {
	// Past a handful, "recently" stops meaning anything and the list is just the
	// registry again.
	var c headless.Commands
	for i := range 40 {
		c.Add(headless.Command{Name: string(rune('a'+i%26)) + string(rune('0'+i/26))})
	}
	for i := range 40 {
		c.Used(string(rune('a'+i%26)) + string(rune('0'+i/26)))
	}
	got := names(c.Find(""))
	if len(got) != c.Len() {
		t.Errorf("the list has %d entries for %d commands", len(got), c.Len())
	}
	// The most recent is still first, however many were used.
	if got[0] != string(rune('a'+39%26))+string(rune('0'+39/26)) {
		t.Errorf("the list starts with %q, want the last one used", got[0])
	}
}
