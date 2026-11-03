package base

import (
	"context"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func (m *MultiSelect[T]) choiceState(option Option[T]) ChoiceState[T] {
	at := optionIndex(m.list.items, option.Value, m.Same)
	_, taken := slices.BinarySearch(m.taken, at)
	return ChoiceState[T]{Option: option, Taken: taken}
}

func (m *MultiSelect[T]) takenLabels() string {
	labels := make([]string, len(m.taken))
	for i, at := range m.taken {
		labels[i] = m.list.items[at].Label
	}
	return strings.Join(labels, ", ")
}

func (m *MultiSelect[T]) setTakenAction() scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[[]T, []T]{
		ID: scene.ActionSetValue, Label: "Set chosen values",
		Availability: func(values []T) scene.Availability {
			indices, complete := m.indices(m.list.items, values)
			if !complete {
				return scene.Availability{Disabled: true, Reason: "value is not available"}
			}
			if m.limit > 0 && len(indices) > m.limit {
				return scene.Availability{Disabled: true, Reason: "selection exceeds the choice limit"}
			}
			return scene.Availability{}
		},
		Run: func(_ context.Context, values []T) (scene.ActionResult[[]T], error) {
			before := m.Revision()
			m.SetTaken(values)
			state := scene.Unchanged
			if m.Revision() != before {
				state = scene.Applied
			}
			return scene.ActionResult[[]T]{State: state, Value: m.Taken()}, nil
		},
	}).Binding()
}

func (f *Filter[T]) patternAction() scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[string, string]{
		ID: scene.ActionFill, Label: "Set filter pattern",
		Run: func(_ context.Context, pattern string) (scene.ActionResult[string], error) {
			before := f.list.contentRevision
			f.SetPattern(pattern)
			state := scene.Unchanged
			if f.list.contentRevision != before {
				state = scene.Applied
			}
			return scene.ActionResult[string]{State: state, Value: f.Pattern()}, nil
		},
	}).Binding()
}

func (t *Tree[T]) branchAction(open bool) scene.ActionBinding {
	id, label := scene.ActionCollapse, "Collapse branch"
	if open {
		id, label = scene.ActionExpand, "Expand branch"
	}
	return scene.NewAction(scene.ActionConfig[CollectionIndex, bool]{
		ID: id, Label: label,
		Availability: func(address CollectionIndex) scene.Availability {
			if availability := t.list.indexAvailability(address); availability.Disabled {
				return availability
			}
			if !t.list.items[address.Index].Branch {
				return scene.Availability{Disabled: true, Reason: "item is not a branch"}
			}
			return scene.Availability{}
		},
		Run: func(_ context.Context, address CollectionIndex) (scene.ActionResult[bool], error) {
			identity := t.list.items[address.Index].id
			state := scene.Unchanged
			if t.set(address.Index, open) {
				state = scene.Applied
			}
			return scene.ActionResult[bool]{State: state, Value: t.open[identity]}, nil
		},
	}).Binding()
}
