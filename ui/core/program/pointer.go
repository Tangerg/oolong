package program

import (
	"errors"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// PointerSource binds synthetic intent to the completed frame observed at source
// admission. Physical reports remain on EventSource.Events. The source must use
// only this binding's capture callback, before buffering, and preserve one FIFO
// across pointer input, Events and Repaints. A zero capture must not be admitted.
// Binding and release run on the reader and must not block. A successful binding
// returns a non-nil channel and release function. Any returned release runs once
// when the reader ends and revokes further capture calls; bindings must not
// overlap. Closing the pointer channel does not end the main input stream.
type PointerSource interface {
	BindPointerInput(capture func(input.Mouse, uint64) scene.CapturedInput) (<-chan scene.CapturedInput, func(), error)
}

func bindPointerInput(root *scene.Root, source EventSource) (<-chan scene.CapturedInput, func(), error) {
	pointers, ok := source.(PointerSource)
	if !ok {
		return nil, nil, nil
	}
	stream, release, err := pointers.BindPointerInput(root.CapturePresentedInput)
	if err == nil && (stream == nil || release == nil) {
		err = errors.New("program: pointer source returned an incomplete binding")
	}
	return stream, release, err
}
