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
			return scene.EventResult{Consumed: component.Handle(event.Input)}
		},
	})
}

func runSSHFixture(session charmssh.Session, cfg program.ScreenConfig) (err error) {
	host, err := ssh.Open(ssh.Config{
		Session:  session,
		Terminal: term.Config{Features: cfg.Terminal, AltScreen: true},
	})
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
