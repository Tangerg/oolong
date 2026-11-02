package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestPlanUpdateFollowsStepIdentity(t *testing.T) {
	plan := newWorkflow(kit.Dark(), kit.Unicode())
	slices.Reverse(plan.steps)
	if err := plan.Apply(stepUpdate{Step: stepVerify, State: stepRunning}); err != nil {
		t.Fatal(err)
	}
	surface := grid.NewSurface(80, plan.HeightForWidth(80))
	plan.Draw(surface.View())
	matched := 0
	for _, row := range surface.Rows() {
		if strings.Contains(row, "verify invariants") {
			matched++
			if !strings.Contains(row, "running") {
				t.Fatalf("step status did not follow its identity: %q", row)
			}
		} else if strings.Contains(row, "running") {
			t.Fatalf("another step received the update: %q", row)
		}
	}
	if matched != 1 {
		t.Fatalf("rendered %d verification steps", matched)
	}
}
