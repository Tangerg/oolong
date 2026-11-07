package main

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"slices"

	"github.com/Tangerg/oolong/ui/core/grid"
)

type frameStyle struct {
	Foreground *[3]uint8 `json:"foreground"`
	Background *[3]uint8 `json:"background"`
	Bold       bool      `json:"bold"`
	Dim        bool      `json:"dim"`
	Italic     bool      `json:"italic"`
	Underline  bool      `json:"underline"`
	Reverse    bool      `json:"reverse"`
	Strike     bool      `json:"strike"`
}

func (style frameStyle) gridStyle() grid.Style {
	var out grid.Style
	if rgb := style.Foreground; rgb != nil {
		out.FG = grid.RGBColor(rgb[0], rgb[1], rgb[2])
	}
	if rgb := style.Background; rgb != nil {
		out.BG = grid.RGBColor(rgb[0], rgb[1], rgb[2])
	}
	for _, attribute := range []struct {
		set   bool
		value grid.Attr
	}{
		{style.Bold, grid.Bold},
		{style.Dim, grid.Dim},
		{style.Italic, grid.Italic},
		{style.Underline, grid.Underline},
		{style.Reverse, grid.Reverse},
		{style.Strike, grid.Strike},
	} {
		if attribute.set {
			out.Attr |= attribute.value
		}
	}
	return out
}

type frameAtom struct {
	Text    string     `json:"text"`
	Columns int        `json:"columns"`
	Style   frameStyle `json:"style"`
	Link    string     `json:"link"`
}

