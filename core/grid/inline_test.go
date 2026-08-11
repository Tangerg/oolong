package grid_test

import (
	"bytes"
	"errors"
	"image"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
)

type drawable struct {
	rows int
	draw func(grid.View)
}

func (d drawable) Measure(int) int { return d.rows }

func (d drawable) Draw(view grid.View) {
	if d.draw != nil {
		d.draw(view)
	}
}

func content(rows int, draw func(grid.View)) grid.Drawable {
	return drawable{rows: rows, draw: draw}
}

// inline renders one inline frame and returns the bytes, without the frame markers.
func inline(t *testing.T, i *grid.Inline, cursor grid.Cursor, draw func(grid.View)) string {
	t.Helper()
	v := i.Frame()
	if cursor.Visible {
		v.PlaceCursor(cursor.Pos.X, cursor.Pos.Y, cursor.Style)
	}
	if draw != nil {
		draw(v)
	}
	var buf bytes.Buffer
	if err := i.Flush(&buf); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	out := buf.String()
	if out == "" {
		return ""
	}
	if !strings.HasPrefix(out, "\x1b[?2026h") || !strings.HasSuffix(out, "\x1b[?2026l") {
		t.Fatalf("frame is not wrapped for atomic application: %q", out)
	}
	return strings.TrimSuffix(strings.TrimPrefix(out, "\x1b[?2026h"), "\x1b[?2026l")
}

// lines draws one string per row.
func lines(rows ...string) func(grid.View) {
	return func(v grid.View) {
		for y, row := range rows {
			v.Text(0, y, row, grid.Style{})
		}
	}
}

