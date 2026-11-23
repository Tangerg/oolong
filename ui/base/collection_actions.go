package base

import (
	"context"

	"github.com/Tangerg/oolong/ui/core/scene"
)

const ActionReadItems scene.ActionID = "read-items"

// Refusal reasons shared by more than one collection operation.
const (
	staleCollectionRevision = "collection revision changed"
	unavailableValue        = "value is not available"
)

const (
	DefaultCollectionPageSize = 64
	MaxCollectionPageSize     = 256
)

// CollectionIndex carries a revision so a replaced, filtered or sorted row cannot
// silently inherit an action intended for the previous occupant of that position.
type CollectionIndex struct {
	ContentRevision uint64
	Index           int
}

// CollectionQuery pins the observed revision so pagination cannot mix collections.
// Zero Limit uses DefaultCollectionPageSize.
type CollectionQuery struct {
	ContentRevision uint64
	Start           int
	Limit           int
}

// CollectionPage detaches its item slice so observation cannot mutate the owner.
// References inside T retain the application's ownership, as with List.Items.
type CollectionPage[T any] struct {
	ContentRevision uint64
	Start           int
	Total           int
	Selected        int
	Items           []T
}

type CollectionSelection[T any] struct {
	Address CollectionIndex
	Item    T
	Present bool
}

type ChoiceState[T any] struct {
	Option Option[T]
	Taken  bool
}

func unchangedItem[T any](item T) T { return item }

func collectionActions[T, R any](list *rows[T], project func(T) R) []scene.ActionBinding {
	return []scene.ActionBinding{collectionReadAction(list, project), collectionSelectAction(list, project)}
}

func collectionReadAction[T, R any](list *rows[T], project func(T) R) scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[CollectionQuery, CollectionPage[R]]{
		ID: ActionReadItems, Label: "Read items",
		Availability: func(query CollectionQuery) scene.Availability {
			if query.ContentRevision != list.contentRevision {
				return scene.Unavailable(staleCollectionRevision)
			}
			if query.Start < 0 || query.Start > list.Len() || query.Limit < 0 || query.Limit > MaxCollectionPageSize {
				return scene.Unavailable("item page is outside the collection budget")
			}
			return scene.Availability{}
		},
		Run: func(_ context.Context, query CollectionQuery) (scene.ActionResult[CollectionPage[R]], error) {
			page := CollectionPage[R]{ContentRevision: list.contentRevision, Start: query.Start, Total: list.Len(), Selected: list.Selected()}
			limit := query.Limit
			if limit == 0 {
				limit = DefaultCollectionPageSize
			}
			count := min(limit, list.Len()-query.Start)
			page.Items = make([]R, count)
			list.element.ReadOnly(func() {
				for i := range page.Items {
					page.Items[i] = project(list.items[query.Start+i])
				}
			})
			return scene.ActionResult[CollectionPage[R]]{Value: page}, nil
		},
	}).Binding()
}

func collectionSelectAction[T, R any](list *rows[T], project func(T) R) scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[CollectionIndex, CollectionSelection[R]]{
		ID: scene.ActionSelect, Label: "Select item", Availability: list.indexAvailability,
		Run: func(_ context.Context, address CollectionIndex) (scene.ActionResult[CollectionSelection[R]], error) {
			before := list.version
			list.Select(address.Index)
			state := scene.AppliedIf(before != list.version)
			selected := CollectionSelection[R]{Address: CollectionIndex{ContentRevision: list.contentRevision, Index: list.Selected()}}
			if item, ok := list.Current(); ok {
				selected.Item = readProjection(list.element, func() R { return project(item) })
				selected.Present = true
			}
			return scene.ActionResult[CollectionSelection[R]]{State: state, Value: selected}, nil
		},
	}).Binding()
}

// collectionActivateAction lets an automation client open an item the way a click
// does: the cursor moves to it first, then its activation runs.
func collectionActivateAction[T any](list *rows[T]) scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[CollectionIndex, struct{}]{
		ID: scene.ActionActivate, Label: "Activate item",
		Availability: func(address CollectionIndex) scene.Availability {
			if list.activate == nil {
				return scene.Unavailable("items have no activation")
			}
			return list.indexAvailability(address)
		},
		Run: func(_ context.Context, address CollectionIndex) (scene.ActionResult[struct{}], error) {
			list.Select(address.Index)
			return scene.ActionResult[struct{}]{State: scene.AppliedIf(list.activateAt(address.Index))}, nil
		},
	}).Binding()
}

func (l *rows[T]) indexAvailability(address CollectionIndex) scene.Availability {
	if address.ContentRevision != l.contentRevision {
		return scene.Unavailable(staleCollectionRevision)
	}
	if address.Index < 0 || address.Index >= l.Len() {
		return scene.Unavailable("item is not available")
	}
	return scene.Availability{}
}
