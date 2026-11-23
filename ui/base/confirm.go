package base

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"weak"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/internal/identity"
	"github.com/Tangerg/oolong/ui/internal/setting"
)

// Confirm is a field holding a yes or a no.
//
// The zero value answers no and is ready. A Confirm must not be copied after first
// use: its answer, matcher and committed pointer split are one mutable field.
// All input shares one revision-checked request path to the answer's owner.
type Confirm struct {
	noCopy noCopy

	field
	label string
	// Yes and No are the two answers as they are shown. Empty uses "yes" and "no".
	yes, no string
	// Keys say which keystrokes answer. Nil reads through [DefaultConfirmKeys].
	keys keyBinding

	// control is the answer's only owner: a domain owner, or a local answer
	// installed on first use when none was configured.
	control       ConfirmControl
	check         func(bool) error
	unsubscribe   func()
	closed        bool
	feedback      requestFeedback
	requestDepth  int
	pendingChange bool
	matcher       keymap.Matcher
	// split is the committed column where the second answer begins.
	split scene.Snapshot[int]
}

// ConfirmConfig chooses an owned Initial or a live domain Control. Keys and pointer input use the same version-checked request path.
type ConfirmConfig struct {
	Key     string
	Control ConfirmControl
	Initial bool
	Label   string
	Yes, No string
	Check   func(bool) error
	Keys    *keymap.Map
}

// NewConfirm constructs one owned or domain-controlled answer. Initial must be
// false when Control is present. It panics for a typed-nil Control, competing
// initial ownership or a subscription that returns no cancellation function.
func NewConfirm(config ConfirmConfig) *Confirm {
	if config.Control != nil && identity.IsNil(config.Control) {
		panic("base: nil confirm control")
	}
	if config.Control != nil && config.Initial {
		panic("base: Confirm Initial and Control are mutually exclusive")
	}
	control := config.Control
	if control == nil {
		control = &localConfirm{answer: ConfirmSnapshot{Value: config.Initial}}
	}
	c := &Confirm{label: strings.Clone(config.Label), yes: strings.Clone(config.Yes), no: strings.Clone(config.No), check: config.Check, keys: keyBinding{current: config.Keys}, control: control}
	c.key = strings.Clone(config.Key)
	if config.Control != nil {
		ref := weak.Make(c)
		c.unsubscribe = c.control.Subscribe(func() {
			if owner := ref.Value(); owner != nil && !owner.closed {
				owner.invalidate()
			}
		})
		if c.unsubscribe == nil {
			panic("base: ConfirmControl.Subscribe returned nil")
		}
	}
	return c
}

func (c *Confirm) Answer() bool {
	return c.Snapshot().Value
}

// Say requests an answer against the current revision. ChangeError reports a
// rejected UI request; use Request when the caller needs the typed result directly.
func (c *Confirm) Say(yes bool) {
	_, _ = c.requestAnswer(c.Snapshot(), yes)
}

func (c *Confirm) requestAnswer(before ConfirmSnapshot, yes bool) (scene.ActionResult[ConfirmSnapshot], error) {
	return c.request(context.Background(), ConfirmRequest{ExpectedRevision: before.Revision, Value: yes}, true)
}

func (c *Confirm) HeightForWidth(int) int { return layout.Sum(1, c.rows(c.label, c.ChangeError())) }

func (c *Confirm) Draw(v scene.Frame) {
	c.DrawWith(v, Look{})
}

// DrawWith paints the field using this frame's look without changing configuration.
// It panics unless the field is mounted in the frame's scene outside read-only projection.
func (c *Confirm) DrawWith(v scene.Frame, look Look) {
	requireProjection(c.element, v)
	answer := c.Answer()
	row := c.frame(v, c.label, look, c.ChangeError())
	w, h := row.Size()
	if w <= 0 || h <= 0 {
		c.split.Stage(v, 0)
		return
	}
	x, split := 0, 0
	for _, yes := range []bool{true, false} {
		if !yes {
			split = x
		}
		style := look.Text
		if yes == answer {
			style = look.Selection.Merge(look.Accent)
		}
		if mark, width := look.mark(yes == answer); width > 0 {
			text.Of(mark, style).Draw(row.View, x, 0)
			x = layout.Sum(x, width)
		}
		x = layout.Sum(x, text.Of(c.word(yes), style).Draw(row.View, x, 0))
		x = layout.Sum(x, row.Text(x, 0, "  ", look.Text))
	}
	c.split.Stage(v, split)
}