func TestAnInlineBlockIsAsTallAsWhatWasDrawn(t *testing.T) {
	// Nothing declares a height. An interface that draws two rows occupies two rows,
	// and the eight it was offered stay the terminal's.
	i := grid.NewInline(10, 10)
	got := inline(t, i, grid.Cursor{}, lines("ab", "cd"))
	want := "\x1b[0m" + "\r" + "ab" + "\x1b[K" + "\r\n" + "cd" + "\x1b[K" + "\r" + "\x1b[?25l"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestAnInlineFrameNeverAddressesTheTerminalAbsolutely(t *testing.T) {
	// The whole model in one assertion: the block's position is decided by whatever is
	// above it, which this type does not own and cannot ask about. A single absolute
	// move would be right until the first time something else scrolled the terminal.
	i := grid.NewInline(10, 6)
	var all strings.Builder
	all.WriteString(inline(t, i, grid.Cursor{}, lines("one")))
	all.WriteString(inline(t, i, grid.Cursor{}, lines("one", "two", "three")))
	i.Print(content(1, func(v grid.View) { v.Text(0, 0, "done", grid.Style{}) }))
	all.WriteString(inline(t, i, grid.Cursor{Visible: true}, lines("one", "two")))
	all.WriteString(inline(t, i, grid.Cursor{}, lines("one")))
	all.WriteString(inline(t, i, grid.Cursor{}, nil))

	forbidden := map[byte]string{
		'H': "absolute cursor position",
		'f': "absolute cursor position",
		'J': "erase display",
		'r': "a scrolling region",
		'S': "scroll up",
		'T': "scroll down",
	}
	for _, final := range csiFinals(all.String()) {
		if what, bad := forbidden[final]; bad {
			t.Errorf("an inline frame used %s (CSI %c), which assumes it knows where it is",
				what, final)
		}
	}
}

// csiFinals is the final byte of every CSI sequence in s, which is what says what the
// sequence did.
func csiFinals(s string) []byte {
	var out []byte
	for i := 0; i+1 < len(s); i++ {
		if s[i] != 0x1b || s[i+1] != '[' {
			continue
		}
		j := i + 2
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3f {
			j++
		}
		if j < len(s) {
			out = append(out, s[j])
		}
		i = j
	}
	return out
}

func TestAnIdleInlineFrameIsSilent(t *testing.T) {
	// Same reason a screen's is: bytes for nothing, and every cursor command restarts
	// the terminal's blink timer.
	i := grid.NewInline(10, 4)
	inline(t, i, grid.Cursor{}, lines("ab"))
	if got := inline(t, i, grid.Cursor{}, lines("ab")); got != "" {
		t.Fatalf("an unchanged frame wrote %q", got)
	}
}

func TestOnlyTheRowsThatChangedAreRewritten(t *testing.T) {
	i := grid.NewInline(10, 4)
	inline(t, i, grid.Cursor{}, lines("keep", "change"))

	got := inline(t, i, grid.Cursor{}, lines("keep", "changed"))
	want := "\x1b[0m" + "\r" + "\x1b[1A" + "\r\n" + "changed" + "\x1b[K" + "\r"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
	if strings.Contains(got, "keep") {
		t.Error("a row that did not change was rewritten")
	}
}

func TestEveryFrameStartsFromTheTopOfTheBlock(t *testing.T) {
	// The anchor is where the last frame left the cursor. Getting the count wrong by
	// one would draw the interface over the session's output, one row at a time.
	i := grid.NewInline(10, 6)
	inline(t, i, grid.Cursor{}, lines("a", "b", "c", "d"))
	got := inline(t, i, grid.Cursor{}, lines("a", "b", "c", "D"))
	if !strings.HasPrefix(got, "\x1b[0m"+"\r"+"\x1b[3A") {
		t.Fatalf("frame = %q, want it to climb the three rows back to the block's top", got)
	}
}

func TestABlockThatGotShorterErasesTheRowsItGaveUp(t *testing.T) {
	// Erased where they are rather than deleted: that leaves the block shorter with
	// blank rows below it and moves nothing that is above it.
	i := grid.NewInline(10, 4)
	inline(t, i, grid.Cursor{}, lines("ab", "cd"))

	got := inline(t, i, grid.Cursor{}, lines("ab"))
	want := "\x1b[0m" + "\r" + "\x1b[1A" + "\r\n" + "\x1b[K" + "\x1b[1A"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
	// And the anchor has to come back with it, or the next frame climbs too far.
	if got := inline(t, i, grid.Cursor{}, lines("AB")); !strings.HasPrefix(got, "\x1b[0m"+"\rAB") {
		t.Fatalf("the frame after a shrink = %q, want it to start at the block's one row", got)
	}
}

func TestABlockThatEmptiedGivesUpEveryRow(t *testing.T) {
	i := grid.NewInline(10, 4)
	inline(t, i, grid.Cursor{}, lines("ab", "cd"))
	got := inline(t, i, grid.Cursor{}, nil)
	want := "\x1b[0m" + "\r" + "\x1b[1A" + "\x1b[K" + "\r\n" + "\x1b[K" + "\x1b[1A"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestABlockIsTallEnoughToHoldTheCursor(t *testing.T) {
	// A caret on the row below the last thing drawn — an empty last line of an editor —
	// is still part of the interface. A block that stopped short of it would put the
	// cursor on a row it does not own.
	i := grid.NewInline(10, 6)
	got := inline(t, i, grid.Cursor{Visible: true, Pos: image.Pt(0, 2)}, lines("ab"))
	want := "\x1b[0m" + "\r" + "ab" + "\x1b[K" +
		"\r\n" + "\x1b[K" + "\r\n" + "\x1b[K" + "\x1b[0 q" + "\x1b[?25h"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestTheCursorIsPlacedRelativeToTheBlock(t *testing.T) {
	i := grid.NewInline(10, 6)
	got := inline(t, i, grid.Cursor{Visible: true, Pos: image.Pt(2, 1)}, lines("ab", "cdef"))
	if !strings.HasSuffix(got, "\r"+"\x1b[0 q"+"\x1b[2C"+"\x1b[?25h") {
		t.Fatalf("frame = %q, want the caret placed by moving across the block's last row", got)
	}
}

func TestAnUnmovedCursorIsNotRestated(t *testing.T) {
	// Every positioning command restarts the blink timer, so a caret that sits still
	// must be left alone or it never blinks.
	i := grid.NewInline(10, 6)
	at := grid.Cursor{Visible: true, Pos: image.Pt(2, 0)}
	inline(t, i, at, lines("abcd"))
	if got := inline(t, i, at, lines("abcd")); got != "" {
		t.Fatalf("a still caret wrote %q", got)
	}
	got := inline(t, i, grid.Cursor{Visible: true, Pos: image.Pt(3, 0)}, lines("abcd"))
	if got != "\x1b[0m"+"\r"+"\x1b[3C" {
		t.Fatalf("frame = %q, want just the move", got)
	}
}

func TestInlineCursorStyleIsDiffedWithoutRestartingAnIdleCursor(t *testing.T) {
	i := grid.NewInline(10, 2)
	bar := grid.Cursor{
		Visible: true, Pos: image.Pt(1, 0),
		Style: grid.CursorStyle{Shape: grid.CursorBar, Blink: true},
	}
	if first := inline(t, i, bar, lines("ab")); !strings.Contains(first, "\x1b[5 q") {
		t.Fatalf("first frame = %q, want a blinking bar", first)
	}
	if idle := inline(t, i, bar, lines("ab")); idle != "" {
		t.Fatalf("unchanged cursor wrote %q", idle)
	}
	bar.Style.Blink = false
	if changed := inline(t, i, bar, lines("ab")); changed != "\x1b[0m"+"\r"+"\x1b[6 q"+"\x1b[1C" {
		t.Fatalf("blink change = %q, want only style and re-anchor", changed)
	}
}

func TestWritingCellsReAnchorsTheCursor(t *testing.T) {
	// Writing a row leaves the terminal's cursor after the last glyph, so a caret that
	// has not moved still has to be stated again.
	i := grid.NewInline(10, 6)
	at := grid.Cursor{Visible: true, Pos: image.Pt(1, 0)}
	inline(t, i, at, lines("ab"))
	got := inline(t, i, at, lines("xy"))
	want := "\x1b[0m" + "\r" + "xy" + "\x1b[K" + "\r" + "\x1b[1C"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestPrintedRowsGoAboveTheBlock(t *testing.T) {
	// This is what "inline" is for: the interface says something final, and it becomes
	// the terminal's from then on.
	i := grid.NewInline(10, 6)
	inline(t, i, grid.Cursor{}, lines("prompt"))

	i.Print(content(1, func(v grid.View) { v.Text(0, 0, "done", grid.Style{}) }))
	got := inline(t, i, grid.Cursor{}, lines("prompt"))
	want := "\x1b[0m" + "\r" + "done" + "\x1b[K" + "\r\n" + "prompt" + "\x1b[K" + "\r"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestPrintedRowsAreWrittenOnce(t *testing.T) {
	// They belong to the terminal now. Writing them again on the next frame would
	// print the transcript twice over.
	i := grid.NewInline(10, 6)
	inline(t, i, grid.Cursor{}, lines("prompt"))
	i.Print(content(1, func(v grid.View) { v.Text(0, 0, "done", grid.Style{}) }))
	inline(t, i, grid.Cursor{}, lines("prompt"))

	if got := inline(t, i, grid.Cursor{}, lines("prompt")); got != "" {
		t.Fatalf("the frame after a print wrote %q", got)
	}
}

func TestPrintingRewritesTheBlockItPushedDown(t *testing.T) {
	// The printed rows land on the rows the block's first rows were on, and the block
	// moves down past them. Diffing against where it used to be would leave half of the
	// old interface on screen.
	i := grid.NewInline(10, 6)
	inline(t, i, grid.Cursor{}, lines("one", "two"))

	i.Print(content(1, func(v grid.View) { v.Text(0, 0, "said", grid.Style{}) }))
	got := inline(t, i, grid.Cursor{}, lines("one", "two"))
	if !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Fatalf("frame = %q, want the whole block rewritten below the printed row", got)
	}
}

func TestPrintingTakesTheRowsTheBlockNoLongerReaches(t *testing.T) {
	// A tall block that printed and then shrank leaves rows on screen below itself, and
	// they are as stale as any other row it gave up.
	i := grid.NewInline(10, 8)
	inline(t, i, grid.Cursor{}, lines("a", "b", "c"))

	i.Print(content(1, func(v grid.View) { v.Text(0, 0, "said", grid.Style{}) }))
	got := inline(t, i, grid.Cursor{}, lines("a"))
	want := "\x1b[0m" + "\r" + "\x1b[2A" +
		"said" + "\x1b[K" + "\r\n" +
		"a" + "\x1b[K" + "\r\n" + "\x1b[K" +
		"\x1b[1A"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestEveryPrintedRowIsPrinted(t *testing.T) {
	// Including the blank ones: a caller that laid its output out knows how tall it is,
	// and a blank row between two answers is content, not slack.
	i := grid.NewInline(10, 4)
	i.Print(content(3, func(v grid.View) {
		v.Text(0, 0, "first", grid.Style{})
		v.Text(0, 2, "third", grid.Style{})
	}))
	got := inline(t, i, grid.Cursor{}, nil)
	want := "\x1b[0m" + "\r" +
		"first" + "\x1b[K" + "\r\n" +
		"\x1b[K" + "\r\n" +
		"third" + "\x1b[K" + "\r\n" +
		"\x1b[?25l"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestOutputThatDidNotStopAtALineBoundaryCarriesOn(t *testing.T) {
	// Streaming output does not arrive on line boundaries. A reply delivered three
	// words at a time is one paragraph, and a printer that began every piece at column
	// zero would make it three rows tall.
	i := grid.NewInline(10, 6)
	i.Append(func(v grid.View) { v.Text(0, 0, "Hel", grid.Style{}) })
	i.Append(func(v grid.View) { v.Text(0, 0, "lo", grid.Style{}) })

	got := inline(t, i, grid.Cursor{}, lines("prompt"))
	want := "\x1b[0m" + "\r" +
		"Hel" + "\x1b[K" + "lo" + "\x1b[K" + "\r\n" +
		"prompt" + "\x1b[K" + "\r" + "\x1b[?25l"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
	if col, open := i.Tail(); col != 5 || !open {
		t.Fatalf("tail = %d (open=%v), want the row open five columns along", col, open)
	}
}

func TestCarryingOnReachesBackOverTheBlock(t *testing.T) {
	// The row being carried on was published with the block drawn underneath it, so
	// continuing it means going back up past the block and along to where it stopped.
	// Nothing about the block itself has changed, and nothing about it is rewritten.
	i := grid.NewInline(10, 6)
	i.Append(func(v grid.View) { v.Text(0, 0, "Hel", grid.Style{}) })
	inline(t, i, grid.Cursor{}, lines("prompt"))

	i.Append(func(v grid.View) { v.Text(0, 0, "lo", grid.Style{}) })
	got := inline(t, i, grid.Cursor{}, lines("prompt"))
	want := "\x1b[0m" + "\r" + "\x1b[1A" + "\x1b[3C" + "lo" + "\x1b[K" + "\r\n"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestAWholeRowWillNotShareOneWithAnythingElse(t *testing.T) {
	// Print is whole rows. What was left open is not part of them, and squeezing the
	// next block onto the end of it would put two unrelated things on one line.
	i := grid.NewInline(10, 6)
	i.Append(func(v grid.View) { v.Text(0, 0, "Hel", grid.Style{}) })
	i.Print(content(1, func(v grid.View) { v.Text(0, 0, "done", grid.Style{}) }))

	got := inline(t, i, grid.Cursor{}, nil)
	want := "\x1b[0m" + "\r" +
		"Hel" + "\x1b[K" + "\r\n" +
		"done" + "\x1b[K" + "\r\n" + "\x1b[?25l"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
	if _, open := i.Tail(); open {
		t.Fatal("a whole row left the one before it open")
	}
}

func TestAppendingToAFullRowStartsTheNextOne(t *testing.T) {
	// Appending means putting something after what is there, not squeezing it in
	// beside it — and a row with no room left would wrap, which moves the block.
	i := grid.NewInline(4, 6)
	i.Append(func(v grid.View) { v.Text(0, 0, "abcd", grid.Style{}) })
	i.Append(func(v grid.View) {
		if w, _ := v.Size(); w != 4 {
			t.Errorf("the second piece was offered %d columns, want a whole row", w)
		}
		v.Text(0, 0, "ef", grid.Style{})
	})

	got := inline(t, i, grid.Cursor{}, nil)
	want := "\x1b[0m" + "\r" +
		"abcd" + "\r\n" +
		"ef" + "\x1b[K" + "\r\n" + "\x1b[?25l"
	if got != want {
		t.Fatalf("frame  = %q\nwant   = %q", got, want)
	}
}

func TestWhatIsAppendedIsOfferedWhatIsLeftOfTheRow(t *testing.T) {
	// Which is what stops it running past the edge and taking the block's anchor with
	// it: a caller cannot draw outside the view it was handed.
	i := grid.NewInline(10, 6)
	i.Append(func(v grid.View) { v.Text(0, 0, "abc", grid.Style{}) })
	i.Append(func(v grid.View) {
		if w, _ := v.Size(); w != 7 {
			t.Fatalf("offered %d columns, want what is left of the row", w)
		}
	})
}

func TestBreakingARowCostsNothingAndIsNotAFrame(t *testing.T) {
	// The row was published with the block underneath it already, so ending it is only
	// a matter of not carrying it on.
	i := grid.NewInline(10, 6)
	i.Append(func(v grid.View) { v.Text(0, 0, "Hel", grid.Style{}) })
	inline(t, i, grid.Cursor{}, lines("prompt"))

	i.Break()
	if got := inline(t, i, grid.Cursor{}, lines("prompt")); got != "" {
		t.Fatalf("ending a row wrote %q", got)
	}
	i.Append(func(v grid.View) { v.Text(0, 0, "lo", grid.Style{}) })
	got := inline(t, i, grid.Cursor{}, lines("prompt"))
	if strings.Contains(got, "\x1b[1A") {
		t.Fatalf("frame = %q, want the next piece on a row of its own", got)
	}
}

func TestAppendingNothingCostsNoRow(t *testing.T) {
	// A chunk that came to nothing is not a blank line: it is nothing.
	i := grid.NewInline(10, 4)
	inline(t, i, grid.Cursor{}, lines("ab"))
	i.Append(func(grid.View) {})
	if got := inline(t, i, grid.Cursor{}, lines("ab")); got != "" {
		t.Fatalf("appending nothing wrote %q", got)
	}
	if _, open := i.Tail(); open {
		t.Fatal("appending nothing opened a row")
	}
}

func TestPrintingNothingIsNotAFrame(t *testing.T) {
	i := grid.NewInline(10, 4)
	inline(t, i, grid.Cursor{}, lines("ab"))
	i.Print(content(0, func(v grid.View) { v.Text(0, 0, "never", grid.Style{}) }))
	if got := inline(t, i, grid.Cursor{}, lines("ab")); got != "" {
		t.Fatalf("printing no rows wrote %q", got)
	}
}

func TestPrintedRowsAreClippedToTheBlocksWidth(t *testing.T) {
	// A printed row wider than the terminal would wrap, and a wrap moves everything
	// below it — including the block, whose position is counted in rows.
	i := grid.NewInline(6, 4)
	i.Print(content(1, func(v grid.View) { v.Text(0, 0, "far too long", grid.Style{}) }))
	got := inline(t, i, grid.Cursor{}, nil)
	if !strings.Contains(got, "far to"+"\r\n") {
		t.Fatalf("frame = %q, want the printed row clipped to six columns", got)
	}
	if strings.Contains(got, "far to"+"\x1b[K") {
		t.Fatalf("frame = %q erases the rightmost cell after filling the row", got)
	}
	if strings.Contains(got, "long") {
		t.Fatalf("frame = %q, want nothing past the block's width", got)
	}
}

func TestAnInlineBlockCannotOutgrowTheRoomItWasGiven(t *testing.T) {
	// A terminal shorter than the interface is the ordinary case for a small window,
	// and a block taller than the screen has no top to climb back to.
	i := grid.NewInline(10, 2)
	got := inline(t, i, grid.Cursor{}, lines("one", "two", "three", "four"))
	if strings.Contains(got, "three") || strings.Contains(got, "four") {
		t.Fatalf("frame = %q, want nothing past the two rows there was room for", got)
	}
	if n := strings.Count(got, "\r\n"); n != 1 {
		t.Fatalf("frame = %q, want one row break for two rows", got)
	}
}

func TestAResizeRewritesTheWholeBlock(t *testing.T) {
	// The terminal may have reflowed what is above the block, so nothing about what it
	// is showing can be assumed.
	i := grid.NewInline(10, 4)
	inline(t, i, grid.Cursor{}, lines("ab", "cd"))
	i.Resize(12, 4)
	got := inline(t, i, grid.Cursor{}, lines("ab", "cd"))
	if !strings.Contains(got, "ab") || !strings.Contains(got, "cd") {
		t.Fatalf("frame = %q, want the whole block rewritten after a resize", got)
	}
}

func TestFinishLeavesTheCursorBelowTheBlock(t *testing.T) {
	// So the shell's next prompt lands under the interface instead of on top of it.
	i := grid.NewInline(10, 4)
	inline(t, i, grid.Cursor{Visible: true, Pos: image.Pt(1, 0)}, lines("ab", "cd"))

	var buf bytes.Buffer
	if err := i.Finish(&buf); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	want := "\x1b[1B" + "\r\n" + "\x1b[0m" + "\x1b[0 q" + "\x1b[?25h"
	if got := buf.String(); got != want {
		t.Fatalf("Finish  = %q\nwant    = %q", got, want)
	}
}

func TestFinishingAnEmptyBlockJustHandsTheCursorBack(t *testing.T) {
	// Nothing was drawn, so there is nothing to step past — and a newline for nothing
	// is a blank line the user did not ask for.
	i := grid.NewInline(10, 4)
	var buf bytes.Buffer
	if err := i.Finish(&buf); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got := buf.String(); got != "\x1b[0m"+"\x1b[0 q"+"\x1b[?25h" {
		t.Fatalf("Finish = %q", got)
	}
}

func TestAFailedInlineWriteKeepsWhatWasPrinted(t *testing.T) {
	// Output the caller asked for is worth writing twice and not worth losing.
	i := grid.NewInline(10, 4)
	i.Print(content(1, func(v grid.View) { v.Text(0, 0, "said", grid.Style{}) }))

	v := i.Frame()
	v.Text(0, 0, "prompt", grid.Style{})
	if err := i.Flush(failing{}); err == nil {
		t.Fatal("a failed write was reported as success")
	}
	if got := inline(t, i, grid.Cursor{}, lines("prompt")); !strings.Contains(got, "said") {
		t.Fatalf("frame = %q, want the printed row written again", got)
	}
}

// failing is a writer that never accepts anything.
type failing struct{}

func (failing) Write([]byte) (int, error) { return 0, errNope }

var errNope = errors.New("no")

func TestInlineStyleAndLinksSurviveARow(t *testing.T) {
	// Rows are printed as self-contained text, because a printed row has to keep
	// looking right with no screen behind it to restate anything.
	i := grid.NewInline(20, 3)
	got := inline(t, i, grid.Cursor{}, func(v grid.View) {
		v.Text(0, 0, "plain", grid.Style{})
		v.Text(5, 0, "loud", grid.Style{Attr: grid.Bold})
	})
	if !strings.Contains(got, "plain") || !strings.Contains(got, "loud") {
		t.Fatalf("frame = %q", got)
	}
	if !strings.Contains(got, "\x1b[0;1m") {
		t.Fatalf("frame = %q, want the bold run stated", got)
	}
	if !strings.Contains(got, "loud"+"\x1b[0m") {
		t.Fatalf("frame = %q, want the row to end at the default style", got)
	}
}

func TestInlineSizeIsTheRoomItWasGiven(t *testing.T) {
	i := grid.NewInline(30, 5)
	if w, h := i.Size(); w != 30 || h != 5 {
		t.Fatalf("Size = %d, %d", w, h)
	}
	i.Resize(20, 3)
	if w, h := i.Size(); w != 20 || h != 3 {
		t.Fatalf("Size after a resize = %d, %d", w, h)
	}
}

func TestAnInlineBlockWithNoRoomDrawsNothing(t *testing.T) {
	// None of this may panic: a layout collapses before it disappears.
	for _, size := range [][2]int{{0, 0}, {0, 4}, {4, 0}} {
		i := grid.NewInline(size[0], size[1])
		inline(t, i, grid.Cursor{}, lines("ab", "cd"))
		i.Print(content(2, func(v grid.View) { v.Text(0, 0, "said", grid.Style{}) }))
		inline(t, i, grid.Cursor{}, nil)
		var buf bytes.Buffer
		if err := i.Finish(&buf); err != nil {
			t.Fatalf("Finish: %v", err)
		}
	}
}

func TestFullWidthInlineRowsKeepTheirRightmostCell(t *testing.T) {
	// A row that filled the last column leaves the terminal in its pending-wrap state,
	// where erase-to-end would erase that last cell. Carriage return both preserves the
	// cell and cancels pending wrap before the newline moves to the next row.
	i := grid.NewInline(4, 3)
	got := inline(t, i, grid.Cursor{}, lines("abcd", "efgh"))
	if strings.Contains(got, "abcd"+"\x1b[K") || strings.Contains(got, "efgh"+"\x1b[K") {
		t.Fatalf("frame = %q erases a rightmost cell after filling its row", got)
	}
	if !strings.Contains(got, "abcd"+"\r\n") {
		t.Fatalf("frame = %q, want the full row ended by carriage return and newline", got)
	}
	for at := range got {
		if got[at] == '\n' && (at == 0 || got[at-1] != '\r') {
			t.Fatalf("frame = %q has a newline at %d with no carriage return before it", got, at)
		}
	}
}

func TestFullWidthInlineRowsUseCellGeometryWithStylesAndLinks(t *testing.T) {
	// ANSI styling and hyperlink wrappers make the encoded byte string much longer
	// than the cells it paints. Margin detection therefore belongs to the surface row,
	// before encoding, and must stay true when presentation metadata is present.
	i := grid.NewInline(4, 2)
	got := inline(t, i, grid.Cursor{}, func(v grid.View) {
		v.Text(0, 0, "ab", grid.Style{})
		v.Text(2, 0, "cd", grid.Style{Attr: grid.Bold})
		v.Link(0, 0, 4, "https://example.test/full")
	})
	if strings.Contains(got, "\x1b[K") {
		t.Fatalf("frame = %q erases a styled, linked row that fills its cells", got)
	}
	if !strings.Contains(got, "https://example.test/full") || !strings.Contains(got, "cd") {
		t.Fatalf("frame = %q, want the complete linked and styled row", got)
	}
}
