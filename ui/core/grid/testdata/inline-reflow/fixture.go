// Command fixture supplies the compositor's actual bytes to the independent
// terminal oracle so no test-owned row encoder can conceal reflow mistakes.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"log"
	"os"

	"github.com/Tangerg/oolong/ui/core/grid"
)

type dimensions struct {
	Columns int `json:"columns"`
	Rows    int `json:"rows"`
}

type display struct {
	History []string    `json:"history"`
	Live    []string    `json:"live"`
	Cursor  image.Point `json:"cursor"`
	Hard    int         `json:"hard"`
}

type step struct {
	Name      string      `json:"name"`
	Resize    *dimensions `json:"resize,omitempty"`
	Output    string      `json:"output,omitempty"`
	Want      *display    `json:"want,omitempty"`
	Unchanged bool        `json:"unchanged,omitempty"`
}

type scenario struct {
	Name    string     `json:"name"`
	Size    dimensions `json:"size"`
	History []string   `json:"history"`
	Steps   []step     `json:"steps"`
}

func main() {
	scenarios := []scenario{
		resizeScenario("hidden cursor", -1),
		resizeScenario("cursor on first row", 0),
		resizeScenario("cursor on middle row", 1),
		resizeScenario("cursor on last row", 2),
		wideScenario(),
		publicationScenario(),
		clippedHeightScenario(),
	}
	check(json.NewEncoder(os.Stdout).Encode(struct {
		Inline []scenario       `json:"inline"`
		Screen []screenSequence `json:"screen"`
	}{scenarios, screenSequences()}))
}

func resizeScenario(name string, cursorRow int) scenario {
	test := scenario{
		Name: name,
		Size: dimensions{10, 6},
		History: []string{
			"history with a long logical line",
			"a second history line",
			"history end",
		},
	}
	canvas := grid.NewInline(test.Size.Columns, test.Size.Rows)
	add := func(name string, rows []string, unchanged bool) {
		cursor := grid.Cursor{}
		if cursorRow >= 0 {
			cursor = grid.Cursor{Visible: true, Pos: image.Pt(1, min(cursorRow, len(rows)-1))}
		}
		wire := paint(canvas, rows, cursor, nil)
		test.Steps = append(test.Steps, step{
			Name: name, Output: wire, Unchanged: unchanged,
			Want: expected(test.History, rows, cursor),
		})
	}
	initial := []string{"abcdefghij", "old row", "tail"}
	changed := []string{initial[0], "old", initial[2]}
	add("first frame", initial, false)
	add("diff clears a tail without splitting the group", changed, false)
	add("unchanged frame", changed, true)
	resize(&test, canvas, dimensions{5, 6})
	narrow := []string{"NEW", "go", "end"}
	add("repaint after shrink", narrow, false)
	add("shorter live block", narrow[:2], false)
	add("grow the live block", []string{narrow[0], narrow[1], "back"}, false)
	resize(&test, canvas, dimensions{12, 6})
	add("repaint after widening", []string{"wide first", "wide second", "wide third"}, false)
	resize(&test, canvas, dimensions{12, 3})
	last := []string{"first", "second", "third"}
	add("height reduced to the live footprint", last, false)
	resize(&test, canvas, dimensions{7, 9})
	add("width and height changed together", last, false)
	finish(&test, canvas, test.History, last)
	return test
}

func wideScenario() scenario {
	test := scenario{
		Name: "wide cells, styling, and margin wrapping",
		Size: dimensions{5, 4}, History: []string{"界界 history", "keep history"},
	}
	canvas := grid.NewInline(test.Size.Columns, test.Size.Rows)
	add := func(name string, rows []string) {
		wire := paint(canvas, rows, grid.Cursor{}, func(view grid.View) {
			view.Text(0, 1, rows[1], grid.Style{Attr: grid.Bold})
			width, _ := canvas.Size()
			view.Link(0, 1, width, "https://example.test/wide")
		})
		test.Steps = append(test.Steps, step{
			Name: name, Output: wire, Want: expected(test.History, rows, grid.Cursor{}),
		})
	}
	add("wide atom ends at right margin", []string{"abc界", "界x"})
	resize(&test, canvas, dimensions{2, 4})
	add("narrowest xterm viewport", []string{"界", "x"})
	add("blank tail diff", []string{"a", "x"})
	resize(&test, canvas, dimensions{6, 4})
	add("wide margin after widening", []string{"abcd界", "界x"})
	finish(&test, canvas, test.History, []string{"abcd界", "界x"})
	return test
}

