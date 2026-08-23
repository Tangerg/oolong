// Package visualtest captures the screen produced by an example program.
//
// It composes programtest's in-process host with ptytest's renderer-sized screen
// model. The first drives the real program loop and captures its frame bytes; the
// second answers what those bytes leave in terminal cells. Keeping that composition
// here gives examples one visual assertion path instead of a private screen model in
// every command.
package visualtest

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/programtest"
	"github.com/Tangerg/oolong/ptytest"
)

var update = flag.Bool("update", false, "update visual golden files")

// Host is a programtest host that also reports the terminal colours an example is
// being checked against. No other optional capability is invented.
type Host struct {
	*programtest.Host
	ground grid.Ground
}

// Config is the complete initial state of a visual-test [Host]. Width and Height
// must be positive terminal-cell dimensions. The zero Ground models an unknown
// terminal background.
type Config struct {
	Width, Height int
	Ground        grid.Ground
}

// New returns a visual host at size with the given terminal colours.
func New(tb testing.TB, config Config) *Host {
	tb.Helper()
	return &Host{
		Host:   programtest.New(tb, programtest.Config{Width: config.Width, Height: config.Height}),
		ground: config.Ground,
	}
}

// Ground reports the terminal colours used to build the example's theme.
func (h *Host) Ground() grid.Ground {
	if h == nil {
		return grid.Ground{}
	}
	return h.ground
}

// Capture is one completely repainted program screen and the renderer bytes that
// produced that repaint.
type Capture struct {
	Rows     []string
	Encoding string
}

// Capture asks for a full repaint, waits for it to reach the host, and interprets
// every frame written so far as one terminal screen.
func (h *Host) Capture(tb testing.TB) Capture {
	tb.Helper()
	if h == nil || h.Host == nil {
		tb.Fatal("visualtest: capture from nil host")
	}
	before := len(h.Frames())
	if !h.Repaint() {
		tb.Fatal("visualtest: host refused repaint")
	}
	h.Until(tb, "a complete visual-test repaint", func() bool {
		return len(h.Frames()) > before
	})

	width, height, err := h.Size()
	if err != nil {
		tb.Fatalf("visualtest: read host size: %v", err)
	}
	screen, err := ptytest.NewScreen(ptytest.Size{Cols: width, Rows: height})
	if err != nil {
		tb.Fatalf("visualtest: create screen: %v", err)
	}
	if err := screen.Apply([]byte(h.Frames())); err != nil {
		tb.Fatalf("visualtest: interpret frames: %v", err)
	}
	if err := screen.Flush(); err != nil {
		tb.Fatalf("visualtest: finish screen: %v", err)
	}
	return Capture{Rows: screen.Rows(), Encoding: h.Frame()}
}

// Match compares the visible part of rows with the golden file at path. Running the
// example module's tests with -update replaces the file with the captured result.
//
// Trailing spaces and blank rows carry no visible information and are removed.
// Spaces inside a row and empty rows between content remain significant.
func Match(tb testing.TB, path string, rows []string) {
	tb.Helper()
	if path == "" {
		tb.Fatal("visualtest: empty golden path")
	}
	got := format(rows)
	if *update {
		// The path is a source-controlled test fixture chosen by the test author, not
		// input from the program being tested; it deliberately uses ordinary source
		// file permissions.
		if err := os.WriteFile(path, got, 0o644); err != nil { //nolint:gosec // Test-owned source fixture needs source permissions.
			tb.Fatalf("visualtest: update %s: %v", path, err)
		}
		tb.Logf("updated %s", path)
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // Golden path is test-owned source, not external input.
	if err != nil {
		tb.Fatalf("visualtest: read %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		tb.Fatalf("visual screen differs from %s\n\ngot:\n%s\nwant:\n%s",
			path, got, want)
	}
}

func format(rows []string) []byte {
	rows = append([]string(nil), rows...)
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	for len(rows) > 0 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	if len(rows) == 0 {
		return nil
	}
	return []byte(strings.Join(rows, "\n") + "\n")
}
