package main

import (
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/fuzzy"
)

type reference struct{ name, detail string }

func (p *prompt) refreshCompletion() {
	token, ok := p.completion.CurrentToken()
	if !ok {
		p.completion.Dismiss()
		return
	}
	matches := fuzzy.FilterFunc(token.Query, p.references, func(r reference) string { return r.name })
	candidates := make([]base.Candidate, 0, len(matches))
	for _, match := range matches {
		item := p.references[match.Index]
		candidates = append(candidates, base.Candidate{
			Text: item.name, Label: item.name, Detail: item.detail, Matched: match.Match.At,
		})
	}
	p.completion.Offer(token, candidates)
}