func publicationScenario() scenario {
	test := scenario{
		Name: "publication abort and composition failure",
		Size: dimensions{10, 6}, History: []string{"history unchanged"},
	}
	canvas := grid.NewInline(test.Size.Columns, test.Size.Rows)
	initial := []string{"old top", "old bottom"}
	cursor := grid.Cursor{Visible: true, Pos: image.Pt(1, 0)}
	test.Steps = append(test.Steps, step{
		Name: "initial accepted frame", Output: paint(canvas, initial, cursor, nil),
		Want: expected(test.History, initial, cursor),
	})
	contribution, err := grid.Capture(grid.CaptureConfig{Width: 10, Content: rowsDrawable{"published", "answer"}})
	check(err)
	resize(&test, canvas, dimensions{5, 6})
	replacement := []string{"new", "live"}
	draw(canvas, replacement, cursor, nil)
	candidate, err := canvas.Prepare(contribution)
	check(err)
	if candidate.Len() == 0 {
		log.Fatal("aborted publication produced no candidate")
	}
	candidate.Abort()
	test.Steps = append(test.Steps, step{Name: "aborted publication writes nothing", Unchanged: true})
	draw(canvas, replacement, cursor, func(view grid.View) {
		view.Paint(grid.Area(0, 0, 1, 1), 1, failedPainter{})
	})
	candidate, err = canvas.Prepare(contribution)
	if candidate != nil || !errors.Is(err, errPaint) {
		log.Fatalf("failed preparation: candidate %v, error %v", candidate, err)
	}
	test.Steps = append(test.Steps, step{Name: "failed preparation writes nothing", Unchanged: true})
	draw(canvas, replacement, cursor, nil)
	var output bytes.Buffer
	check(canvas.Flush(&output, contribution))
	history := append(append([]string{}, test.History...), "publi", "shed", "answe", "r")
	test.Steps = append(test.Steps, step{
		Name: "retry publishes once from accepted anchor", Output: output.String(),
		Want: expected(history, replacement, cursor),
	})
	test.Steps = append(test.Steps, step{
		Name: "accepted contribution is not repeated", Unchanged: true,
		Output: paint(canvas, replacement, cursor, nil),
		Want:   expected(history, replacement, cursor),
	})
	finish(&test, canvas, history, replacement)
	return test
}

func clippedHeightScenario() scenario {
	test := scenario{
		Name: "height below live footprint preserves inaccessible history",
		Size: dimensions{10, 6}, History: []string{"history"},
	}
	canvas := grid.NewInline(test.Size.Columns, test.Size.Rows)
	rows := []string{"old0", "old1", "old2", "old3"}
	test.Steps = append(test.Steps, step{
		Name: "live block before height shrink", Output: paint(canvas, rows, grid.Cursor{}, nil),
		Want: expected(test.History, rows, grid.Cursor{}),
	})
	resize(&test, canvas, dimensions{5, 2})
	// The terminal has already put this prefix in scrollback. Only its visible
	// suffix remains addressable; clearing scrollback would also destroy history.
	history := []string{"history", "old0 old1"}
	replacement := []string{"NEW"}
	test.Steps = append(test.Steps, step{
		Name:   "replace visible suffix without scrolling replacement away",
		Output: paint(canvas, replacement, grid.Cursor{}, nil),
		Want:   expected(history, replacement, grid.Cursor{}),
	})
	resize(&test, canvas, dimensions{10, 6})
	wider := []string{"new wider"}
	test.Steps = append(test.Steps, step{
		Name:   "recovered room keeps terminal-owned history intact",
		Output: paint(canvas, wider, grid.Cursor{}, nil),
		Want:   expected(history, wider, grid.Cursor{}),
	})
	finish(&test, canvas, history, wider)
	return test
}

func resize(test *scenario, canvas *grid.Inline, size dimensions) {
	canvas.Resize(size.Columns, size.Rows)
	test.Steps = append(test.Steps, step{Name: "physical terminal resize", Resize: &size})
}

func draw(canvas *grid.Inline, rows []string, cursor grid.Cursor, extra func(grid.View)) {
	view := canvas.Frame()
	for y, row := range rows {
		view.Text(0, y, row, grid.Style{})
	}
	if cursor.Visible {
		view.PlaceCursor(cursor.Pos.X, cursor.Pos.Y, cursor.Style)
	}
	if extra != nil {
		extra(view)
	}
}

func paint(canvas *grid.Inline, rows []string, cursor grid.Cursor, extra func(grid.View)) string {
	draw(canvas, rows, cursor, extra)
	var output bytes.Buffer
	check(canvas.Flush(&output))
	return output.String()
}

func expected(history, live []string, cursor grid.Cursor) *display {
	at := image.Pt(0, len(live)-1)
	if cursor.Visible {
		at = cursor.Pos
	}
	return &display{History: history, Live: live, Cursor: at}
}

func finish(test *scenario, canvas *grid.Inline, history, rows []string) {
	candidate, err := canvas.PrepareFinish()
	check(err)
	candidate.Abort()
	test.Steps = append(test.Steps, step{Name: "aborted finish writes nothing", Unchanged: true})
	var output bytes.Buffer
	check(canvas.Finish(&output))
	history = append(append([]string{}, history...), rows...)
	test.Steps = append(test.Steps, step{
		Name: "finish leaves independent history rows", Output: output.String(),
		Want: &display{History: history, Hard: len(rows)},
	})
	output.Reset()
	check(canvas.Finish(&output))
	test.Steps = append(test.Steps, step{Name: "finish is not repeated", Output: output.String(), Unchanged: true})
}

type rowsDrawable []string

func (rows rowsDrawable) HeightForWidth(int) int { return len(rows) }

func (rows rowsDrawable) Draw(view grid.View) {
	for y, row := range rows {
		view.Text(0, y, row, grid.Style{})
	}
}

var errPaint = errors.New("fixture paint failure")

type failedPainter struct{}

func (failedPainter) Paint(grid.PaintContext) error { return errPaint }
func (failedPainter) Erase(grid.PaintContext) error { return nil }

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
