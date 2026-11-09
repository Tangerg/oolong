package uitest

import (
	"errors"
	"sync"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/fifo"
)

// eventSource is an ordered input FIFO with one coalesced wake-up.
//
// A fixed-capacity event channel would make Send block behind work only a program
// that may not have started yet can consume. The queue has no arbitrary burst limit;
// tests choose how many events they retain by how many they send.
type eventSource struct {
	mu      sync.Mutex
	queue   fifo.Queue[hostEvent]
	closed  bool
	pointer *pointerBinding

	events   chan input.Event
	repaints chan func(int, int, error)
	wake     chan struct{}
	done     chan struct{}
	ended    chan struct{}
}

func newEventSource() *eventSource {
	source := &eventSource{
		events:   make(chan input.Event),
		repaints: make(chan func(int, int, error)),
		wake:     make(chan struct{}, 1),
		done:     make(chan struct{}),
		ended:    make(chan struct{}),
	}
	go source.run()
	return source
}

type hostEvent interface {
	deliver(source *eventSource) bool
}

type physicalEvent struct{ input input.Event }

func (event physicalEvent) deliver(s *eventSource) bool {
	select {
	case s.events <- event.input:
		return true
	case <-s.done:
		return false
	}
}

type repaintRequest struct{ reply func(int, int, error) }

func (event repaintRequest) deliver(s *eventSource) bool {
	select {
	case s.repaints <- event.reply:
		return true
	case <-s.done:
		return false
	}
}

type pointerEvent struct {
	input   scene.CapturedInput
	binding *pointerBinding
}

func (event pointerEvent) deliver(s *eventSource) bool {
	select {
	case event.binding.events <- event.input:
		return true
	case <-event.binding.done:
		return true
	case <-s.done:
		return false
	}
}

type pointerBinding struct {
	capture func(input.Mouse, uint64) scene.CapturedInput
	events  chan scene.CapturedInput
	done    chan struct{}
}

func (s *eventSource) bindPointer(capture func(input.Mouse, uint64) scene.CapturedInput) (<-chan scene.CapturedInput, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, nil, program.ErrStopped
	}
	if capture == nil || s.pointer != nil {
		return nil, nil, errors.New("uitest: pointer input requires one active capture binding")
	}
	binding := &pointerBinding{capture: capture, events: make(chan scene.CapturedInput), done: make(chan struct{})}
	s.pointer = binding
	return binding.events, func() { s.releasePointer(binding) }, nil
}

func (s *eventSource) releasePointer(binding *pointerBinding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pointer != binding {
		return
	}
	s.pointer = nil
	binding.capture = nil
	close(binding.done)
	for remaining := s.queue.Len(); remaining > 0; remaining-- {
		event, _ := s.queue.Pop()
		if pointer, ok := event.(pointerEvent); !ok || pointer.binding != binding {
			s.queue.Push(event)
		}
	}
}

func (s *eventSource) postPointer(mouse input.Mouse, written func() (uint64, bool)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.pointer == nil {
		return false
	}
	sequence, known := written()
	if !known {
		return false
	}
	captured := s.pointer.capture(mouse, sequence)
	if captured.Input() == nil {
		return false
	}
	s.queue.Push(pointerEvent{input: captured, binding: s.pointer})
	s.signal()
	return true
}

func (s *eventSource) post(event input.Event) bool { return s.enqueue(physicalEvent{input: event}) }

func (s *eventSource) accepting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

func (s *eventSource) postRepaint(reply func(int, int, error)) bool {
	return s.enqueue(repaintRequest{reply: reply})
}

func (s *eventSource) enqueue(event hostEvent) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	s.queue.Push(event)
	s.mu.Unlock()
	s.signal()
	return true
}

func (s *eventSource) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *eventSource) run() {
	defer close(s.ended)
	defer close(s.events)
	defer close(s.repaints)
	for {
		event, ok := s.take()
		if ok {
			if !event.deliver(s) {
				return
			}
			continue
		}
		select {
		case <-s.wake:
		case <-s.done:
			return
		}
	}
}

func (s *eventSource) take() (hostEvent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queue.Pop()
}

func (s *eventSource) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		<-s.ended
		return
	}
	s.closed = true
	s.queue.Clear()
	close(s.done)
	binding := s.pointer
	s.mu.Unlock()
	if binding != nil {
		s.releasePointer(binding)
	}
	<-s.ended
}
