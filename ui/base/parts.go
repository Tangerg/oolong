package base

import (
	"context"
	"strconv"

	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func (t *Text) Element() scene.Element {
	if t.element.IsZero() {
		t.line()
		t.element = scene.NewElement(scene.ElementConfig{
			Key: t.key, Measure: scene.MeasureRows(t.HeightForWidth), Paint: t.Draw,
			Handle: func(e scene.Event) scene.EventResult { return partEvent(e, t.Handle, &t.geometry, &t.editor.geometry) }, Focusable: func() bool { return true }, FocusChanged: t.focusChanged,
			Semantics: func() scene.Semantic {
				return withFeedback(scene.Semantic{Role: scene.RoleTextBox, Name: t.label, Value: t.editor.semanticValue(), Sensitive: t.Mask() != "", ContentRevision: t.Revision()}, t.feedbackError(t.PasteError()))
			}, Actions: []scene.ActionBinding{t.editor.fillAction()},
		})
	}
	t.editor.element = t.element
	t.editor.scroll.element = t.element
	t.editor.keys.attach(t.element, &t.editor.matcher)
	return t.element
}

func (c *Confirm) Element() scene.Element {
	if c.element.IsZero() {
		set := scene.NewAction(scene.ActionConfig[bool, bool]{ID: scene.ActionSetValue, Label: "Set answer", Run: func(ctx context.Context, value bool) (scene.ActionResult[bool], error) {
			snapshot := c.Snapshot()
			result, err := c.Request(ctx, ConfirmRequest{ExpectedRevision: snapshot.Revision, Value: value})
			return scene.ActionResult[bool]{State: result.State, Value: result.Value.Value}, err
		}})
		c.element = scene.NewElement(scene.ElementConfig{
			Key: c.key, Enabled: func() bool { return !c.closed }, Measure: scene.MeasureRows(c.HeightForWidth), Paint: c.Draw,
			Handle: func(e scene.Event) scene.EventResult { return partEvent(e, c.Handle, &c.geometry) }, Focusable: func() bool { return true }, FocusChanged: c.focusChanged,
			Semantics: func() scene.Semantic {
				state := scene.SemanticState(0)
				snapshot := c.Snapshot()
				if snapshot.Value {
					state |= scene.StateChecked
				}
				semantic := scene.Semantic{Role: scene.RoleCheckBox, Name: c.label, Value: strconv.FormatBool(snapshot.Value), State: state}
				return withFeedback(semantic, c.feedbackError(c.ChangeError()))
			}, Actions: []scene.ActionBinding{set.Binding()},
		})
	}
	c.keys.attach(c.element, &c.matcher)
	return c.element
}

func (s *Select[T]) Element() scene.Element {
	if s.element.IsZero() {
		selectValue := scene.NewAction(scene.ActionConfig[T, T]{ID: scene.ActionSelect, Label: "Select", Availability: func(value T) scene.Availability {
			_, ok := s.indexOf(s.list.items, value)
			return scene.Availability{Disabled: !ok, Reason: "value is not available"}
		}, Run: func(_ context.Context, value T) (scene.ActionResult[T], error) {
			before := s.selected()
			s.Choose(value)
			state := scene.Unchanged
			if before != s.selected() {
				state = scene.Applied
			}
			selected, _ := s.Chosen()
			return scene.ActionResult[T]{State: state, Value: selected.Value}, nil
		}})
		s.adopt(scene.NewElement(scene.ElementConfig{
			Key: s.key, Measure: scene.MeasureRows(s.HeightForWidth), Paint: s.Draw,
			Handle: func(e scene.Event) scene.EventResult {
				return partEvent(e, s.Handle, &s.geometry, &s.list.geometry, &s.list.pointer.geometry)
			}, Focusable: func() bool { return true }, FocusChanged: s.focusChanged,
			Semantics: func() scene.Semantic {
				choice, _ := s.Chosen()
				return withFeedback(scene.Semantic{Role: scene.RoleList, Name: s.label, Value: choice.Label, ContentRevision: s.list.contentRevision}, s.Error())
			}, Actions: []scene.ActionBinding{selectValue.Binding(), collectionReadAction(&s.list, unchangedItem[Option[T]])},
		}))
	}
	return s.element
}

func (m *MultiSelect[T]) Element() scene.Element {
	if m.element.IsZero() {
		m.adopt(scene.NewElement(scene.ElementConfig{
			Key: m.key, Measure: scene.MeasureRows(m.HeightForWidth), Paint: m.Draw,
			Handle: func(e scene.Event) scene.EventResult {
				return partEvent(e, m.Handle, &m.geometry, &m.list.geometry, &m.list.pointer.geometry)
			}, Focusable: func() bool { return true }, FocusChanged: m.focusChanged,
			Semantics: func() scene.Semantic {
				return withFeedback(scene.Semantic{Role: scene.RoleList, Name: m.label, Value: m.takenLabels(), ContentRevision: m.list.contentRevision}, m.Error())
			}, Actions: append(collectionActions(&m.list, m.choiceState), m.setTakenAction()),
		}))
	}
	return m.element
}

func (e *Editor) Element() scene.Element {
	if e.element.IsZero() {
		e.element = scene.NewElement(scene.ElementConfig{
			Measure: scene.MeasureRows(e.HeightForWidth), Paint: e.Draw,
			Handle: func(event scene.Event) scene.EventResult { return partEvent(event, e.Handle, &e.geometry) }, Focusable: func() bool { return true }, FocusChanged: e.focusChanged,
			Semantics: func() scene.Semantic {
				return withFeedback(scene.Semantic{Role: scene.RoleTextBox, Name: e.placeholder, Value: e.semanticValue(), Sensitive: e.Mask() != "", ContentRevision: e.Revision()}, e.PasteError())
			}, Actions: []scene.ActionBinding{e.fillAction()},
		})
	}
	e.scroll.element = e.element
	e.keys.attach(e.element, &e.matcher)
	return e.element
}

// A synchronous edit subscriber may replace the answer or change its masking policy.
func (e *Editor) fillAction() scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[string, string]{
		ID: scene.ActionFill, Label: "Fill", Run: func(_ context.Context, value string) (scene.ActionResult[string], error) {
			masked := e.Mask() != ""
			before := e.Revision()
			e.SetText(value)
			result := scene.ActionResult[string]{}
			if e.Revision() != before {
				result.State = scene.Applied
			}
			if !masked {
				result.Value = e.semanticValue()
			}
			return result, nil
		},
	}).Binding()
}

