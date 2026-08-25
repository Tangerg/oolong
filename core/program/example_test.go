package program_test

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

// clock is a component: it draws itself, and says whether it wants an event. There is
// no base type to embed and no lifecycle to implement.
type clock struct {
	runtime *program.Runtime
	now     time.Time
}

func (c *clock) Draw(view grid.View) {
	view.Text(0, 0, c.now.Format(time.TimeOnly), grid.Style{})
	view.Text(0, 2, "q to quit", grid.Style{Attr: grid.Dim})
}

func (c *clock) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() || key.Rune != 'q' {
		return false
	}
	c.runtime.Quit()
	return true
}

// Config is what a program needs to run, and passing it to [program.Run] takes the
// terminal and gives it back on the way out — including when the component panics or
// the context is cancelled.
//
// Exactly one of Root and Inline says what to run. Nothing here starts a frame loop
// that runs regardless: a component that wants a clock asks for one, and an interface
// with nothing scheduled costs nothing.
//
// The examples in this package validate a configuration rather than running it,
// because running one would take the terminal the test is printing to.
func ExampleConfig() {
	config := program.Config{
		Root: func(runtime *program.Runtime) program.Component {
			c := &clock{runtime: runtime, now: time.Now()}
			runtime.Every(time.Second, func() { c.now = time.Now() })
			return c
		},
		// Optional terminal behaviours are requested, never assumed.
		Terminal: term.Features{Mouse: true, Focus: true},
	}
	fmt.Println(config.Validate())

	// Output:
	// <nil>
}

// Config.Inline draws in the terminal's own screen instead of taking one, with the
// session's output above it. What such an interface has finished with is printed with
// [program.InlineRuntime.Print] and belongs to the terminal from then on — scrollable,
// selectable, and still there after the program exits.
//
// Which of the two is set decides where the interface lives, so setting both is a
// configuration with no answer and Validate says so.
func ExampleConfig_inline() {
	inline := program.Config{
		Inline: func(runtime *program.InlineRuntime) program.Component {
			return &clock{runtime: runtime.Runtime, now: time.Now()}
		},
	}
	fmt.Println("inline:", inline.Validate())

	both := inline
	both.Root = func(*program.Runtime) program.Component { return nil }
	fmt.Println("both:", both.Validate() == nil)

	// Output:
	// inline: <nil>
	// both: false
}

// One goroutine draws and handles input, and state reached only from it needs no lock.
// Work that happens anywhere else — a request finishing, a file changing, a timer
// firing — comes back through a Dispatcher and runs there.
//
// This is the whole concurrency model. There is no second rule.
func ExampleRuntime_Dispatcher() {
	type page struct {
		runtime *program.Runtime
		status  string
	}

	const endpoint = "https://example.test/status"
	load := func(p *page) {
		// Taken here because this is the goroutine that owns the runtime; asking the
		// runtime for one from inside the goroutine below would be the race this
		// whole arrangement exists to remove.
		post := p.runtime.Dispatcher()
		go func() {
			response, err := http.Get(endpoint) //nolint:noctx // an example, not a client
			// Back on the program goroutine: assigning p.status here is safe, and
			// assigning it in the goroutine above would not have been.
			post.Post(func() {
				if err != nil {
					p.status = "unreachable"
					return
				}
				defer func() {
					_, _ = io.Copy(io.Discard, response.Body)
					_ = response.Body.Close()
				}()
				p.status = response.Status
			})
		}()
	}
	_ = load

	fmt.Println("work returns to the goroutine that owns the state")
	// Output:
	// work returns to the goroutine that owns the state
}
