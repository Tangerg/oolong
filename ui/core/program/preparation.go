package program

import "errors"

var errFrameNotReady = errors.New("program: frame preparation is pending")

// ErrRepaintBackpressure ends a preparation barrier so bounded input can advance.
// No frame was completed; the session continues with its queued input in order.
var ErrRepaintBackpressure = errors.New("program: repaint preparation blocks input")

// Size remains valid during final preparation and painting, after task admission
// has stopped. Cleanup releases the canvas; retained runtimes then return zero.
func (r *Runtime) Size() (width, height int) {
	if r != nil && r.p != nil && r.p.canvas != nil {
		return r.p.canvas.Size()
	}
	return 0, 0
}

// SetFramePreparation installs the one readiness check before scene painting.
// It runs on the UI owner and may start scoped background work, but must not
// block. Reentering rendering from the check panics. False retains the requested
// frame without polling; completion must wake the owner through a task delivery
// or Refresh. Nil removes the check. An error ends the session without repainting
// or retrying that model.
// Finalization may check readiness after workers have been cancelled.
// If still unprepared, accepted publications replace the old live block in FIFO
// order; without pending publications the old block is finished in place. The
// unprepared model is never painted during that settlement.
func (r *Runtime) SetFramePreparation(prepare func(width, height int) (bool, error)) error {
	p := r.owner()
	if p == nil {
		return ErrStopped
	}
	p.framePreparation = prepare
	r.Refresh()
	return nil
}

func (p *program) frameReady() (bool, error) {
	if p.framePreparation == nil {
		return true, nil
	}
	if p.preparingFrame {
		panic("program: frame preparation is not reentrant")
	}
	p.preparingFrame = true
	defer func() { p.preparingFrame = false }()
	width, height := p.canvas.Size()
	ready, err := p.framePreparation(width, height)
	if err != nil {
		p.frameFailed = true
	}
	return ready, err
}

type repaintRequest struct{ reply func(int, int, error) }

func (p *program) completeRepaint(err error) {
	request := p.pendingRepaint
	if request == nil {
		return
	}
	p.pendingRepaint = nil
	var width, height int
	if err == nil {
		width, height = p.canvas.Size()
	}
	if request.reply != nil {
		completed := false
		defer func() {
			if !completed {
				p.frameFailed = true
			}
		}()
		p.apply(func() bool {
			request.reply(width, height, err)
			return false
		})
		completed = true
	}
}

func (p *program) attemptRepaint() error {
	err := ErrInvocationAborted
	defer func() {
		if !p.frameFailed && errors.Is(err, errFrameNotReady) {
			return
		}
		if err != nil {
			p.frameFailed = true
		}
		p.completeRepaint(err)
	}()
	err = p.repaint()
	if !p.frameFailed && errors.Is(err, errFrameNotReady) {
		return nil
	}
	return err
}