type frameCursor struct {
	Visible bool   `json:"visible"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Shape   string `json:"shape"`
	Blink   bool   `json:"blink"`
}

func (cursor frameCursor) place(view grid.View) {
	if !cursor.Visible {
		return
	}
	var shape grid.CursorShape
	switch cursor.Shape {
	case "block":
		shape = grid.CursorBlock
	case "underline":
		shape = grid.CursorUnderline
	case "bar":
		shape = grid.CursorBar
	default:
		panic("fixture: unspecified cursor shape")
	}
	view.PlaceCursor(cursor.X, cursor.Y, grid.CursorStyle{Shape: shape, Blink: cursor.Blink})
}

type screenFrame struct {
	Name      string        `json:"name"`
	Size      dimensions    `json:"size"`
	Rows      [][]frameAtom `json:"rows"`
	Cursor    frameCursor   `json:"cursor"`
	Diff      string        `json:"diff"`
	Full      string        `json:"full"`
	Unchanged bool          `json:"unchanged"`
	Scroll    bool          `json:"scroll"`
}

type screenSequence struct {
	Name   string        `json:"name"`
	Frames []screenFrame `json:"frames"`
}

func screenSequences() []screenSequence {
	sequences := []screenSequence{screenTransitions()}
	const seeds, frames = 16, 64
	for seed := range uint64(seeds) {
		random := rand.New(rand.NewPCG(seed, seed+91))
		sequence := screenSequence{Name: fmt.Sprintf("screen sequence seed %d", seed)}
		frame := screenFrame{Size: dimensions{12, 6}}
		frame.Rows = randomRows(random, frame.Size)
		for index := range frames {
			frame.Name = fmt.Sprintf("frame %d", index)
			frame.Unchanged = false
			switch random.IntN(8) {
			case 0:
				frame.Rows[random.IntN(frame.Size.Rows)] = randomRows(random, dimensions{frame.Size.Columns, 1})[0]
			case 1:
				frame.Rows = append(frame.Rows[1:], randomRows(random, dimensions{frame.Size.Columns, 1})[0])
			case 2:
				frame.Rows = append(randomRows(random, dimensions{frame.Size.Columns, 1}), frame.Rows[:frame.Size.Rows-1]...)
			case 3:
				row := frame.Rows[random.IntN(frame.Size.Rows)]
				atom := &row[random.IntN(len(row))]
				atom.Style, atom.Link = randomStyle(random), randomLink(random)
			case 4:
				frame.Size = dimensions{2 + random.IntN(23), 2 + random.IntN(7)}
				frame.Rows = randomRows(random, frame.Size)
			case 5:
				frame.Rows = blankRows(frame.Size)
			case 6:
				frame.Unchanged = index > 0
			}
			if !frame.Unchanged {
				frame.Cursor = frameCursor{
					Visible: random.IntN(3) != 0,
					X:       random.IntN(frame.Size.Columns), Y: random.IntN(frame.Size.Rows),
					Shape: []string{"block", "underline", "bar"}[random.IntN(3)], Blink: random.IntN(2) == 0,
				}
			}
			sequence.add(frame)
		}
		sequences = append(sequences, sequence)
	}
	for index := range sequences {
		sequences[index].encode()
	}
	return sequences
}

func screenTransitions() screenSequence {
	sequence := screenSequence{Name: "screen style, links, wide cells, cursor, resize, and scroll"}
	frame := screenFrame{
		Name: "styled atoms at both margins", Size: dimensions{8, 4},
		Cursor: frameCursor{Visible: true, X: 3, Y: 2, Shape: "bar"},
	}
	frame.Rows = blankRows(frame.Size)
	frame.Rows[0] = []frameAtom{
		{Text: "A", Columns: 1, Style: frameStyle{Bold: true, Foreground: &[3]uint8{18, 52, 86}, Background: &[3]uint8{160, 32, 64}}, Link: "https://example.test/first"},
		{Text: "界", Columns: 2, Style: frameStyle{Underline: true}, Link: "https://example.test/wide"},
		{Text: "e\u0301", Columns: 1, Style: frameStyle{Italic: true, Strike: true}},
		{Text: "x", Columns: 1, Style: frameStyle{Dim: true, Reverse: true}},
		{Text: " ", Columns: 1},
		{Text: "界", Columns: 2},
	}
	sequence.add(frame)
	frame.Name, frame.Unchanged = "identical accepted frame", true
	sequence.add(frame)
	frame.Name, frame.Unchanged = "only style and link identity change", false
	frame.Rows[0][0].Style = frameStyle{Dim: true, Strike: true}
	frame.Rows[0][0].Link = "https://example.test/second"
	sequence.add(frame)
	frame.Name = "style and link return to defaults"
	frame.Rows[0][0].Style, frame.Rows[0][0].Link = frameStyle{}, ""
	sequence.add(frame)
	frame.Name, frame.Rows = "narrow cells replace wide atoms and linked tails", blankRows(frame.Size)
	frame.Rows[0][1].Text, frame.Rows[0][2].Text = "a", "b"
	frame.Rows[0][7].Style.Background = &[3]uint8{15, 45, 90}
	sequence.add(frame)
	frame.Name, frame.Cursor.Visible = "cursor disappears without cell changes", false
	sequence.add(frame)
	frame.Name = "cursor returns with a different shape"
	frame.Cursor.Visible, frame.Cursor.Shape, frame.Cursor.Blink = true, "underline", true
	sequence.add(frame)
	for _, size := range []dimensions{{3, 2}, {10, 6}, {20, 8}} {
		frame.Name, frame.Size = fmt.Sprintf("resize to %dx%d", size.Columns, size.Rows), size
		frame.Rows = blankRows(size)
		frame.Rows[0] = append([]frameAtom{{Text: "界", Columns: 2}}, frame.Rows[0][2:]...)
		frame.Cursor.X, frame.Cursor.Y = size.Columns-1, size.Rows-1
		sequence.add(frame)
	}
	frame.Rows = blankRows(frame.Size)
	for row := range frame.Rows {
		for column := range frame.Rows[row] {
			frame.Rows[row][column] = frameAtom{
				Text: string(rune('A' + row)), Columns: 1,
				Style: frameStyle{Bold: row%2 == 0}, Link: fmt.Sprintf("https://example.test/row/%d", row),
			}
		}
	}
	frame.Name = "rows before scrolling"
	sequence.add(frame)
	frame.Name, frame.Scroll = "scroll all rows up", true
	frame.Rows = append(frame.Rows[1:], blankRows(dimensions{frame.Size.Columns, 1})[0])
	sequence.add(frame)
	frame.Name = "scroll all rows down"
	frame.Rows = append(blankRows(dimensions{frame.Size.Columns, 1}), frame.Rows[:frame.Size.Rows-1]...)
	sequence.add(frame)
	frame.Name = "scroll a region between stable header and footer"
	copy(frame.Rows[1:len(frame.Rows)-2], frame.Rows[2:len(frame.Rows)-1])
	frame.Rows[len(frame.Rows)-2] = blankRows(dimensions{frame.Size.Columns, 1})[0]
	sequence.add(frame)
	frame.Name, frame.Scroll, frame.Rows = "clear after scrolling", false, blankRows(frame.Size)
	sequence.add(frame)
	return sequence
}

func (sequence *screenSequence) add(frame screenFrame) {
	frame.Rows = slices.Clone(frame.Rows)
	for index := range frame.Rows {
		frame.Rows[index] = slices.Clone(frame.Rows[index])
	}
	sequence.Frames = append(sequence.Frames, frame)
}

func (sequence *screenSequence) encode() {
	diff, full := grid.NewScreen(0, 0), grid.NewScreen(0, 0)
	for index := range sequence.Frames {
		frame := &sequence.Frames[index]
		frame.Diff = frame.render(diff)
		full.Invalidate()
		frame.Full = frame.render(full)
	}
}

func (frame screenFrame) render(canvas *grid.Screen) string {
	canvas.Resize(frame.Size.Columns, frame.Size.Rows)
	view := canvas.Frame()
	for y, row := range frame.Rows {
		x := 0
		for _, atom := range row {
			view.Text(x, y, atom.Text, atom.Style.gridStyle())
			view.Link(x, y, atom.Columns, atom.Link)
			x += atom.Columns
		}
	}
	frame.Cursor.place(view)
	var output bytes.Buffer
	check(canvas.Flush(&output))
	return output.String()
}

func blankRows(size dimensions) [][]frameAtom {
	rows := make([][]frameAtom, size.Rows)
	for y := range rows {
		rows[y] = make([]frameAtom, size.Columns)
		for x := range rows[y] {
			rows[y][x] = frameAtom{Text: " ", Columns: 1}
		}
	}
	return rows
}

func randomRows(random *rand.Rand, size dimensions) [][]frameAtom {
	// These literal widths are a bounded Unicode oracle, independent of grid's
	// segmentation and width estimator. Emoji presentation is terminal-dependent.
	atoms := []frameAtom{
		{Text: "a", Columns: 1},
		{Text: "Z", Columns: 1},
		{Text: " ", Columns: 1},
		{Text: "界", Columns: 2},
		{Text: "e\u0301", Columns: 1},
		{Text: "λ", Columns: 1},
	}
	rows := make([][]frameAtom, size.Rows)
	for y := range rows {
		for remaining := size.Columns; remaining > 0; {
			atom := atoms[random.IntN(len(atoms))]
			if atom.Columns > remaining {
				atom = frameAtom{Text: " ", Columns: 1}
			}
			atom.Style, atom.Link = randomStyle(random), randomLink(random)
			rows[y] = append(rows[y], atom)
			remaining -= atom.Columns
		}
	}
	return rows
}

func randomStyle(random *rand.Rand) frameStyle {
	style := frameStyle{
		Bold: random.IntN(2) == 0, Dim: random.IntN(3) == 0, Italic: random.IntN(2) == 0,
		Underline: random.IntN(2) == 0, Reverse: random.IntN(3) == 0, Strike: random.IntN(2) == 0,
	}
	if random.IntN(2) == 0 {
		style.Foreground = &[3]uint8{18, 52, 86}
	}
	if random.IntN(2) == 0 {
		style.Background = &[3]uint8{160, 32, 64}
	}
	return style
}

func randomLink(random *rand.Rand) string {
	return []string{"", "https://example.test/first", "https://example.test/second"}[random.IntN(3)]
}
