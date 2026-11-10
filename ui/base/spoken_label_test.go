package base_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func same(a, b string) bool { return a == b }

// A reply was read as a number before a label, so a numeric label either could
// not be chosen or chose a different option without a word of warning.
func TestASpokenLabelIsWhatWasMeant(t *testing.T) {
	sizes := base.NewSelect(base.SelectConfig[string]{Same: same, Options: base.Options("10", "20", "30")})
	if err := sizes.Reply("10"); err != nil {
		t.Fatalf("exact numeric label refused: %v", err)
	}
	if chosen, _ := sizes.Chosen(); chosen.Value != "10" {
		t.Fatalf("chose %q", chosen.Value)
	}
	if err := sizes.Reply("2"); err != nil {
		t.Fatalf("a number that names no label refused: %v", err)
	}
	if chosen, _ := sizes.Chosen(); chosen.Value != "20" {
		t.Fatalf("number chose %q", chosen.Value)
	}
	shuffled := base.NewSelect(base.SelectConfig[string]{Same: same, Options: base.Options("3", "1", "2")})
	if err := shuffled.Reply("1"); err == nil {
		t.Fatal("a numeral that is one choice's number and another's label was settled silently")
	}
}

func TestAMultiChoiceLabelMayContainAComma(t *testing.T) {
	cities := base.NewMultiSelect(base.MultiSelectConfig[string]{Same: same, Options: base.Options("Paris, France", "Paris, Texas")})
	if err := cities.Reply("Paris, Texas"); err != nil {
		t.Fatalf("label with a comma refused: %v", err)
	}
	if got := cities.Taken(); !slices.Equal(got, []string{"Paris, Texas"}) {
		t.Fatalf("taken = %v", got)
	}
}

func TestAMultiChoiceLimitIsConfiguredWithItsInitialChoices(t *testing.T) {
	tools := base.NewMultiSelect(base.MultiSelectConfig[string]{Same: same, Options: base.Options("read", "write", "run"), Limit: 2, Initial: []string{"read"}})
	if tools.Limit() != 2 || !slices.Equal(tools.Taken(), []string{"read"}) {
		t.Fatalf("limit=%d taken=%v", tools.Limit(), tools.Taken())
	}
	if err := tools.Reply("read, write, run"); err == nil {
		t.Fatal("a reply beyond the limit was accepted")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("initial choices beyond the limit were truncated silently")
		}
	}()
	base.NewMultiSelect(base.MultiSelectConfig[string]{Same: same, Options: base.Options("read", "write"), Limit: 1, Initial: []string{"read", "write"}})
}
