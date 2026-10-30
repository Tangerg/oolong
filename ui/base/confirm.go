package base

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
)

// Confirm is a field holding a yes or a no.
//
// The zero value answers no and is ready. A Confirm must not be copied after first
// use: its answer, matcher and committed pointer split are one mutable field.
// NewConfirm installs its initial answer. Say is the only answer transition.
type Confirm struct {
	noCopy noCopy

	field
	Label string
	// Yes and No are the two answers as they are shown. Empty uses "yes" and "no".
	Yes, No string
	// Check says what is wrong with the answer, or nil.
	Check func(v bool) error
	// Keys say which keystrokes answer. Nil reads through [DefaultConfirmKeys].
	Keys *keymap.Map

	control     ConfirmControl
	unsubscribe func()
	closed      bool
	changeErr   error
	answer      ownedValue[bool]
	matcher     keymap.Matcher
	// split is the committed column where the second answer begins.
	split scene.Snapshot[int]
}

// Prompt is what the field is asking.
func (c *Confirm) Prompt() string { return c.Label }

// Answer is what has been answered.
func (c *Confirm) Answer() bool {
	return c.Snapshot().Value
}

// Say answers the field.
// Say requests an answer against the current revision. ChangeError reports a
// rejected UI request; use Request when the caller needs the typed result directly.
func (c *Confirm) Say(yes bool) {
	snapshot := c.Snapshot()
	_, c.changeErr = c.Request(context.Background(), ConfirmRequest{ExpectedRevision: snapshot.Revision, Value: yes})
}

// HeightForWidth is the label, the two answers on one row, and the problem if there is one.
func (c *Confirm) HeightForWidth(int) int { return layout.Sum(1, c.rows(c.Label)) }

// Draw paints the label and the two answers.
func (c *Confirm) Draw(v scene.Frame) {
	c.DrawWith(v, Look{})
}

// DrawWith paints the field using this frame's look without changing configuration.
func (c *Confirm) DrawWith(v scene.Frame, look Look) {
	answer := c.Answer()
	row := c.frame(v, c.Label, look)
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
			x = layout.Sum(x, row.Text(x, 0, mark, style), 1)
		}
		x = layout.Sum(x, row.Text(x, 0, c.word(yes), style))
		x = layout.Sum(x, row.Text(x, 0, "  ", look.Text))
	}
	c.split.Stage(v, split)
}

// Handle answers the field, by key or by pressing one of the two answers.
func (c *Confirm) Handle(ev input.Event) bool {
	requireMutable(c.element)
	if mouse, ok := ev.(input.Mouse); ok {
		local, in := c.within(mouse)
		if !in || local.Action != input.MouseDown || local.Button != input.ButtonLeft {
			return false
		}
		c.Say(local.Pos.X < c.geometry.read(&c.split))
		return true
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	_, handled := c.matcher.Handle(c.keys(), key, c.Do)
	return handled
}

// Do runs one of the field's actions by name. See [Doer].
func (c *Confirm) Do(action keymap.Action) bool {
	switch action {
	case SelectPrev:
		c.Say(true)
	case SelectNext:
		c.Say(false)
	case Toggle:
		snapshot := c.Snapshot()
		_, c.changeErr = c.Request(context.Background(), ConfirmRequest{ExpectedRevision: snapshot.Revision, Value: !snapshot.Value})
	default:
		return false
	}
	return true
}

// Validate checks the answer.
func (c *Confirm) Validate() error {
	return c.validate(nil, c.checkAnswer)
}

func (c *Confirm) checkAnswer() fieldCheck {
	snapshot := c.Snapshot()
	answer, revision := snapshot.Value, snapshot.Revision
	checked := fieldCheck{current: func() bool { return c.Revision() == revision }}
	if c.Check != nil {
		checked.err = c.Check(answer)
	}
	return checked
}

// Focus takes the keyboard or gives it up, and checks the answer on the way out.
func (c *Confirm) focusChanged(has bool) {
	change := func() {
		c.focus(has, nil, nil, c.Validate)
	}
	if has {
		change()
	} else {
		c.matcher.Reset(change)
	}
}

func (c *Confirm) word(yes bool) string {
	switch {
	case yes && c.Yes != "":
		return c.Yes
	case yes:
		return "yes"
	case c.No != "":
		return c.No
	default:
		return "no"
	}
}

func (c *Confirm) keys() *keymap.Map {
	if c.Keys != nil {
		return c.Keys
	}
	return confirmKeys()
}

// Ask is the question and the two words that answer it.
func (c *Confirm) Ask() string {
	return c.Label + " (" + c.word(true) + "/" + c.word(false) + ")"
}

// Reply takes either answer, or as much of one as is unambiguous — nobody types
// "yes" in full twice.
func (c *Confirm) Reply(said string) error {
	return c.validate(func() error {
		said = strings.ToLower(strings.TrimSpace(said))
		yes, no := strings.ToLower(c.word(true)), strings.ToLower(c.word(false))
		yesMatch, noMatch := strings.HasPrefix(yes, said), strings.HasPrefix(no, said)
		switch {
		case said == "":
			return fmt.Errorf("say %s or %s", yes, no)
		case yesMatch && noMatch:
			return fmt.Errorf("%q could mean either %s or %s", said, yes, no)
		case yesMatch:
			c.Say(true)
		case noMatch:
			c.Say(false)
		default:
			return fmt.Errorf("say %s or %s", yes, no)
		}
		return nil
	}, c.checkAnswer)
}

// Revision identifies a semantic value version; view-only navigation does not change it.
func (c *Confirm) Revision() uint64 { return c.Snapshot().Revision }
