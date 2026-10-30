package base_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestFilterProjectionFailureLeavesThePublishedSourceIntact(t *testing.T) {
	for _, operation := range []string{"pattern", "source"} {
		t.Run(operation, func(t *testing.T) {
			var filter base.Filter[string]
			fail := false
			project := func(item string) string {
				if fail {
					panic("projection failed")
				}
				return item
			}
			filter.SetItems([]string{"alpha", "beta"}, project)
			filter.SetPattern("a")
			filter.Select(1)
			fail = true
			func() {
				defer func() {
					if got := recover(); got != "projection failed" {
						t.Fatalf("projection panic = %v", got)
					}
				}()
				if operation == "pattern" {
					filter.SetPattern("b")
				} else {
					filter.SetItems([]string{"gamma"}, project)
				}
			}()
			current, ok := filter.Current()
			if !slices.Equal(filter.Items(), []string{"alpha", "beta"}) || filter.Pattern() != "a" ||
				filter.Matched() != 2 || filter.Selected() != 1 || !ok || current != "beta" {
				t.Fatalf("failed projection changed publication: source=%v pattern=%q matches=%d selected=%d current=%q",
					filter.Items(), filter.Pattern(), filter.Matched(), filter.Selected(), current)
			}
			fail = false
			filter.SetPattern("b")
			if current, ok := filter.Current(); !ok || current != "beta" || filter.Matched() != 1 {
				t.Fatal("filter did not retain its previous projection after failure")
			}
		})
	}
}

func TestFilterProjectionCannotReplaceItsOwnInputs(t *testing.T) {
	for _, operation := range []string{"pattern", "source"} {
		t.Run(operation, func(t *testing.T) {
			var filter base.Filter[string]
			entered := false
			filter.SetItems([]string{"alpha", "beta"}, func(item string) string {
				if entered {
					return item
				}
				entered = true
				if operation == "pattern" {
					filter.SetPattern("obsolete")
				} else {
					filter.SetItems([]string{"obsolete"}, nil)
				}
				return item
			})
			filter.Select(1)
			func() {
				defer func() {
					if recover() == nil {
						t.Error("projection independently changed its matching inputs")
					}
				}()
				filter.SetPattern("a")
			}()
			current, ok := filter.Current()
			if !slices.Equal(filter.Items(), []string{"alpha", "beta"}) || filter.Pattern() != "" ||
				filter.Matched() != 2 || filter.Selected() != 1 || !ok || current != "beta" {
				t.Fatalf("interrupted projection changed publication: source=%v pattern=%q matches=%d selected=%d current=%q",
					filter.Items(), filter.Pattern(), filter.Matched(), filter.Selected(), current)
			}
			filter.SetPattern("b")
			if current, ok := filter.Current(); !ok || current != "beta" || filter.Matched() != 1 {
				t.Fatal("filter could not match after interruption")
			}
		})
	}
}

func TestFilterProjectionReadsTheLastCompleteSource(t *testing.T) {
	for _, operation := range []string{"pattern", "source"} {
		t.Run(operation, func(t *testing.T) {
			var filter base.Filter[string]
			checking := false
			project := func(item string) string {
				if checking && (!slices.Equal(filter.Items(), []string{"alpha", "beta"}) || filter.Pattern() != "a" || filter.Matched() != 2) {
					t.Fatal("projection observed a partially published source")
				}
				return item
			}
			filter.SetItems([]string{"alpha", "beta"}, project)
			filter.SetPattern("a")
			checking = true
			if operation == "pattern" {
				filter.SetPattern("b")
			} else {
				filter.SetItems([]string{"gamma"}, project)
			}
			want := "beta"
			if operation == "source" {
				want = "gamma"
			}
			if current, ok := filter.Current(); !ok || current != want || filter.Matched() != 1 {
				t.Fatalf("current=%q exists=%t matches=%d, want %s", current, ok, filter.Matched(), want)
			}
		})
	}
}