func (e *Editor) semanticValue() string {
	if e.Mask() != "" {
		return ""
	}
	return e.Text()
}

func (l *List[T]) Element() scene.Element {
	if l.element.IsZero() {
		l.element = scene.NewElement(scene.ElementConfig{
			Measure: scene.MeasureRows(l.HeightForWidth), Paint: l.Draw,
			Handle: func(e scene.Event) scene.EventResult { return partEvent(e, l.Handle, &l.geometry, &l.pointer.geometry) }, Focusable: func() bool { return true }, FocusChanged: l.focusChanged,
			Semantics: func() scene.Semantic {
				return scene.Semantic{Role: scene.RoleList, ContentRevision: l.contentRevision, Value: strconv.Itoa(l.Selected())}
			}, Actions: collectionActions(l, unchangedItem[T]),
		})
	}
	l.scroll.element = l.element
	l.keys.attach(l.element, &l.matcher)
	return l.element
}

func (t *Tree[T]) Element() scene.Element {
	if t.element.IsZero() {
		t.element = scene.NewElement(scene.ElementConfig{
			Measure: scene.MeasureRows(t.HeightForWidth), Paint: t.Draw,
			Handle: func(event scene.Event) scene.EventResult {
				return partEvent(event, t.Handle, &t.list.geometry, &t.list.pointer.geometry)
			},
			Focusable: func() bool { return true }, FocusChanged: t.focusChanged,
			Semantics: func() scene.Semantic {
				return scene.Semantic{Role: scene.RoleTree, ContentRevision: t.list.contentRevision, Value: strconv.Itoa(t.Selected())}
			}, Actions: append(collectionActions(&t.list, treeRow[T].shown), t.branchAction(true), t.branchAction(false)),
		})
	}
	t.list.element, t.list.scroll.element = t.element, t.element
	t.keys.attach(t.element, &t.matcher)
	return t.element
}

func (f *Filter[T]) Element() scene.Element {
	if f.element.IsZero() {
		f.element = scene.NewElement(scene.ElementConfig{
			Measure: scene.MeasureRows(f.HeightForWidth), Paint: f.Draw,
			Handle: func(e scene.Event) scene.EventResult {
				return partEvent(e, f.Handle, &f.list.geometry, &f.list.pointer.geometry)
			}, Focusable: func() bool { return true }, FocusChanged: f.focusChanged,
			Semantics: func() scene.Semantic {
				return scene.Semantic{Role: scene.RoleList, ContentRevision: f.list.contentRevision, Name: f.Pattern(), Value: strconv.Itoa(f.Selected())}
			}, Actions: append(collectionActions(&f.list, func(row hit[T]) T { return row.item }), f.patternAction()),
		})
	}
	f.list.element = f.element
	f.list.scroll.element = f.element
	f.list.keys.attach(f.element, &f.list.matcher)
	return f.element
}

