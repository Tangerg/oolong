// Package visualtest composes the real program loop with one terminal screen
// model so every example asserts the same observable rendering contract.
package visualtest

import (
	"bytes"
	"context"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/uitest"
	"github.com/Tangerg/oolong/ui/uitest/ptytest"
)

var update = flag.Bool("update", false, "update visual golden files")

// Host adds only terminal ground because inventing unrelated capabilities would make
// visual tests exercise a different host contract.
type Host struct {
	*uitest.Host
	ground grid.Ground
}

// Config requires positive terminal-cell dimensions to keep captures meaningful. An unspecified
// ground preserves the application's unknown-background path.
type Config struct {
	Width, Height int
	Ground        grid.Ground
}

func New(tb testing.TB, config Config) *Host {
	tb.Helper()
	return &Host{
		Host:   uitest.New(tb, uitest.Config{Width: config.Width, Height: config.Height}),
		ground: config.Ground,
	}
}

func (h *Host) Ground() grid.Ground {
	if h == nil {
		return grid.Ground{}
	}
	return h.ground
}

// Capture retains rows and encoded bytes from the same completed repaint so visual and
// protocol assertions describe one screen.
type Capture struct {
	Rows     []string
	Encoding string
}

// Capture interprets a full repaint with all preceding frames so retained
// terminal state cannot be mistaken for newly drawn output.
func (h *Host) Capture(tb testing.TB) Capture {
	tb.Helper()
	if h == nil || h.Host == nil {
		tb.Fatal("visualtest: capture from nil host")
	}
	ctx, cancel := context.WithTimeout(tb.Context(), 5*time.Second)
	defer cancel()
	snapshot, err := h.Repaint(ctx)
	if err != nil {
		tb.Fatalf("visualtest: repaint: %v", err)
	}
	screen, err := ptytest.NewScreen(ptytest.Size{Cols: snapshot.Width, Rows: snapshot.Height})
	if err != nil {
		tb.Fatalf("visualtest: create screen: %v", err)
	}
	if err := screen.Apply([]byte(snapshot.Frames)); err != nil {
		tb.Fatalf("visualtest: interpret frames: %v", err)
	}
	if err := screen.Flush(); err != nil {
		tb.Fatalf("visualtest: finish screen: %v", err)
	}
	return Capture{Rows: screen.Rows(), Encoding: snapshot.Frame}
}

// Match ignores trailing spaces and blank rows because they carry no visible information; internal spaces
// and blank rows remain significant. -update replaces only the chosen test fixture.
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
