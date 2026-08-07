// Command picker narrows a list as you type.
//
// It is three things put together, and the point is that they are three things: a
// field that takes the typing, a fuzzy match that scores every candidate against
// what was typed, and a list that shows what is left. The library deliberately does
// not sell you a "picker" — where the field goes, what a row looks like and what
// choosing one means are all decisions this program makes in twenty lines.
//
// Type to narrow. Up and down move, Enter picks, Esc leaves.
package main

import (
	"context"
	"fmt"
	"image"
	"os"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/fuzzy"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
	"github.com/Tangerg/oolong/core/text"
)

func main() {
	chosen := ""
	err := program.Run(context.Background(), program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			return headless.NewRoot(newPicker(runtime, files(), &chosen))
		},
		Terminal: term.Options{Probe: true},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "picker:", err)
		os.Exit(1)
	}
	if chosen != "" {
		fmt.Println(chosen)
	}
}

// picker is a query and the matches under it.
type picker struct {
	runtime *program.Runtime
	theme   kit.Theme
	query   kit.Composer
	list    *headless.Filter[string]
	chosen  *string
	areas   headless.Snapshot[pickerAreas]
}

func newPicker(runtime *program.Runtime, items []string, chosen *string) *picker {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(os.Getenv)

	p := &picker{runtime: runtime, theme: theme, chosen: chosen}
	p.query = kit.Composer{
		Theme:       theme,
		Prompt:      glyphs.Marker + " ",
		Placeholder: "type to narrow",
	}
	p.list = &headless.Filter[string]{
		Items: items,
		// What an item reads as. An item is whatever the caller says it is, so this is
		// the one thing the filter cannot work out for itself.
		Text: func(s string) string { return s },
		Row:  p.row,
	}
	return p
}

// Draw stacks the query over the matches, with a count where there is room.
func (p *picker) Draw(v headless.Frame) {
	p.areas.Stage(v, pickerAreas{})
	rects := layout.Down.Rects(v.Bounds().Size(),
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(1)},
	)
	rows := v.Subs(rects)
	p.areas.Stage(v, pickerAreas{query: rects[0], list: rects[1]})
	p.query.Draw(rows[0])
	p.list.Draw(rows[1])
	kit.Label{
		Text:  fmt.Sprintf("%d of %d", p.list.Matched(), len(p.list.Items)),
		Style: p.theme.Subtle,
		Align: layout.End,
	}.Draw(rows[2].View)
}

type pickerAreas struct{ query, list image.Rectangle }

// row draws one match, with the characters that answered the query picked out.
//
// The offsets are bytes and can land inside a grapheme cluster — a query character
// can match a combining mark — so a cluster is emphasised when it contains one rather
// than when it begins at one. That is the whole reason the match is handed over
// instead of being worked out again here.
func (p *picker) row(v grid.View, _ int, item string, match fuzzy.Match, selected bool) {
	base := p.theme.Text
	if selected {
		base = base.Merge(p.theme.Selection)
		v.Fill(v.Bounds(), p.theme.Selection)
	}
	hit := base.Merge(p.theme.Accent)

	next, at := 0, 0
	for off, cluster := range text.Clusters(item) {
		for next < len(match.At) && match.At[next] < off {
			next++
		}
		style := base
		if next < len(match.At) && match.At[next] < off+len(cluster) {
			style = hit
		}
		at += v.Text(at, 0, cluster, style)
	}
}

// Handle sends the arrows to the list and everything else to the field, and keeps
// the two in step: what was typed is what the list is narrowed by.
func (p *picker) Handle(ev input.Event) bool {
	if key, ok := ev.(input.Key); ok && key.Down() {
		switch {
		case key.Code == input.Enter:
			p.pick()
			return true
		case key.Code == input.Esc:
			p.runtime.Quit()
			return true
		case moves(key.Code):
			// The field would take these too — it has a cursor of its own — so the list
			// is asked first for exactly the four that mean "move through the matches".
			return p.list.Handle(ev)
		}
	}
	if mouse, ok := ev.(input.Mouse); ok {
		areas := p.areas.Value()
		switch {
		case mouse.Pos.In(areas.query):
			mouse.Pos = mouse.Pos.Sub(areas.query.Min)
			return p.query.Handle(mouse)
		case mouse.Pos.In(areas.list):
			mouse.Pos = mouse.Pos.Sub(areas.list.Min)
			return p.list.Handle(mouse)
		default:
			return false
		}
	}
	if p.query.Handle(ev) {
		p.list.SetPattern(p.query.Text())
		return true
	}
	return false
}

// moves reports whether a key is one of the four that mean "through the matches"
// rather than "through what I typed".
func moves(code input.Code) bool {
	switch code {
	case input.Up, input.Down, input.PageUp, input.PageDown:
		return true
	default:
		return false
	}
}

func (p *picker) pick() {
	if got, ok := p.list.Current(); ok {
		*p.chosen = got
	}
	p.runtime.Quit()
}

// files is what there is to choose from. A real one would read a directory or a
// history; what matters here is that it is a slice of the program's own things.
func files() []string {
	var out []string
	for _, dir := range []string{"core/grid", "core/text", "core/term", "components/headless", "components/kit"} {
		for _, name := range []string{"doc.go", "main.go", "reader.go", "writer.go", "view.go"} {
			out = append(out, strings.Join([]string{dir, name}, "/"))
		}
	}
	return out
}
