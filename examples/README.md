# Examples

Ten programs, shallowest first. The focused examples are one file and meant to be
read as much as run. The agent is deliberately split by responsibility: a complex
application should prove the public APIs compose without turning into a giant `main`.

```sh
go run ./examples/hello        # from the repository root, which is a Go workspace
```

| | what it shows |
| --- | --- |
| [`hello`](hello) | The whole of the contract: draw into the space you are given, say whether you wanted an event, and stop. A box, a label, and a key count. |
| [`form`](form) | The four fields anything ever asks for, bound to variables of the program's own — and the same form asked in words when the output is a pipe rather than a terminal. |
| [`picker`](picker) | A field, a fuzzy match and a list, put together by twenty lines rather than sold as a widget. The characters that answered the query are picked out in the rows. |
| [`composer`](composer) | A product-grade prompt assembled from editor behavior: `@` completion, draft-preserving history, and large pastes collapsed into application-owned atomic chips. |
| [`files`](files) | Two panes and a keyboard that moves between them: a tree that opens and closes, a window onto something taller than the room, and a container that decides which pane an event is for. |
| [`dashboard`](dashboard) | Tabs, a table with a cursor whose header sorts it when pressed, progress bars for work with a total and a spinner for work without one. |
| [`run`](run) | Driving another program: its coloured output read back into styled text, every finished line printed into the terminal's own scrollback, and the terminal handed to `$EDITOR` and taken back. |
| [`read`](read) | An answer arriving a few characters at a time. What is certainly finished is published once and never redrawn; what is still being written is re-rendered every chunk. |
| [`streaming`](streaming) | The canonical inline shape: bounded background ingress, an open markdown tail, a selectable recent transcript, stable publication to terminal scrollback, approval, cancellation, failure, resize, and real-PTY proof. |
| [`agent`](agent) | A complete mock coding-agent session: streamed markdown, bounded live history, a run plan, command completion, a blocking tool approval with a diff, application-level cancellation, and no clock after the run settles. |

## Two things they are all doing

**Nothing draws itself twice.** A program says what to run and where — a screen of
its own, or a block in the terminal's own screen with finished output printed above
it — and the runtime decides when a frame is worth drawing. An interface with nothing
happening costs nothing.

**Every one of them is testable without a terminal.** Beside each program is a test
that starts it over the public [`programtest`](../core/programtest) host, types at it,
and reads what reached the screen. The base host implements only the three required
`program.Host` methods; a test embeds it to add exactly the optional capabilities it
needs to observe. This is a supported application boundary, not an examples-only
trick.
