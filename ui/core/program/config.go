package program

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/internal/identity"
)

// DefaultFrameInterval coalesces bursts without delaying ordinary interactive updates.
const DefaultFrameInterval = 16 * time.Millisecond

// Config describes the session resources shared by both rendering modes. A nil
// Host opens the local terminal. A supplied host must be non-nil and retains its own lifecycle.
type Config struct {
	Terminal      term.Features
	Color         grid.Depth
	Host          Host
	FrameInterval time.Duration
	// Actions bounds ordinary pending callbacks; Resources bounds accepted tasks,
	// clocks and ingress streams. Zero selects the documented finite defaults.
	Actions   int
	Resources int
	// InputEvents and InputBytes bound the normalized-input queue. The default
	// byte budget accommodates every event emitted by the terminal decoder.
	InputEvents int
	InputBytes  int
}

// Validate rejects impossible resource policies without opening a host.
func (c Config) Validate() error {
	if c.Host != nil && identity.IsNil(c.Host) {
		return errors.New("program: supplied host is nil")
	}
	if c.FrameInterval < 0 {
		return errors.New("program: frame interval cannot be negative")
	}
	if c.Actions < 0 || c.Resources < 0 || c.InputEvents < 0 || c.InputBytes < 0 {
		return errors.New("program: capacities cannot be negative")
	}
	return nil
}

// ScreenConfig builds an attached scene after the alternate-screen host is ready.
type ScreenConfig struct {
	Config
	Build func(*Runtime) (scene.Element, error)
}

// InlineConfig adds a bounded publisher to an ordinary-screen session.
type InlineConfig struct {
	Config
	Build       func(*InlineRuntime) (scene.Element, error)
	Publication publication.Limits
}

type runConfig struct {
	Config
	build       func(*Runtime) (scene.Element, error)
	inline      bool
	publication publication.Limits
}

// The unclaimed configuration is the right to start this session. Taking it
// also releases the session handle's reference to the builder and host.
type launch struct{ config atomic.Pointer[runConfig] }

func newLaunch(cfg runConfig) *launch {
	l := &launch{}
	l.config.Store(&cfg)
	return l
}

func (l *launch) run(ctx context.Context) error {
	cfg := l.config.Swap(nil)
	if cfg == nil {
		return ErrAlreadyRun
	}
	return run(ctx, *cfg)
}

// Screen owns one alternate-screen session. Construction does no I/O and Run
// can be called only once, including after a failed build or cancelled run.
type Screen struct{ launch *launch }

// Inline owns one ordinary-screen session. Its terminal must satisfy
// [grid.Inline]'s resize contract.
type Inline struct{ launch *launch }

// NewScreen validates configuration; terminal ownership begins only at Run.
func NewScreen(cfg ScreenConfig) (*Screen, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Build == nil {
		return nil, errors.New("program: screen requires Build")
	}
	return &Screen{launch: newLaunch(runConfig{Config: cfg.Config, build: cfg.Build})}, nil
}

// NewInline validates budgets without capturing content or opening a terminal.
func NewInline(cfg InlineConfig) (*Inline, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Build == nil {
		return nil, errors.New("program: inline requires Build")
	}
	if err := cfg.Publication.Validate(); err != nil {
		return nil, err
	}
	return &Inline{launch: newLaunch(runConfig{
		Config: cfg.Config, inline: true,
		publication: cfg.Publication, build: func(rt *Runtime) (scene.Element, error) {
			return cfg.Build(&InlineRuntime{Runtime: rt})
		},
	})}, nil
}

// ErrAlreadyRun prevents one session from acquiring a second host lifetime.
var ErrAlreadyRun = errors.New("program: session has already run")

// Run acquires the host once and restores it on every exit path. Cancellation ends
// the session normally; transport and construction failures remain errors.
func (s *Screen) Run(ctx context.Context) error {
	if s == nil || s.launch == nil {
		return errors.New("program: uninitialized screen")
	}
	return s.launch.run(ctx)
}

// Run drains accepted permanent content before relinquishing the ordinary screen.
// The session cannot be restarted after either success or failure.
func (i *Inline) Run(ctx context.Context) error {
	if i == nil || i.launch == nil {
		return errors.New("program: uninitialized inline session")
	}
	return i.launch.run(ctx)
}

func (c runConfig) openHost() (hostSession, error) {
	if c.Host != nil {
		return hostSession{Host: c.Host}, nil
	}
	terminal, err := term.Open(term.Config{Features: c.Terminal, AltScreen: !c.inline})
	if err != nil {
		return hostSession{}, err
	}
	return hostSession{Host: TerminalHost(terminal), release: terminal.Close}, nil
}
