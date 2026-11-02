package main

import (
	"context"
	"strings"

	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

type outcomeKind uint8

const (
	outcomeNotice outcomeKind = iota
	outcomeCompleted
	outcomeCancelled
	outcomeFailed
)

type sessionOutcome struct {
	kind   outcomeKind
	detail string
}

func finishedOutcome(err error) sessionOutcome {
	switch {
	case err == context.Canceled: //nolint:errorlint // Wrapped cancellation still carries failure diagnostics.
		return sessionOutcome{kind: outcomeCancelled}
	case err != nil:
		return sessionOutcome{kind: outcomeFailed, detail: err.Error()}
	default:
		return sessionOutcome{kind: outcomeCompleted}
	}
}

func (o sessionOutcome) label(theme kit.Theme, glyphs kit.Glyphs) kit.Label {
	label := kit.Label{Style: theme.Muted, Ellipsis: glyphs.Ellipsis}
	switch o.kind {
	case outcomeNotice:
		label.Text = o.detail
		if label.Text == "" {
			label.Text = "ready"
		}
	case outcomeCompleted:
		label.Text, label.Style = "complete", theme.Success
	case outcomeCancelled:
		label.Text, label.Style = "cancelled", theme.Warning
	case outcomeFailed:
		label.Text, label.Style = "failed: "+strings.ReplaceAll(o.detail, "\n", "; "), theme.Danger
	}
	return label
}

func (o sessionOutcome) progress() term.Progress {
	const completedPercent = 100
	switch o.kind {
	case outcomeNotice:
		return term.Progress{}
	case outcomeCompleted:
		return term.Progress{State: term.ProgressNormal, Percent: completedPercent}
	case outcomeCancelled:
		return term.Progress{State: term.ProgressWarning, Percent: completedPercent}
	case outcomeFailed:
		return term.Progress{State: term.ProgressError, Percent: completedPercent}
	default:
		return term.Progress{}
	}
}
