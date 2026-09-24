// Command files browses a directory beside what is in it.
//
// Two panes and a keyboard that moves between them, which is what a container is
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

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
	"github.com/Tangerg/oolong/core/text"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if err := program.Run(context.Background(), program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			return headless.NewRoot(newBrowser(runtime, read(root, 2)))
		},
		Terminal: term.Features{Probe: true, Mouse: true},
	}); err != nil {
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

// browser is a tree, a preview, and the container that decides which of them an
// event is for.
type browser struct {
	runtime *program.Runtime
	theme   kit.Theme

	tree    *headless.Tree[entry]
	dressed *kit.Tree[entry]
	preview *kit.Paragraph
	window  *headless.Viewport
	treeBox *kit.Panel
	viewBox *kit.Panel
	body    *headless.Container

	showing    string
	generation uint64
}

func newBrowser(runtime *program.Runtime, nodes []headless.Node[entry]) *browser {
	theme := kit.Suited(runtime.Environment().Ground())
	b := &browser{
		runtime: runtime,
		theme:   theme,
		tree:    headless.NewTree(nodes...),
		preview: &kit.Paragraph{},
	}
	// The tree with a look on it. It is still a widget — it takes the keyboard and
	// answers events by passing them down — so the container holds this rather than
	// the bare tree, and nothing here has to dress it at drawing time.
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	b.dressed = kit.NewTree(kit.TreeConfig[entry]{
		Theme: theme, Glyphs: glyphs, Controller: b.tree,
		Text: func(e entry) string { return e.name },
	})
	// A window shows the part of something taller than the room there is. The preview
	// is ordinary wrapped text and knows nothing about being scrolled.
	b.window = headless.NewViewport(headless.Static{Of: b.preview})
	b.treeBox = kit.NewPanel(kit.PanelConfig{Box: kit.Box{Theme: theme, Glyphs: glyphs}, Content: b.dressed})
	b.treeBox.Box.Title = "files"
	b.viewBox = kit.NewPanel(kit.PanelConfig{Box: kit.Box{Theme: theme, Glyphs: glyphs}, Content: b.window})
	b.viewBox.Box.Title = "preview"

	// The two framed panes, with a column between them. A panel translates from its
	// border to its child, while the container translates from the whole row to the
	// panel. Each object owns exactly the coordinate boundary it drew.
	b.body = headless.NewContainer(layout.Across,
		headless.Item{Size: layout.Part(2, 5), Of: b.treeBox},
		headless.Item{Size: layout.Flex(1), Of: b.viewBox},
	)
	b.body.Gap = 1
	b.body.Focus(true)
	b.show()
	return b
}

// Draw paints the two panes, and a hint row under them.
func (b *browser) Draw(v headless.Frame) {
	rows := v.Subs((layout.Flow{Axis: layout.Down}).Rects(v.Bounds().Size(), []layout.Slot{
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(1)},
	}))
	b.body.Draw(rows[0])

	kit.Label{
		Text:  "tab: other pane   →/←: open, close   q: quit",
		Style: b.theme.Subtle,
	}.Draw(rows[1].View)
}

func (b *browser) show() {
	row, ok := b.tree.CurrentRow()
	if !ok || row.Item.path == b.showing {
		return
	}
	b.showing = row.Item.path
	b.generation++
	generation := b.generation
	selected, theme := row.Item, b.theme
	b.preview.SetText([]text.Line{text.Of("Loading "+selected.path, theme.Muted)})
	b.window.Scroll().ToTop()
	dispatcher := b.runtime.Dispatcher()
	go func() {
		lines := preview(selected, theme)
		dispatcher.Post(func() {
			if b.generation == generation {
				b.preview.SetText(lines)
			}
		})
	}()
}

func (b *browser) Handle(ev input.Event) bool {
	if key, ok := ev.(input.Key); ok && key.Down() && key.Rune == 'q' {
		b.runtime.Quit()
		return true
	}
	handled := b.body.Handle(ev)
	b.show()
	return handled
}

// preview is what to show beside the tree: the first part of a file, or what a
// directory holds.
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

func read(root string, depth int) []headless.Node[entry] {
	if depth < 0 {
		return nil
	}
	items, err := os.ReadDir(root)
	if err != nil {
		// A directory nobody may open is not an empty directory, and drawing it as one
		// tells the reader something untrue about their own filesystem.
		return []headless.Node[entry]{{Item: entry{name: filepath.Base(root) + ": " + err.Error()}}}
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

	var nodes []headless.Node[entry]
	for _, item := range items {
		if strings.HasPrefix(item.Name(), ".") {
			continue
		}
		path := filepath.Join(root, item.Name())
		node := headless.Node[entry]{
			Item: entry{name: item.Name(), path: path, dir: item.IsDir()},
		}
		if item.IsDir() {
			node.Children = read(path, depth-1)
		}
		nodes = append(nodes, node)
	}
	return nodes
}
