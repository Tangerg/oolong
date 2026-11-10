package main

import (
	"context"
	"io"

	"github.com/Tangerg/oolong/ui/core/diff"
)

type planningMode string

const (
	planningFast     planningMode = "fast"
	planningCareful  planningMode = "careful"
	planningArgument              = "<" + string(planningFast) + "|" + string(planningCareful) + ">"
)

func parsePlanningMode(value string) (planningMode, bool) {
	switch mode := planningMode(value); mode {
	case planningFast, planningCareful:
		return mode, true
	default:
		return "", false
	}
}

type runRequest struct {
	prompt       string
	planningMode planningMode
}

type changeProposal struct {
	Path          string
	Summary       string
	Before, After []string
}

func (c changeProposal) title() string { return c.Path + " — " + c.Summary }

func (c changeProposal) hunks() []diff.Hunk {
	return diff.Between(c.Before, c.After).Hunks(diffContextLines)
}

type toolResult struct {
	Name, Summary string
	Change        changeProposal
}

// agentBackend is the application's model boundary. It knows prompts, plans and tool
// review, but nothing about terminal frames, components or the owner goroutine.
// Bare context.Canceled reports cancellation; every other error retains its diagnostics.
type agentBackend interface {
	Run(ctx context.Context, request runRequest, output agentOutput) error
}

// agentOutput is the complete conversation one backend run may have with its owner.
// Ordered text uses io.Writer; the few discrete domain transitions have named methods
// because they are not bytes and must never be silently coalesced with them.
type agentOutput interface {
	io.Writer
	Step(ctx context.Context, update stepUpdate) error
	Review(ctx context.Context, proposal changeProposal) (bool, error)
	Tool(ctx context.Context, result toolResult) error
}
