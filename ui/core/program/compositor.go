package program

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/term"
)

// canvas is somewhere frames go: a screen of the program's own, or a block in the
// terminal's. The program drives both the same way, and the difference between them
// is entirely in how a frame reaches the wire.
type canvas interface {
	Size() (w, h int)
	Resize(w, h int)
	Invalidate()
	SetDepth(d grid.Depth)
	SetGround(g grid.Ground)
	Frame() grid.View
}

func (p *program) draw() error {
	_, err := p.present.Present(time.Now(), p.compose)
	return err
}

// compose is the only settlement point for scene geometry, the terminal baseline
// and permanent contributions. No user callback can run between admission and
// the three non-failing commits.
func (p *program) compose(full bool) (uint64, error) {
	if full {
		p.canvas.Invalidate()
	}
	prepared, err := p.root.Prepare(p.canvas.Frame())
	if err != nil {
		return 0, err
	}
	defer prepared.Abort()
	var output *grid.Output
	var batch publication.Batch
	if p.inline != nil {
		batch = p.publisher.Prepare()
		items := batch.Items()
		contents := make([]grid.Frozen, len(items))
		for at, item := range items {
			contents[at] = item.Content
		}
		output, err = p.inline.Prepare(contents...)
	} else {
		screen, ok := p.canvas.(*grid.Screen)
		if !ok {
			return 0, errors.New("program: screen canvas is missing")
		}
		output, err = screen.Prepare()
	}
	if err != nil {
		return 0, fmt.Errorf("program: construct frame: %w", err)
	}
	defer output.Abort()
	seq := uint64(0)
	if output.Len() > 0 {
		seq, err = p.queue(output.Bytes(), batch)
		if err != nil {
			return 0, err
		}
	}
	output.Commit()
	prepared.Commit(seq)
	if batch.Len() > 0 {
		p.publisher.Commit(batch, seq)
	}
	p.wrote(p.writer.Written())
	return seq, nil
}

func (p *program) wrote(sequence uint64) {
	p.present.Wrote(sequence)
	if p.root != nil {
		p.root.Wrote(sequence)
	}
	if p.publisher != nil {
		p.publisher.Written(sequence)
	}
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
func (p *program) queue(frame []byte, batch publication.Batch) (uint64, error) {
	if p.outputFailed {
		return 0, ErrDisplayFailed
	}
	// A panic or Goexit inside an external writer can follow a real write.
	// Neither the attempted batch nor the physical display is safe to replay.
	uncertain := errAdmissionInterrupted
	defer func() {
		if uncertain != nil {
			p.outputFailed = true
			p.publisher.FailAdmission(batch, uncertain)
		}
	}()
	seq, err := p.writer.Queue(frame)
	if err == nil && seq <= p.queued || err != nil && seq != 0 {
		p.publisher.Written(p.writer.Written())
		uncertain = errors.Join(fmt.Errorf("%w: got %d after %d", ErrInvalidFrameSequence, seq, p.queued), err, p.writer.Err())
		return 0, uncertain
	}
	uncertain = nil
	if err != nil {
		if fatal := p.writer.Err(); fatal != nil {
			err = fatal
		}
		if !errors.Is(err, term.ErrQueueFull) {
			p.outputFailed = true
			p.failPublications(err)
		}
		return 0, err
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
		failure := errors.Join(err, frameDrainError(p.writer, drainErr))
		p.failPublications(failure)
		return failure
	}
	// Failed frames count as drained because retrying a partly written terminal
	// stream is unsound. They are still failures, and may not have reached the event
	// loop's progress case before cancellation or Quit ended the loop.
	p.wrote(p.writer.Written())
	failure := errors.Join(err, p.writer.Err())
	if failure != nil {
		p.failPublications(failure)
	}
	p.flushNotifications()
	return failure
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
	// Shutdown may finish only after the previous frame has left the writer.
	if err := p.writer.Drain(term.DrainGrace); err != nil {
		return frameDrainError(p.writer, err)
	}
	p.wrote(p.writer.Written())
	_, err := p.compose(false)
	if err != nil {
		p.frameFailed = true
	}
	return err
}

func (p *program) finishBlock() error {
	// The final live frame can occupy all ordinary writer capacity. Cleanup must
	// wait for that frame rather than lose the cursor restoration to backpressure.
	if err := p.writer.Drain(term.DrainGrace); err != nil {
		return frameDrainError(p.writer, err)
	}
	p.wrote(p.writer.Written())
	output, err := p.inline.PrepareFinish()
	if err != nil {
		return fmt.Errorf("program: finish inline block: %w", err)
	}
	defer output.Abort()
	if output.Len() > 0 {
		if _, err := p.queue(output.Bytes(), publication.Batch{}); err != nil {
			return err
		}
	}
	output.Commit()
	return nil
}

func (p *program) repaint() error {
	if err := p.takeFailure(); err != nil {
		return err
	}
	if err := p.writer.Drain(term.DrainGrace); err != nil {
		return frameDrainError(p.writer, err)
	}
	if err := p.writer.Err(); err != nil {
		return err
	}
	p.wrote(p.writer.Written())
	p.present.RequestFull()
	if err := p.draw(); err != nil {
		return err
	}
	err := p.writer.Drain(term.DrainGrace)
	p.wrote(p.writer.Written())
	return errors.Join(err, p.writer.Err())
}

// RepaintSource optionally supplies ordered requests for a complete output frame.
// The source must deliver preceding input before delivering its repaint request.
// A claimed request receives the drawn size in cells or zero dimensions and an
// error. An interrupted repaint reports [ErrInvocationAborted] as it unwinds.
// Callbacks run on the program owner and must not block. Closing the stream only
// ends repaint requests. Run can stop before claiming a request, so callers must
// also observe their Run context or completion.
type RepaintSource interface {
	Repaints() <-chan func(width, height int, err error)
}

func frameDrainError(writer FrameWriter, drainErr error) error {
	return errors.Join(ErrFrameTimeout, drainErr, writer.Err())
}

func (p *program) failPublications(err error) {
	if p.publisher != nil {
		p.publisher.Written(p.writer.Written())
		p.publisher.Fail(err, true)
	}
}
