// Command picker composes editor, filter and list capabilities so application
// search policy stays outside reusable controls.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/fuzzy"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

func main() {
	chosen := ""
	app, err := program.NewScreen(program.ScreenConfig{Terminal: term.Features{Probe: true}, Build: func(runtime *program.Runtime) (scene.Element, error) {
		return newPicker(runtime, files(), &chosen).Element(), nil
	}})
	if err == nil {
		err = app.Run(context.Background())
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "picker:", err)
		os.Exit(1)
	}
	if chosen != "" {
		fmt.Println(chosen)
	}
}

type picker struct {
	runtime *program.Runtime
	theme   kit.Theme
	query   *kit.Composer
	list    *base.Filter[string]
	chosen  *string
	element scene.Element
}

func newPicker(runtime *program.Runtime, items []string, chosen *string) *picker {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())

	p := &picker{runtime: runtime, theme: theme, chosen: chosen}
	p.query = kit.NewComposer(kit.ComposerConfig{
		Theme:  theme,
		Prompt: glyphs.Marker + " ",
	})
	p.query.Controller().Placeholder = "type to narrow"
	p.list = &base.Filter[string]{
		Row: p.row,
	}
	// What an item reads as travels with the items: an item is whatever the caller
	// says it is, so this is the one thing the filter cannot work out for itself.
	p.list.SetItems(items, func(s string) string { return s })
	if err := p.list.Element().SetFocusable(func() bool { return false }); err != nil {
		panic(err)
	}
	unsubscribe := p.query.Controller().Session().Subscribe(func(base.ChangeMap) {
		p.list.SetPattern(p.query.Controller().Text())
	})
	footer := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		kit.Label{Text: fmt.Sprintf("%d of %d", p.list.Matched(), p.list.Len()), Style: p.theme.Subtle, Align: layout.End}.Draw(frame.View)
	}})
	p.element = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Element: scene.ElementConfig{
		Key: "picker", Preview: func(event scene.Event) scene.EventResult { return scene.EventResult{Consumed: p.Handle(event.Input())} }, Detached: unsubscribe,
	}, Items: []scene.Item{
		{Size: layout.Fixed(1), Element: p.query.Element()},
		{Size: layout.Flex(1), Element: p.list.Element()},
		{Size: layout.Fixed(1), Element: footer},
	}}).Element()
	return p
}

func (p *picker) Element() scene.Element { return p.element }

func (p *picker) row(v grid.View, _ int, item string, match fuzzy.Match, selected bool) {
	base := p.theme.Text
	if selected {
		base = base.Merge(p.theme.Selection)
		v.Fill(v.Bounds(), p.theme.Selection)
	}
	text.Highlight(item, match.At, base, base.Merge(p.theme.Accent)).Draw(v, 0, 0)
}

// Preview keeps navigation policy on the composition. The query's own scene
// element receives editing and paste; the filter's element receives pointer input.
func (p *picker) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	for chord := range key.ShortcutChords() {
		switch chord.Code {
		case input.Enter:
			p.pick()
			return true
		case input.Esc:
			p.runtime.Quit()
			return true
		case input.Up:
			return p.list.Do(base.SelectPrev)
		case input.Down:
			return p.list.Do(base.SelectNext)
		case input.PageUp:
			return p.list.Do(base.SelectPageUp)
		case input.PageDown:
			return p.list.Do(base.SelectPageDown)
		default:
			continue
		}
	}
	return false
}

func (p *picker) pick() {
	if got, ok := p.list.Current(); ok {
		*p.chosen = got
	}
	p.runtime.Quit()
}

func files() []string {
	directories := []string{"ui/core/grid", "ui/core/text", "ui/core/term", "ui/base", "ui/kit"}
	names := []string{"doc.go", "main.go", "reader.go", "writer.go", "view.go"}
	out := make([]string, 0, len(directories)*len(names))
	for _, dir := range directories {
		for _, name := range names {
			out = append(out, strings.Join([]string{dir, name}, "/"))
		}
	}
	return out
}
