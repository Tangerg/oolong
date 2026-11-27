package ptytest

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// A resize starts the output for the new size; what came before was laid out for
// another size and is no longer part of the screen.
func TestScreenReadsTheOutputForTheCurrentSize(t *testing.T) {
	tr := newTranscript(Size{Cols: 10, Rows: 1})
	tr.append([]byte("old frame"))
	if err := tr.resize(Size{Cols: 4, Rows: 2}, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	tr.append([]byte("new"))
	screen, err := tr.Screen()
	if err != nil {
		t.Fatal(err)
	}
	if size := screen.Size(); size != (Size{Cols: 4, Rows: 2}) {
		t.Fatalf("screen size = %+v, want the resized geometry", size)
	}
	if got := strings.TrimRight(screen.Rows()[0], " "); got != "new" {
		t.Fatalf("screen shows %q, want only the output since the resize", got)
	}
}

func TestScreenUsesOnlyCompleteFramesAcrossResize(t *testing.T) {
	const begin, end = "\x1b[?2026h", "\x1b[?2026l"
	const old = begin + "\x1b[1;1H┌old size" + end
	const next = begin + "\x1b[1;1Hnew geometry"
	for name, split := range map[string]int{
		"opening sequence": len(begin) - 2,
		"border character": strings.Index(old, "┌") + 1,
	} {
		t.Run(name, func(t *testing.T) {
			tr := newTranscript(Size{Cols: 10, Rows: 1})
			tr.append([]byte(old[:split]))
			if err := tr.resize(Size{Cols: 16, Rows: 2}, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			tr.append([]byte(old[split:] + next))

			screen, err := tr.Screen()
			if err != nil {
				t.Fatal(err)
			}
			if shown := strings.TrimSpace(strings.Join(screen.Rows(), "\n")); shown != "" {
				t.Fatalf("screen shows %q before the replacement frame completed", shown)
			}
			tr.append([]byte(end))
			tr.finish(nil)
			screen, err = tr.WaitForScreen(t.Context(), "new geometry")
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimRight(screen.Rows()[0], " "); got != "new geometry" {
				t.Fatalf("screen shows %q, want the completed replacement frame", got)
			}
		})
	}
}

// A token split by styling or arriving in pieces is still one token on the screen,
// and a frame cut mid-sequence is waited past rather than reported.
func TestWaitForScreenSeesTextTheRawOutputSplits(t *testing.T) {
	tr := newTranscript(Size{Cols: 20, Rows: 2})
	go func() {
		tr.append([]byte("Ask \x1b[1mfla"))
		time.Sleep(5 * time.Millisecond)
		tr.append([]byte("me\x1b["))
		time.Sleep(5 * time.Millisecond)
		tr.append([]byte("0m ready"))
	}()
	screen, err := tr.WaitForScreen(t.Context(), "Ask flame", "ready")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(screen.Rows()[0], "Ask flame ready") {
		t.Fatalf("returned screen = %q", screen.Rows())
	}
}

func TestWaitForScreenReportsWhatTheScreenLastShowed(t *testing.T) {
	tr := newTranscript(Size{Cols: 12, Rows: 1})
	tr.append([]byte("\x1b[1mvisible"))
	tr.finish(nil)
	_, err := tr.WaitForScreen(t.Context(), "visible", "absent")
	if !errors.Is(err, io.EOF) {
		t.Fatalf("a clean end returned %v, want EOF", err)
	}
	message := err.Error()
	if !strings.Contains(message, `"absent"`) || strings.Contains(message, `"visible",`) ||
		!strings.Contains(message, "the screen showed") || strings.Contains(message, `\x1b`) {
		t.Fatalf("diagnostic = %q", message)
	}
}
