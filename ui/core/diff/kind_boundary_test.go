package diff_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/diff"
)

func TestKindValidationCoversTheWholeDomain(t *testing.T) {
	for value := 0; value <= math.MaxUint8; value++ {
		kind := diff.Kind(value)
		err := kind.Validate()
		if kind <= diff.Removed {
			if err != nil {
				t.Errorf("legal kind %d: %v", kind, err)
			}
		} else if !errors.Is(err, diff.ErrInvalidKind) {
			t.Errorf("unknown kind %d: %v", kind, err)
		}
	}
}

func expectKindPanic(t *testing.T, run func()) {
	t.Helper()
	defer func() {
		err, _ := recover().(error)
		if !errors.Is(err, diff.ErrInvalidKind) {
			t.Errorf("panic = %v, want %v", err, diff.ErrInvalidKind)
		}
	}()
	run()
}

func TestScriptRejectsInvalidKindsAfterAnEarlierChange(t *testing.T) {
	script := diff.Script{
		{Kind: diff.Added, Text: "changed", New: 1},
		{Kind: diff.Context, Text: "context", Old: 1, New: 2},
		{Kind: diff.Kind(math.MaxUint8), Text: "invalid"},
	}
	err := script.Validate()
	if !errors.Is(err, diff.ErrInvalidKind) || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("Validate = %v, want the invalid third line", err)
	}
	for _, tc := range []struct {
		name string
		run  func()
	}{
		{"Kind.String", func() { _ = script[2].Kind.String() }},
		{"Line.String", func() { _ = script[2].String() }},
		{"Script.Same", func() { script.Same() }},
		{"Script.String", func() { _ = script.String() }},
		{"Script.Hunks", func() { script.Hunks(0) }},
		{"Hunk.String", func() { _ = (diff.Hunk{Lines: script}).String() }},
	} {
		t.Run(tc.name, func(t *testing.T) { expectKindPanic(t, tc.run) })
	}
}

func TestEmptyAndGeneratedScriptsValidate(t *testing.T) {
	for _, script := range []diff.Script{
		nil,
		{},
		diff.Between([]string{"before", "same"}, []string{"after", "same"}),
	} {
		if err := script.Validate(); err != nil {
			t.Fatalf("valid script: %v", err)
		}
	}
}
