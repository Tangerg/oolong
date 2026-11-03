package base_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestCompletionActionsExposeBoundedDetachedCandidates(t *testing.T) {
	candidates := make([]base.Candidate, base.DefaultCollectionPageSize+1)
	for i := range candidates {
		candidates[i] = base.Candidate{Text: strconv.Itoa(i), Matched: []int{0}}
	}
	var completion base.Completion
	completion.Offer(base.Token{}, candidates)
	root := semanticOwner(t, completion.Element())
	revision := semanticRevision(t, root, completion.Element())
	read, err := root.Invoke(t.Context(), completion.Element().Target(), base.ActionReadItems, base.CollectionQuery{ContentRevision: revision})
	if err != nil || read.State != scene.Unchanged {
		t.Fatalf("read = %+v, %v", read, err)
	}
	page := read.Value.(base.CollectionPage[base.Candidate])
	if page.ContentRevision != revision || page.Total != len(candidates) || page.Selected != 0 || len(page.Items) != base.DefaultCollectionPageSize {
		t.Fatalf("page = %+v", page)
	}
	page.Items[0].Text = "changed snapshot"
	page.Items[0].Matched[0] = 9
	current, _ := completion.Current()
	if current.Text != candidates[0].Text || current.Matched[0] != 0 {
		t.Fatalf("read borrowed the owned candidate: %+v", current)
	}
	query := base.CollectionQuery{ContentRevision: revision, Start: base.DefaultCollectionPageSize, Limit: 1}
	read, err = root.Invoke(t.Context(), completion.Element().Target(), base.ActionReadItems, query)
	if err != nil || len(read.Value.(base.CollectionPage[base.Candidate]).Items) != 1 {
		t.Fatalf("last page = %+v, %v", read, err)
	}
	query.Limit = base.MaxCollectionPageSize + 1
	if _, err = root.Invoke(t.Context(), completion.Element().Target(), base.ActionReadItems, query); !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("unbounded page = %v", err)
	}
	address := base.CollectionIndex{ContentRevision: revision, Index: 1}
	selected, err := root.Invoke(t.Context(), completion.Element().Target(), scene.ActionSelect, address)
	if err != nil || selected.State != scene.Applied {
		t.Fatalf("selection = %+v, %v", selected, err)
	}
	selection := selected.Value.(base.CollectionSelection[base.Candidate])
	if selection.Address != address || !selection.Present || selection.Item.Text != candidates[1].Text {
		t.Fatalf("selected candidate = %+v", selection)
	}
	selection.Item.Matched[0] = 9
	current, _ = completion.Current()
	if current.Text != candidates[1].Text || current.Matched[0] != 0 {
		t.Fatalf("selection borrowed the owned candidate: %+v", current)
	}
}

func TestCompletionActionsRejectSupersededOffers(t *testing.T) {
	accepted := false
	completion := &base.Completion{Accept: func(base.Candidate, base.Token) { accepted = true }}
	completion.Offer(base.Token{Query: "a"}, []base.Candidate{{Text: "alpha"}, {Text: "apple"}})
	root := semanticOwner(t, completion.Element())
	revision := semanticRevision(t, root, completion.Element())
	address := base.CollectionIndex{ContentRevision: revision, Index: 1}
	completion.Offer(base.Token{Query: "b"}, []base.Candidate{{Text: "beta"}, {Text: "backup"}})
	for _, action := range []scene.ActionID{scene.ActionSelect, scene.ActionActivate} {
		if _, err := root.Invoke(t.Context(), completion.Element().Target(), action, address); !errors.Is(err, scene.ErrUnavailable) {
			t.Fatalf("stale %s = %v", action, err)
		}
	}
	if _, err := root.Invoke(t.Context(), completion.Element().Target(), base.ActionReadItems, base.CollectionQuery{ContentRevision: revision}); !errors.Is(err, scene.ErrUnavailable) {
		t.Fatalf("stale page = %v", err)
	}
	for _, call := range []struct {
		action scene.ActionID
		params any
	}{{scene.ActionSelect, 1}, {scene.ActionActivate, nil}} {
		if _, err := root.Invoke(t.Context(), completion.Element().Target(), call.action, call.params); !errors.Is(err, scene.ErrArgument) {
			t.Fatalf("unqualified %s = %v", call.action, err)
		}
	}
	current, _ := completion.Current()
	if accepted || current.Text != "beta" {
		t.Fatalf("stale intent changed replacement offer: accepted=%t current=%+v", accepted, current)
	}
}

func TestCompletionActivationAcceptsTheAddressedCandidate(t *testing.T) {
	var accepted base.Candidate
	var offered base.Token
	completion := &base.Completion{Accept: func(candidate base.Candidate, token base.Token) {
		candidate.Matched[0] = 9
		accepted, offered = candidate, token
	}}
	token := base.Token{Query: "a"}
	completion.Offer(token, []base.Candidate{{Text: "alpha", Matched: []int{0}}, {Text: "apple", Matched: []int{0}}})
	root := semanticOwner(t, completion.Element())
	address := base.CollectionIndex{ContentRevision: semanticRevision(t, root, completion.Element())}
	completion.Do(base.SelectNext)
	result, err := root.Invoke(t.Context(), completion.Element().Target(), scene.ActionActivate, address)
	if err != nil || result.State != scene.Applied || accepted.Text != "alpha" || offered != token || completion.Open() {
		t.Fatalf("activation = %+v, %v, accepted=%+v token=%+v open=%t", result, err, accepted, offered, completion.Open())
	}
	if candidate := result.Value.(base.Candidate); candidate.Text != "alpha" || candidate.Matched[0] != 0 {
		t.Fatalf("acceptance callback changed the result: %+v", candidate)
	}
}
