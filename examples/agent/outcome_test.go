package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestRunOutcomeOwnsStatusAndNativeProgress(t *testing.T) {
	theme := kit.Dark()
	for _, test := range []struct {
		name     string
		cause    error
		text     string
		style    grid.Style
		progress term.ProgressState
	}{
		{name: "complete", text: "complete", style: theme.Success, progress: term.ProgressNormal},
		{name: "cancelled", cause: context.Canceled, text: "cancelled", style: theme.Warning, progress: term.ProgressWarning},
		{name: "failed", cause: errors.New("cancelled by upstream failure"), text: "failed: cancelled by upstream failure", style: theme.Danger, progress: term.ProgressError},
		{name: "wrapped cancellation", cause: fmt.Errorf("backend: %w", context.Canceled), text: "failed: backend: context canceled", style: theme.Danger, progress: term.ProgressError},
		{name: "joined cancellation", cause: errors.Join(context.Canceled, errors.New("backend failure")), text: "failed: context canceled; backend failure", style: theme.Danger, progress: term.ProgressError},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := &agent{theme: theme, glyphs: kit.Unicode(), outcome: finishedOutcome(test.cause)}
			surface := grid.NewSurface(80, 1)
			a.drawStatus(surface.View())
			if got := strings.TrimSpace(surface.Rows()[0]); got != test.text {
				t.Fatalf("status text = %q, want %q", got, test.text)
			}
			cell, _ := surface.CellAt(0, 0)
			if cell.Style != test.style {
				t.Fatalf("status style = %+v, want %+v", cell.Style, test.style)
			}
			if progress := a.outcome.progress(); progress.State != test.progress || progress.Percent != 100 {
				t.Fatalf("native progress = %+v", progress)
			}
		})
	}
}

func TestIdleCommandFeedbackHasItsOwnOutcome(t *testing.T) {
	a := &agent{theme: kit.Dark(), outcome: finishedOutcome(errors.New("old failure"))}
	a.inform("failed is a word in this notice")
	label := a.outcome.label(a.theme, a.glyphs)
	if label.Text != "failed is a word in this notice" || label.Style != a.theme.Muted {
		t.Fatalf("idle feedback = %+v", label)
	}
}
