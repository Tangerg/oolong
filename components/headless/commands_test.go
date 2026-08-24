package headless_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

func registry() *headless.Commands[string] {
	var c headless.Commands[string]
	for _, cmd := range []headless.Command{
		{Name: "new-session", Title: "start again"},
		{Name: "clear", Title: "empty the screen", Aliases: []string{"cls"}},
		{Name: "model", Title: "choose a model"},
		{Name: "quit", Title: "leave"},
	} {
		c.Add(cmd, "value:"+cmd.Name)
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

func TestCommandsBreakEqualSearchScoresByRecency(t *testing.T) {
	var commands headless.Commands[struct{}]
	commands.Add(headless.Command{Name: "alpha-one"}, struct{}{})
	commands.Add(headless.Command{Name: "alpha-two"}, struct{}{})
	commands.Used("alpha-two")

	got := names(commands.Find("alpha"))
	if len(got) != 2 || got[0] != "alpha-two" {
		t.Fatalf("equal-score search order = %v, want the most recently used first", got)
	}
}

func TestCommandsRankAliasMatchesWithoutPuttingThemBeforeNames(t *testing.T) {
	var commands headless.Commands[struct{}]
	commands.Add(headless.Command{Name: "first", Aliases: []string{"far-a-b"}}, struct{}{})
	commands.Add(headless.Command{Name: "second", Aliases: []string{"ab"}}, struct{}{})

	got := names(commands.Find("ab"))
	if len(got) != 2 || got[0] != "second" {
		t.Fatalf("alias search order = %v, want the best alias match first", got)
	}

	commands.Add(headless.Command{Name: "a---b"}, struct{}{})
	got = names(commands.Find("ab"))
	if len(got) != 3 || got[0] != "a---b" {
		t.Fatalf("name and alias search order = %v, want every name match before aliases", got)
	}
}

func TestCommandsRecordAliasUseUnderTheCanonicalName(t *testing.T) {
	c := registry()
	c.Used("cls")
	got := names(c.Find(""))
	if len(got) != c.Len() {
		t.Fatalf("using an alias produced %d rows for %d commands: %v", len(got), c.Len(), got)
	}
	if got[0] != "clear" {
		t.Fatalf("the alias moved %q to the front, want clear: %v", got[0], got)
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
	c.Add(headless.Command{Name: "clear", Title: "something else"}, "replacement")
	if c.Len() != before {
		t.Errorf("adding a command of an existing name made %d of them", c.Len())
	}
	cmd, value, ok := c.Lookup("clear")
	if !ok || cmd.Title != "something else" || value != "replacement" {
		t.Errorf("lookup found %+v, %q (%v)", cmd, value, ok)
	}
	c.Add(headless.Command{}, "ignored")
	if c.Len() != before {
		t.Errorf("a command with no name was registered")
	}
}

func TestCommandsOwnAliasesAcrossTheirBoundary(t *testing.T) {
	aliases := []string{"old-name"}
	var commands headless.Commands[string]
	commands.Add(headless.Command{Name: "current", Aliases: aliases}, "meaning")
	aliases[0] = "changed-outside"

	if _, _, ok := commands.Lookup("old-name"); !ok {
		t.Fatal("changing the Add input changed the registered aliases")
	}
	command, _, _ := commands.Lookup("current")
	command.Aliases[0] = "changed-result"
	if _, _, ok := commands.Lookup("old-name"); !ok {
		t.Fatal("changing a Lookup result changed the registered aliases")
	}

	found := commands.Find("")
	found[0].Command.Aliases[0] = "changed-found"
	if _, _, ok := commands.Lookup("old-name"); !ok {
		t.Fatal("changing a Find result changed the registered aliases")
	}
}

func TestCommandsRemove(t *testing.T) {
	c := registry()
	c.Used("quit")
	if !c.Remove("quit") {
		t.Fatal("removing a registered command reported false")
	}
	if _, _, ok := c.Lookup("quit"); ok {
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
	if _, _, ok := registry().Lookup("cls"); !ok {
		t.Error("an alias did not find its command")
	}
	if _, _, ok := registry().Lookup("nothing"); ok {
		t.Error("a name nobody registered found something")
	}
}

func TestAnExactCommandNameOutranksAnotherCommandsAlias(t *testing.T) {
	var commands headless.Commands[int]
	commands.Add(headless.Command{Name: "old", Aliases: []string{"current"}}, 1)
	commands.Add(headless.Command{Name: "current"}, 2)
	got, value, ok := commands.Lookup("current")
	if !ok || got.Name != "current" {
		t.Fatalf("Lookup(current) = %+v, %v; want the exact command", got, ok)
	}
	if value != 2 {
		t.Fatalf("Lookup(current) value = %d, want the exact command's value", value)
	}
}

func TestCommandsDoNotInterpretApplicationSyntax(t *testing.T) {
	// A slash is one application's command introducer, not part of searching a
	// registry. The application extracts its query before it reaches this layer.
	if got := names(registry().Find("/clear")); len(got) != 0 {
		t.Errorf("a generic registry interpreted slash syntax and found %v", got)
	}
}

func TestCommandsRememberOnlyRecently(t *testing.T) {
	// Past a handful, "recently" stops meaning anything and the list is just the
	// registry again.
	var c headless.Commands[struct{}]
	for i := range 40 {
		c.Add(headless.Command{Name: string(rune('a'+i%26)) + string(rune('0'+i/26))}, struct{}{})
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
