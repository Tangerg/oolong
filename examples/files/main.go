// Command files browses a directory beside what is in it.
//
// Two panes and a keyboard that moves between them, which is what a scene is
// for: a key goes to the pane that has it, a press goes to the pane it is over in
// that pane's own coordinates, and neither is worked out by this program. Tab moves
// the keyboard, the arrows move within a pane, right and left open and close a
// branch, and q leaves.
//
// The tree is read once at startup and never again. A real one would watch, and
// would still be this tree — which branches are open is remembered by position, so a
// tree that is replaced under the reader keeps the shape they gave it.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	app, err := program.NewScreen(program.ScreenConfig{Terminal: term.Features{Probe: true, Mouse: true}, Build: func(runtime *program.Runtime) (scene.Element, error) {
		return newBrowser(runtime, read(root, 2)).Element(), nil
	}})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "files:", err)
		os.Exit(1)
	}
}

// entry is one file or directory: what it is called and where it is.
type entry struct {
	name string
	path string
	dir  bool
}

// browser composes two scene children. The tree owns selection; its change
// notification starts a scoped preview task whose stale delivery is revoked.
type browser struct {
	runtime *program.Runtime
	theme   kit.Theme

	tree         *base.Tree[entry]
	dressed      *kit.Tree[entry]
	preview      *kit.Paragraph
	window       *base.Viewport
	treeBox      *kit.Panel
	viewBox      *kit.Panel
	body         *scene.Group
	element      scene.Element
	previewScope *program.Scope

	showing string
}

func newBrowser(runtime *program.Runtime, nodes []base.Node[entry]) *browser {
	theme := kit.Suited(runtime.Environment().Ground())
	b := &browser{
		runtime: runtime,
		theme:   theme,
		preview: &kit.Paragraph{},
	}
	// The tree with a look on it. It is still a widget — it takes the keyboard and
	// answers events by passing them down — so the container holds this rather than
	// the bare tree, and nothing here has to dress it at drawing time.
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	b.dressed = kit.NewTree(kit.TreeConfig[entry]{
		Theme: theme, Glyphs: glyphs, Nodes: nodes,
		Text: func(e entry) string { return e.name },
	})
	b.tree = b.dressed.Controller()
	previewElement := scene.NewElement(scene.ElementConfig{
		Key:   "file-preview",
		Paint: func(frame scene.Frame) { b.preview.Draw(frame.View) },
		Measure: func(axis layout.Axis, width int) int {
			if axis == layout.Down {
				return b.preview.HeightForWidth(width)
			}
			return 0
		},
	})
	b.window = base.NewViewport(base.ViewportConfig{Content: previewElement})
	b.treeBox = kit.NewPanel(kit.PanelConfig{Box: kit.Box{Theme: theme, Glyphs: glyphs, Title: "files"}, Content: b.dressed.Element()})
	b.viewBox = kit.NewPanel(kit.PanelConfig{Box: kit.Box{Theme: theme, Glyphs: glyphs, Title: "preview"}, Content: b.window.Element()})
	b.body = scene.NewGroup(scene.GroupConfig{Axis: layout.Across, Gap: 1, Items: []scene.Item{
		{Size: layout.Part(2, 5), Element: b.treeBox.Element()},
		{Size: layout.Flex(1), Element: b.viewBox.Element()},
	}})
	footer := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		kit.Label{Text: "tab: other pane   →/←: open, close   q: quit", Style: b.theme.Subtle}.Draw(frame.View)
	}})
	unsubscribe := b.tree.Subscribe(b.show)
	b.element = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Element: scene.ElementConfig{
		Key: "files",
		Preview: func(event scene.Event) scene.EventResult {
			if key, ok := event.Input.(input.Key); ok && key.Down() && key.Code == input.Tab {
				return scene.EventResult{Consumed: event.FocusNext(key.Mods.Has(input.Shift))}
			}
			return scene.EventResult{Consumed: b.Handle(event.Input)}
		},
		Detached: func() {
			unsubscribe()
			if b.previewScope != nil {
				b.previewScope.Close()
			}
		},
	}, Items: []scene.Item{
		{Size: layout.Flex(1), Element: b.body.Element()},
		{Size: layout.Fixed(1), Element: footer},
	}}).Element()
	b.show()
	return b
}

func (b *browser) Element() scene.Element { return b.element }

func (b *browser) show() {
	row, ok := b.tree.CurrentRow()
	if !ok || row.Item.path == b.showing {
		return
	}
	b.showing = row.Item.path
	if b.previewScope != nil {
		b.previewScope.Close()
	}
	selected, theme := row.Item, b.theme
	b.preview.SetText([]text.Line{text.Of("Loading "+selected.path, theme.Muted)})
	b.window.Scroll().ToTop()
	scope, err := b.runtime.NewScope(program.ScopeConfig{})
	if err != nil {
		b.preview.SetText([]text.Line{text.Of(err.Error(), theme.Danger)})
		return
	}
	b.previewScope = scope
	_, err = b.runtime.Start(scope, program.Work[[]text.Line]{
		Run: func(ctx context.Context) ([]text.Line, error) {
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
			return preview(selected, theme), nil
		},
		Apply: func(lines []text.Line, err error) {
			defer scope.Close()
			if err != nil {
				lines = []text.Line{text.Of(err.Error(), theme.Danger)}
			}
			b.preview.SetText(lines)
		},
	})
	if err != nil {
		scope.Close()
		b.preview.SetText([]text.Line{text.Of(err.Error(), theme.Danger)})
	}
}

func (b *browser) Handle(ev input.Event) bool {
	if key, ok := ev.(input.Key); ok && key.Down() && key.Rune == 'q' {
		b.runtime.Quit()
		return true
	}
	return false
}

func preview(of entry, theme kit.Theme) []text.Line {
	if of.dir {
		return []text.Line{text.Of(of.path+" is a directory", theme.Muted)}
	}
	// A real browser would read as much as the window can show. Reading a fixed
	// amount is the same idea with the size decided here rather than there.
	file, err := openPreview(of.path)
	if err != nil {
		return []text.Line{text.Of(err.Error(), theme.Danger)}
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return []text.Line{text.Of(err.Error(), theme.Danger)}
	}
	if !info.Mode().IsRegular() {
		return []text.Line{text.Of("Preview requires a regular file", theme.Danger)}
	}
	body, err := io.ReadAll(io.LimitReader(file, 64<<10))
	if err != nil {
		return []text.Line{text.Of(err.Error(), theme.Danger)}
	}
	var lines []text.Line
	for i, row := range strings.Split(string(body), "\n") {
		if i >= 200 {
			break
		}
		lines = append(lines, text.Of(row, theme.Text))
	}
	return lines
}

func read(root string, depth int) []base.Node[entry] {
	if depth < 0 {
		return nil
	}
	items, err := os.ReadDir(root)
	if err != nil {
		// A directory nobody may open is not an empty directory, and drawing it as one
		// tells the reader something untrue about their own filesystem.
		return []base.Node[entry]{{Item: entry{name: filepath.Base(root) + ": " + err.Error()}}}
	}
	slices.SortFunc(items, func(a, b os.DirEntry) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name(), b.Name())
	})

	var nodes []base.Node[entry]
	for _, item := range items {
		if strings.HasPrefix(item.Name(), ".") {
			continue
		}
		path := filepath.Join(root, item.Name())
		node := base.Node[entry]{
			Item: entry{name: item.Name(), path: path, dir: item.IsDir()},
		}
		if item.IsDir() {
			node.Children = read(path, depth-1)
		}
		nodes = append(nodes, node)
	}
	return nodes
}
