package base

import (
	"context"
	"image"
	"strings"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// DialogConfig supplies open state, semantic text and one content relation. Place and Backdrop are pure presentation callbacks; keys and outside presses request the same Dismiss transition.
type DialogConfig struct {
	Initial            bool
	Title, Description string
	Content            scene.Element
	Place              func(image.Point) image.Rectangle
	Backdrop           func(scene.Frame)
	DismissOutside     bool
	Keys               *keymap.Map
}

// Dialog owns its open policy. The private modal group is its only attachment
// relation; scene owns capture, focus trapping and restoration of valid targets.
// Dialog must not be copied after first use; its collaborators share one live identity.
type Dialog struct {
	noCopy         noCopy
	open           bool
	title, detail  string
	content        scene.Element
	place          func(image.Point) image.Rectangle
	backdrop       func(scene.Frame)
	dismissOutside bool
	keys           *keymap.Map
	matcher        keymap.Matcher
	group          *scene.Group
	presentation   scene.Snapshot[image.Rectangle]
}

// NewDialog constructs a modal controller. It panics for empty content or, when initially open, content already owned by another scene relation.
func NewDialog(config DialogConfig) *Dialog {
	if config.Content.IsZero() {
		panic("base: dialog requires content")
	}
	d := &Dialog{title: strings.Clone(config.Title), detail: strings.Clone(config.Description), content: config.Content, place: config.Place, backdrop: config.Backdrop, dismissOutside: config.DismissOutside, keys: config.Keys}
	d.ensure()
	if config.Initial {
		d.Show()
	}
	return d
}

func (d *Dialog) ensure() {
	if d.group != nil {
		return
	}
	dismiss := scene.NewAction(scene.ActionConfig[struct{}, struct{}]{ID: scene.ActionDismiss, Label: "Dismiss", Availability: func(struct{}) scene.Availability {
		return scene.Availability{Disabled: !d.open, Reason: "dialog is closed"}
	}, Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
		d.Dismiss()
		return scene.ActionResult[struct{}]{State: scene.Applied}, nil
	}})
	d.group = scene.NewGroup(scene.GroupConfig{
		Overlay: true, Modal: true,
		Element: scene.ElementConfig{
			Enabled: func() bool { return d.open }, Handle: d.handleEvent,
			Paint: func(frame scene.Frame) {
				if !d.open {
					return
				}
				if d.backdrop != nil {
					d.backdrop(frame)
				}
				d.presentation.Stage(frame, d.placement(frame.Bounds().Size()))
			}, Semantics: func() scene.Semantic {
				state := scene.SemanticState(0)
				if d.open {
					state = scene.StateOpen
				}
				return scene.Semantic{Role: scene.RoleDialog, Name: d.title, Description: d.detail, State: state}
			}, Actions: []scene.ActionBinding{dismiss.Binding()},
		},
		Layout: func(space image.Point, items []scene.Item) []image.Rectangle {
			if len(items) == 0 {
				return nil
			}
			return []image.Rectangle{d.placement(space)}
		},
	})
}

func (d *Dialog) placement(space image.Point) image.Rectangle {
	if d.place != nil {
		return d.place(space)
	}
	return image.Rectangle{Max: space}
}

// Element is the modal group whose attachment, focus trap and capture belong to scene.
func (d *Dialog) Element() scene.Element { d.ensure(); return d.group.Element() }

// Open reports the current open policy. A nil dialog is closed.
func (d *Dialog) Open() bool { return d != nil && d.open }

// Show opens the content and requests modal focus settlement. It panics for missing or foreign-owned content, or during presentation or observation.
func (d *Dialog) Show() {
	if d == nil || d.open {
		return
	}
	d.ensure()
	if d.content.IsZero() {
		panic("base: dialog requires content")
	}
	requireMutable(d.Element())
	d.open = true
	if err := d.group.Set(scene.Item{Element: d.content, Size: layout.Flex(1)}); err != nil {
		d.open = false
		panic(err)
	}
	d.Element().Invalidate()
}

// Dismiss closes the modal group and lets scene restore valid focus. It panics during presentation or observation.
func (d *Dialog) Dismiss() {
	if d == nil || !d.open {
		return
	}
	requireMutable(d.Element())
	d.open = false
	if err := d.group.Set(); err != nil {
		d.open = true
		panic(err)
	}
	d.matcher.Clear()
	d.Element().Invalidate()
}

// Content is the one child owned by this dialog while open.
func (d *Dialog) Content() scene.Element {
	if d == nil {
		return scene.Element{}
	}
	return d.content
}

// SetContent atomically replaces the dialog body. It panics for empty content, an invalid active child relation, or mutation during presentation or observation.
func (d *Dialog) SetContent(content scene.Element) {
	requireMutable(d.Element())
	if content.IsZero() {
		panic("base: dialog requires content")
	}
	if d.open {
		if err := d.group.Set(scene.Item{Element: content, Size: layout.Flex(1)}); err != nil {
			panic(err)
		}
	}
	d.content = content
	d.Element().Invalidate()
}

// Title is the dialog's semantic name.
func (d *Dialog) Title() string { return d.title }

// SetTitle replaces the owned semantic name. It panics during presentation or observation.
func (d *Dialog) SetTitle(title string) {
	requireMutable(d.Element())
	d.title = strings.Clone(title)
	d.Element().Invalidate()
}

// Description is the dialog's explanatory semantic text.
func (d *Dialog) Description() string { return d.detail }

// SetDescription replaces the owned explanation. It panics during presentation or observation.
func (d *Dialog) SetDescription(description string) {
	requireMutable(d.Element())
	d.detail = strings.Clone(description)
	d.Element().Invalidate()
}

// Trigger creates an ordinary button whose action requests Show; the dialog remains the only open-state owner.
func (d *Dialog) Trigger(label string) *Button {
	return NewButton(ButtonConfig{Label: label, Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
		state := scene.Unchanged
		if !d.open {
			d.Show()
			state = scene.Applied
		}
		return scene.ActionResult[struct{}]{State: state}, nil
	}})
}

func (d *Dialog) handleEvent(event scene.Event) scene.EventResult {
	if !d.open {
		return scene.EventResult{}
	}
	if mouse, ok := event.Input.(input.Mouse); ok {
		area, valid := event.Read(&d.presentation)
		if valid && mouse.Action == input.MouseDown && mouse.Button == input.ButtonLeft && !mouse.Pos.In(area) && d.dismissOutside {
			d.Dismiss()
			return scene.EventResult{Consumed: true}
		}
		return scene.EventResult{}
	}
	key, ok := event.Input.(input.Key)
	if !ok {
		return scene.EventResult{}
	}
	if key.Is(input.Tab, 0) {
		return scene.EventResult{Consumed: event.FocusNext(false)}
	}
	if key.Is(input.Tab, input.Shift) {
		return scene.EventResult{Consumed: event.FocusNext(true)}
	}
	keys := d.keys
	if keys == nil {
		keys = dialogKeys()
	}
	_, handled := d.matcher.Handle(keys, key, func(action keymap.Action) bool {
		if action != Close {
			return false
		}
		d.Dismiss()
		return true
	})
	return scene.EventResult{Consumed: handled}
}
