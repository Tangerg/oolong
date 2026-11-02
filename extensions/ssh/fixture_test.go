package ssh_test

import (
	"errors"

	charmssh "charm.land/ssh"
	"github.com/Tangerg/oolong/extensions/ssh"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
)

func fixtureElement(component interface {
	Draw(view grid.View)
	Handle(event input.Event) bool
},
) scene.Element {
	return scene.NewElement(scene.ElementConfig{
		Paint: func(frame scene.Frame) { component.Draw(frame.View) },
		Handle: func(event scene.Event) scene.EventResult {
			return scene.EventResult{Consumed: component.Handle(event.Input())}
		},
	})
}

func runSSHFixture(adapter *ssh.Adapter, session charmssh.Session, cfg program.ScreenConfig) (err error) {
	terminal, err := adapter.Bind(session)
	if err != nil {
		return err
	}
	host, err := terminal.Open(term.Config{Features: cfg.Terminal, AltScreen: true})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, host.Close()) }()
	cfg.Host = host
	screen, err := program.NewScreen(cfg)
	if err != nil {
		return err
	}
	return screen.Run(session.Context())
}

func runFakeSSHFixture(session interface {
	charmssh.Session
	requestedPTY() (charmssh.Pty, bool)
}, cfg program.ScreenConfig,
) (err error) {
	adapter := new(ssh.Adapter)
	server := new(charmssh.Server)
	if installErr := server.SetOption(adapter.Install); installErr != nil {
		return installErr
	}
	if pty, ok := session.requestedPTY(); ok {
		closer, setupErr := server.PtyHandler(session.Context(), session, pty)
		if setupErr != nil {
			return setupErr
		}
		defer func() { err = errors.Join(err, closer()) }()
	}
	return runSSHFixture(adapter, session, cfg)
}