func (t *Table[T]) Element() scene.Element {
	if t.element.IsZero() {
		t.element = scene.NewElement(scene.ElementConfig{
			Measure: scene.MeasureRows(t.HeightForWidth), Paint: t.Draw,
			Handle: func(event scene.Event) scene.EventResult {
				return partEvent(event, t.Handle, &t.rows.geometry, &t.rows.pointer.geometry)
			},
			Focusable: func() bool { return true }, FocusChanged: t.focusChanged,
			Semantics: func() scene.Semantic {
				return scene.Semantic{Role: scene.RoleTable, ContentRevision: t.rows.contentRevision, Value: strconv.Itoa(t.Selected())}
			}, Actions: append(collectionActions(&t.rows, unchangedItem[T]), t.sortAction(), t.clearSortAction()),
		})
	}
	t.rows.element, t.rows.scroll.element = t.element, t.element
	t.rows.keys.attach(t.element, &t.rows.matcher)
	return t.element
}

func (s *Settings[T]) Element() scene.Element {
	if s.list.element.IsZero() {
		s.list.element = scene.NewElement(scene.ElementConfig{
			Measure: scene.MeasureRows(s.HeightForWidth), Paint: s.Draw,
			Handle: func(e scene.Event) scene.EventResult {
				return partEvent(e, s.Handle, &s.list.geometry, &s.list.pointer.geometry)
			}, Focusable: func() bool { return true }, FocusChanged: s.focusChanged,
			Semantics: func() scene.Semantic {
				return withFeedback(scene.Semantic{Role: scene.RoleList, ContentRevision: s.list.contentRevision, Value: strconv.Itoa(s.list.Selected())}, s.ChangeError())
			}, Actions: append(collectionActions(&s.list, unchangedItem[T]), s.valueActions()...),
		})
	}
	s.list.scroll.element = s.list.element
	s.list.keys.attach(s.list.element, &s.list.matcher)
	s.editBindings.attach(s.list.element, &s.matcher)
	return s.list.element
}

func (s *Slider) Element() scene.Element {
	if s.element.IsZero() {
		set := scene.NewAction(scene.ActionConfig[int, int]{ID: scene.ActionSetValue, Label: "Set value", Run: func(_ context.Context, value int) (scene.ActionResult[int], error) {
			state := scene.Unchanged
			if s.Set(value) {
				state = scene.Applied
			}
			return scene.ActionResult[int]{State: state, Value: s.Value()}, nil
		}})
		s.element = scene.NewElement(scene.ElementConfig{
			Focusable: func() bool { return true }, FocusChanged: s.focusChanged,
			Measure: func(axis layout.Axis, _ int) int {
				if axis == layout.Down {
					return 1
				}
				return 0
			},
			Paint: func(frame scene.Frame) {
				s.Stage(frame, frame.Bounds())
				frame.Text(0, 0, strconv.Itoa(s.Value()), Look{}.Text)
			},
			Handle: func(e scene.Event) scene.EventResult { return partEvent(e, s.Handle, &s.pointer.geometry) },
			Semantics: func() scene.Semantic {
				return scene.Semantic{Role: scene.RoleSlider, Name: s.Label(), Value: strconv.Itoa(s.Value())}
			}, Actions: []scene.ActionBinding{set.Binding()},
		})
	}
	s.keys.attach(s.element, &s.matcher)
	return s.element
}

// Element is the completion's one candidate-list identity. It receives pointer
// input while keyboard focus remains with the composing editor.
func (c *Completion) Element() scene.Element {
	if c.element.IsZero() {
		accept := scene.NewAction(scene.ActionConfig[CollectionIndex, Candidate]{ID: scene.ActionActivate, Label: "Accept completion", Availability: func(address CollectionIndex) scene.Availability {
			if !c.Open() || c.accept == nil {
				return scene.Availability{Disabled: true, Reason: "completion is not accepting"}
			}
			return c.list.indexAvailability(address)
		}, Run: func(_ context.Context, address CollectionIndex) (scene.ActionResult[Candidate], error) {
			result, _ := c.acceptIndex(address.Index)
			return result, nil
		}})
		dismiss := scene.NewAction(scene.ActionConfig[struct{}, struct{}]{ID: scene.ActionDismiss, Label: "Dismiss completion", Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
			state := scene.Unchanged
			if c.Dismiss() {
				state = scene.Applied
			}
			return scene.ActionResult[struct{}]{State: state}, nil
		}})
		c.element = scene.NewElement(scene.ElementConfig{Enabled: c.Open, Measure: scene.MeasureRows(c.HeightForWidth), Paint: c.Draw, Handle: func(event scene.Event) scene.EventResult {
			return partEvent(event, c.Handle, &c.list.geometry, &c.list.pointer.geometry)
		}, Semantics: func() scene.Semantic {
			return scene.Semantic{Role: scene.RoleList, ContentRevision: c.list.contentRevision, Value: strconv.Itoa(c.list.Selected())}
		}, Actions: append([]scene.ActionBinding{accept.Binding(), dismiss.Binding()}, collectionActions(&c.list, Candidate.snapshot)...)})
	}
	c.list.element, c.list.scroll.element = c.element, c.element
	c.keys.attach(c.element, &c.matcher)
	return c.element
}
