package identity_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/internal/identity"
)

type mapValue map[string]int

func TestSameNeverRequiresAnOpenValueToBeComparable(t *testing.T) {
	shared := mapValue{"one": 1}
	other := mapValue{"one": 1}
	tests := []struct {
		name  string
		left  any
		right any
		want  bool
	}{
		{name: "nil", want: true},
		{name: "comparable value", left: "same", right: "same", want: true},
		{name: "different type", left: "one", right: []byte("one")},
		{name: "same map", left: shared, right: shared, want: true},
		{name: "different maps", left: shared, right: other},
		{name: "slice is conservative", left: []int{1}, right: []int{1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := identity.Same(test.left, test.right); got != test.want {
				t.Fatalf("Same(%T, %T) = %t, want %t", test.left, test.right, got, test.want)
			}
		})
	}
}
