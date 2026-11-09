// Command files revokes obsolete preview tasks so late file reads cannot replace
// the selection currently owned by the tree.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/examples/internal/regularfile"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	directoryScanDepth               = 2
	maxPreviewBytes                  = 64 << 10
	maxPreviewRows                   = 200
	quitBrowser        keymap.Action = "quit"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	app, err := program.NewScreen(program.ScreenConfig{Terminal: term.Features{Probe: true, Mouse: true}, Build: func(runtime *program.Runtime) (scene.Element, error) {
		nodes, readErr := read(root, directoryScanDepth)
		if readErr != nil {
			return scene.Element{}, readErr
		}
		return newBrowser(runtime, nodes).Element(), nil
	}})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "files:", err)
		os.Exit(1)
	}
}

type entry struct {
	name    string
	path    string
	dir     bool
	scanErr error
}

// The tree owns selection. Scoped preview delivery is revoked on a later
// selection so stale I/O cannot replace current content.
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
}

func newBrowser(runtime *program.Runtime, nodes []base.Node[entry]) *browser {
	theme := kit.Suited(runtime.Environment().Ground())
	b := &browser{
		runtime: runtime,
		theme:   theme,
		preview: &kit.Paragraph{},
	}
	// The dressed tree retains its controller's scene identity, so this container
	// can delegate input without duplicating tree behavior.
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	b.dressed = kit.NewTree(kit.TreeConfig[entry]{
		Theme: theme, Glyphs: glyphs, Nodes: nodes,
		Text: func(e entry) string { return e.name },
	})
	b.tree = b.dressed.Controller()
	keys := base.DefaultTreeKeys()
	for _, binding := range base.DefaultFocusKeys().Bindings() {
		keys.Bind(binding.Action, binding.Keys...)
	}
	keys.Bind(quitBrowser, input.Chord{Rune: 'q'})
	b.tree.SetKeys(keys)
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
		actions := []keymap.Action{base.FocusNext, quitBrowser}
		if b.tree.Element().FocusedWithin() {
			actions = []keymap.Action{base.FocusNext, base.Expand, base.Collapse, quitBrowser}
		}
		kit.Help{Theme: b.theme, Keys: b.tree.Keys(), Show: actions}.Draw(frame.View)
	}})
	unsubscribe := b.tree.Subscribe(b.show)
	b.element = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Element: scene.ElementConfig{
		Key: "files",
		Preview: func(event scene.Event) scene.EventResult {
			if key, ok := event.Input().(input.Key); ok {
				if action, bound := b.tree.Keys().ActionFor(key); bound && (action == base.FocusNext || action == base.FocusPrev) {
					return scene.EventResult{Consumed: event.FocusNext(action == base.FocusPrev)}
				}
			}
			return scene.EventResult{Consumed: b.Handle(event.Input())}
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
	if b.previewScope != nil {
		b.previewScope.Close()
		b.previewScope = nil
	}
	b.window.Scroll().ToTop()
	selected, ok := b.tree.Current()
	if !ok {
		b.preview.SetText(nil)
		return
	}
	theme := b.theme
	if selected.dir {
		b.preview.SetText(preview(selected, theme))
		return
	}
	b.preview.SetText([]text.Line{text.Of("Loading "+selected.path, theme.Muted)})
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
	if key, ok := ev.(input.Key); ok {
		if action, bound := b.tree.Keys().ActionFor(key); bound && action == quitBrowser {
			b.runtime.Quit()
			return true
		}
	}
	return false
}

func preview(of entry, theme kit.Theme) []text.Line {
	if of.scanErr != nil {
		return []text.Line{text.Of(of.scanErr.Error(), theme.Danger)}
	}
	if of.dir {
		return []text.Line{text.Of(of.path+" is a directory", theme.Muted)}
	}
	// Preview budgets bound retained file content independently of terminal size.
	file, err := regularfile.Open(of.path)
	if err != nil {
		return []text.Line{text.Of(err.Error(), theme.Danger)}
	}
	defer func() { _ = file.Close() }()
	body, err := io.ReadAll(io.LimitReader(file, maxPreviewBytes))
	if err != nil {
		return []text.Line{text.Of(err.Error(), theme.Danger)}
	}
	var lines []text.Line
	for row := range strings.SplitSeq(string(body), "\n") {
		if len(lines) >= maxPreviewRows {
			break
		}
		lines = append(lines, text.Of(row, theme.Text))
	}
	return lines
}

func read(root string, depth int) ([]base.Node[entry], error) {
	if depth < 0 {
		return nil, nil
	}
	items, err := os.ReadDir(root)
	if err != nil {
		return nil, err
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
			node.Children, node.Item.scanErr = read(path, depth-1)
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}
