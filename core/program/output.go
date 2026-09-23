package program

import (
	"errors"
	"fmt"

	"github.com/Tangerg/oolong/core/term"
)

// flush hands the frame to the writer and returns the sequence it was queued under.
//
// The canvas writes into a buffer rather than straight to the terminal, because the
// write has to happen on the writer's goroutine: that is the whole reason there is one.
func (p *program) flush() (uint64, error) {
	var frame frameBuffer
	if err := p.canvas.Flush(&frame); err != nil {
		return 0, fmt.Errorf("program: construct frame: %w", err)
	}
	if len(frame.bytes) == 0 {
		return 0, nil
	}
	return p.queue(frame.bytes)
}

// queue transfers one non-empty frame and validates the watermark that now owns it.
// Keeping this edge in one method makes ordinary frames and inline settlement obey
// the same publication protocol.
//
// It is also where "no frame after the transport is known unusable" is enforced,
// rather than at each place that publishes one. Spread across call sites, that rule
// held wherever somebody remembered it and nowhere else: a host whose writer broke
// the sequence contract once would be handed the next frame anyway, into a stream
// whose position this program has just said it no longer knows.
func (p *program) queue(frame []byte) (uint64, error) {
	if p.outputFailed {
		return 0, ErrDisplayFailed
	}
	seq := p.writer.Queue(frame)
	if seq == 0 || seq <= p.queued {
		p.outputFailed = true
		return 0, fmt.Errorf("%w: got %d after %d", ErrInvalidFrameSequence, seq, p.queued)
	}
	p.queued = seq
	return seq, nil
}

// finish settles what the program leaves behind.
//
// An inline interface's last state is what stays in the terminal, so it is drawn one
// more time and the cursor is left below it — otherwise the shell's next prompt lands
// on top of what the program was showing. The last frame is drawn without asking the
// presenter: pacing is about not drawing more often than a terminal can keep up with,
// and there is no next frame to be too close to. Anything printed but not yet written
// goes out with it, because output the caller asked for must not be lost to the timing
// of the exit.
//
// A program on a screen of its own needs none of that. Giving the screen back takes
// the interface with it, which is what makes that mode simple and this one not.
//
// Both wait for the terminal to catch up, so that Run returning means what the
// program drew has been written. Without it a caller printing its own output next
// would find the program's last frame arriving in the middle of it.
func (p *program) finish() error {
	var err error
	if p.writer.Err() != nil {
		p.outputFailed = true
	}
	if p.inline != nil && !p.outputFailed {
		err = p.leaveBlock()
	}
	if drainErr := p.writer.Drain(term.DrainGrace); drainErr != nil {
		return errors.Join(err, frameDrainError(p.writer, drainErr))
	}
	// Failed frames count as drained because retrying a partly written terminal
	// stream is unsound. They are still failures, and may not have reached the event
	// loop's progress case before cancellation or Quit ended the loop.
	return errors.Join(err, p.writer.Err())
}

func frameDrainError(writer FrameWriter, drainErr error) error {
	return errors.Join(ErrFrameTimeout, drainErr, writer.Err())
}

// leaveBlock draws the interface one last time and leaves it in the terminal's own
// output, with the cursor below it.
//
// It is what the end of an inline program is made of, and it is also what handing
// the terminal to a child is made of, which is why it is not written inside either:
// both mean "this block is finished with, and whatever writes next starts on a line
// of its own". After it, the block has no position to write relative to, so the next
// frame draws wherever the cursor has ended up.
//
// A construction already known to have failed is not repeated. The pending logical
// frame never existed, so building it again would only run the application's painters
// a second time to arrive at the same failure — while the block last delivered still
// needs its cursor moved below it.
func (p *program) leaveBlock() error {
	var err error
	if !p.frameFailed {
		err = p.renderBlock()
	}
	return errors.Join(err, p.finishBlock())
}

// renderBlock builds one inline frame. It owns the frameFailed transition because
// construction is where it is discovered; no caller has to remember to record it.
func (p *program) renderBlock() error {
	p.root.Draw(p.inline.Frame())
	_, err := p.flush()
	if err != nil {
		p.frameFailed = true
	}
	return err
}

func (p *program) finishBlock() error {
	var tail frameBuffer
	if err := p.inline.Finish(&tail); err != nil {
		return fmt.Errorf("program: finish inline block: %w", err)
	}
	if len(tail.bytes) > 0 {
		if _, err := p.queue(tail.bytes); err != nil {
			return err
		}
	}
	return nil
}

// frameBuffer collects one frame's bytes.
type frameBuffer struct{ bytes []byte }

func (f *frameBuffer) Write(b []byte) (int, error) {
	f.bytes = append(f.bytes, b...)
	return len(b), nil
}
