package base_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func same(a, b string) bool { return a == b }

func TestSpokenLabelsUseUnicodeCaseFolding(t *testing.T) {
	for _, test := range []struct {
		name, label, reply string
	}{
		{"final sigma", "Σ", "ς"},
		{"final sigma prefix", "Σειρά", "ς"},
		{"long s", "ſ", "S"},
		{"long s prefix", "ſeed", "s"},
		{"long s reply prefix", "Seed", "ſ"},
		{"Kelvin prefix", "Kelvin", "k"},
		{"Kelvin reply prefix", "kelvin", "K"},
		{"label containing comma", "Σ, accepted", "ς, ACCEPTED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := base.Options("other", test.label)
			single := base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string], Options: options})
			multiple := base.NewMultiSelect(base.MultiSelectConfig[string]{Same: base.Equal[string], Options: options})
			confirm := base.NewConfirm(base.ConfirmConfig{Yes: test.label, No: "other"})
			for _, field := range []struct {
				name     string
				value    base.SpokenField
				accepted func() bool
			}{
				{"select", single, func() bool { return chosenValue(single) == test.label }},
				{"multiselect", multiple, func() bool { return slices.Equal(multiple.Taken(), []string{test.label}) }},
				{"confirm", confirm, confirm.Answer},
			} {
				t.Run(field.name, func(t *testing.T) {
					if err := field.value.Reply(test.reply); err != nil {
						t.Fatal("folded label was refused:", err)
					}
					if !field.accepted() {
						t.Fatal("reply did not select its folded label")
					}
				})
			}
		})
	}
}

func TestFoldedSpokenAmbiguityPreservesTheAnswer(t *testing.T) {
	for _, test := range []struct {
		name, first, second, reply string
	}{
		{"sigma exact", "Σ", "ς", "Σ"},
		{"sigma prefix", "Σfirst", "ςsecond", "σ"},
		{"long s exact", "ſ", "s", "S"},
		{"long s prefix", "ſeed", "Sand", "s"},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := base.Options("kept", test.first, test.second)
			single := base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string], Options: options})
			multiple := base.NewMultiSelect(base.MultiSelectConfig[string]{Same: base.Equal[string], Options: options, Initial: []string{"kept"}})
			confirm := base.NewConfirm(base.ConfirmConfig{Yes: test.first, No: test.second})
			for _, field := range []struct {
				name      string
				value     base.SpokenField
				unchanged func() bool
			}{
				{"select", single, func() bool { return chosenValue(single) == "kept" }},
				{"multiselect", multiple, func() bool { return slices.Equal(multiple.Taken(), []string{"kept"}) }},
				{"confirm", confirm, func() bool { return !confirm.Answer() }},
			} {
				t.Run(field.name, func(t *testing.T) {
					before := field.value.Revision()
					if err := field.value.Reply(test.reply); err == nil {
						t.Fatal("ambiguous folded label was accepted")
					}
					if !field.unchanged() || field.value.Revision() != before {
						t.Fatal("ambiguous reply changed the answer")
					}
				})
			}
		})
	}
}

func TestAnExactFoldedSpokenLabelWinsOverLongerPrefixes(t *testing.T) {
	field := base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string], Options: base.Options("Σeed", "Σ", "ςmore")})
	if err := field.Reply("ς"); err != nil {
		t.Fatal("exact folded label was refused:", err)
	}
	if chosenValue(field) != "Σ" {
		t.Fatal("a prefix displaced the exact folded label")
	}
}

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