// Handle answers the field, by key or by pressing one of the two answers.
func (c *Confirm) Handle(ev input.Event) bool {
	requireMutable(c.element)
	if mouse, ok := ev.(input.Mouse); ok {
		local, in := c.within(mouse)
		if !in || !local.Presses(input.ButtonLeft) {
			return false
		}
		c.Say(local.Pos.X < c.geometry.read(&c.split))
		return true
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	_, handled := c.matcher.Handle(c.resolvedKeys(), key, keyboardReceipt(&c.element), c.Do)
	return handled
}

func (c *Confirm) Do(action keymap.Action) bool {
	switch action {
	case SelectPrev:
		c.Say(true)
	case SelectNext:
		c.Say(false)
	case Toggle:
		snapshot := c.Snapshot()
		_, _ = c.requestAnswer(snapshot, !snapshot.Value)
	default:
		return false
	}
	return true
}

func (c *Confirm) Validate() error {
	return c.validate(nil, c.Revision, c.checkAnswer)
}

// SetCheck revokes old results because validation belongs to the rule as well as
// the answer. Reinstalling the same callback also revokes receipts; nil removes it.
func (c *Confirm) SetCheck(check func(bool) error) {
	requireMutable(c.element)
	c.check = check
	c.InvalidateValidation()
}

func (c *Confirm) checkAnswer() error {
	if c.check == nil {
		return nil
	}
	return c.check(c.Answer())
}

func (c *Confirm) focusChanged(has bool) {
	change := func() {
		c.focus(has, nil, c.Validate)
	}
	if has {
		change()
	} else {
		c.matcher.Reset(change)
	}
}

func (c *Confirm) word(yes bool) string {
	switch {
	case yes && c.yes != "":
		return c.yes
	case yes:
		return "yes"
	case c.no != "":
		return c.no
	default:
		return "no"
	}
}

func (c *Confirm) resolvedKeys() *keymap.Map { return c.keys.resolve(confirmKeys) }

func (c *Confirm) Ask() string {
	return c.label + " (" + c.word(true) + "/" + c.word(false) + ")"
}

// Reply validates only an accepted, current answer. A Started request without
// another failure reports ErrConfirmPending. Domain failures wrap
// ErrReplyUnavailable even when the owner already applied or started the effect.
func (c *Confirm) Reply(said string) error {
	return c.validate(func() error {
		at, err := chooseLabel(strings.TrimSpace(said), Options(c.word(true), c.word(false)))
		if err != nil {
			return err
		}
		yesMatch := at == 0
		result, err := c.requestAnswer(c.Snapshot(), yesMatch)
		if result.State == scene.Started && err == nil {
			err = ErrConfirmPending
		}
		if err != nil {
			return fmt.Errorf("%w: %w", ErrReplyUnavailable, err)
		}
		if result.Value.Value != yesMatch || result.Value != c.Snapshot() {
			return ErrValidationSuperseded
		}
		return nil
	}, c.Revision, c.checkAnswer)
}

// Revision identifies a semantic value version; view-only navigation does not change it.
func (c *Confirm) Revision() uint64 { return c.Snapshot().Revision }

func (c *Confirm) Label() string { return c.label }

func (c *Confirm) SetLabel(value string) {
	setting.Assign(c.element, &c.label, strings.Clone(value))
}

func (c *Confirm) Yes() string { return c.yes }

func (c *Confirm) SetYes(value string) {
	setting.Assign(c.element, &c.yes, strings.Clone(value))
}

func (c *Confirm) No() string { return c.no }

func (c *Confirm) SetNo(value string) {
	setting.Assign(c.element, &c.no, strings.Clone(value))
}

func (c *Confirm) Keys() *keymap.Map { return c.keys.current }

// KeysFor is what the effective key map binds to action, defaults included.
func (c *Confirm) KeysFor(action keymap.Action) []input.Keys {
	return c.keys.bound(confirmKeys, action)
}

func (c *Confirm) SetKeys(keys *keymap.Map) {
	c.keys.set(c.element, &c.matcher, keys)
}

func (c *Confirm) Element() scene.Element {
	if c.element.IsZero() {
		set := scene.NewAction(scene.ActionConfig[bool, bool]{ID: scene.ActionSetValue, Label: "Set answer", Run: func(ctx context.Context, value bool) (scene.ActionResult[bool], error) {
			snapshot := c.Snapshot()
			result, err := c.Request(ctx, ConfirmRequest{ExpectedRevision: snapshot.Revision, Value: value})
			return scene.ActionResult[bool]{State: result.State, Value: result.Value.Value}, err
		}})
		c.element = scene.NewElement(scene.ElementConfig{
			Key: c.key, Enabled: func() bool { return !c.closed }, Measure: layout.MeasureRows(c.HeightForWidth), Paint: c.Draw,
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
