package program

import (
	"errors"

	"github.com/Tangerg/oolong/core/term"
)

// RepaintSource optionally supplies ordered requests for a complete output frame.
// The source must deliver preceding input before delivering its repaint request.
// Each callback runs on the program owner after that frame has settled, with nil
// only when output succeeded. Callbacks must not block. A closed stream ends these
// requests without ending ordinary input. Requests not received before Run ends
// are abandoned; callers must also observe their Run context or completion.
type RepaintSource interface{ Repaints() <-chan func(error) }

func (p *program) repaint() error {
	if err := p.writer.Drain(term.DrainGrace); err != nil {
		return frameDrainError(p.writer, err)
	}
	if err := p.writer.Err(); err != nil {
		return err
	}
	p.present.Wrote(p.writer.Written())
	p.present.RequestFull()
	if err := p.draw(); err != nil {
		return err
	}
	return errors.Join(p.writer.Drain(term.DrainGrace), p.writer.Err())
}
