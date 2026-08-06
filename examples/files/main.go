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
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
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
			return newBrowser(runtime, read(root, 2))
		},
		Terminal: term.Options{Probe: true, Mouse: true},
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
	body    *headless.Container

	showing string
}

func newBrowser(runtime *program.Runtime, nodes []headless.Node[entry]) *browser {
	theme := kit.Suited(runtime.Environment().Ground())
	b := &browser{
		runtime: runtime,
		theme:   theme,
		tree:    &headless.Tree[entry]{Nodes: nodes},
		preview: &kit.Paragraph{},
	}
	// The tree with a look on it. It is still a widget — it takes the keyboard and
	// answers events by passing them down — so the container holds this rather than
	// the bare tree, and nothing here has to dress it at drawing time.
	b.dressed = &kit.Tree[entry]{
		Of:     b.tree,
		Text:   func(e entry) string { return e.name },
		Theme:  theme,
		Glyphs: kit.GlyphsFor(os.Getenv),
	}
	// A window shows the part of something taller than the room there is. The preview
	// is ordinary wrapped text and knows nothing about being scrolled.
	b.window = &headless.Viewport{Content: b.preview}

	// The two panes, with a column between them. Which one has the keyboard is the
	// container's, and so is which one a press landed in — in that pane's own
	// coordinates, which is the part nobody should be writing by hand.
	b.body = headless.Columns(
		headless.Item{Size: layout.Part(2, 5), Of: b.dressed},
		headless.Item{Size: layout.Flex(1), Of: b.window},
	)
	b.body.Gap = 1
	b.body.Focus(true)
	return b
}

// Draw paints the two panes, and a hint row under them.
func (b *browser) Draw(v grid.View) {
	b.show()
	rows := v.Subs(layout.Down.Rects(v.Bounds().Size(),
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(1)},
	))
	b.body.Draw(rows[0])

	kit.Label{
		Text:  "tab: other pane   →/←: open, close   q: quit",
		Style: b.theme.Subtle,
	}.Draw(rows[1])
}

// show keeps the preview in step with what the cursor is on, once per change.
func (b *browser) show() {
	row, ok := b.tree.CurrentRow()
	if !ok || row.Item.path == b.showing {
		return
	}
	b.showing = row.Item.path
	b.preview.SetText(preview(row.Item, b.theme))
	b.window.Scroll().ToTop()
}

func (b *browser) Handle(ev input.Event) bool {
	if key, ok := ev.(input.Key); ok && key.Down() && key.Rune == 'q' {
		b.runtime.Quit()
		return true
	}
	return b.body.Handle(ev)
}

// preview is what to show beside the tree: the first part of a file, or what a
// directory holds.
func preview(of entry, theme kit.Theme) []text.Line {
	if of.dir {
		return []text.Line{text.Of(of.path+" is a directory", theme.Muted)}
	}
	// A real browser would read as much as the window can show. Reading a fixed
	// amount is the same idea with the size decided here rather than there.
	body, err := os.ReadFile(of.path)
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

// read walks a directory into the nodes a tree is made of, to a depth.
func read(root string, depth int) []headless.Node[entry] {
	if depth < 0 {
		return nil
	}
	items, err := os.ReadDir(root)
	if err != nil {
		return nil
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
