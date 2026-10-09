---
title: Build bounded streaming output
description: Connect an ordered byte source while keeping pending work bounded and completed output releasable.
contentType: How-to
---

# Build bounded streaming output

Language: English | [简体中文](zh/streaming.md)

This guide connects an ordered background byte source to an inline interface. It
keeps pending input bounded, transforms partial content on the interface owner, and
publishes finished blocks into terminal scrollback.

Use [`examples/streaming`](https://github.com/Tangerg/oolong/tree/main/examples/streaming) as the complete implementation.
The snippets below isolate the ownership boundaries that an application must keep.

## Before you begin

Read [Compose a themeable picker](components.md) for owner-side component state. Read
[Render Markdown, code, and mathematics](content.md) when the source emits Markdown;
plain byte streams do not require an optional content module.

## Keep four lifetimes separate

A streaming interface owns four different states:

1. The source goroutine owns network, process, or model I/O
2. `program.ByteIngress` owns accepted bytes waiting for the interface
3. The interface goroutine owns components and the open transformed tail
4. The terminal owns content after `InlineRuntime.Print` commits it

```mermaid
flowchart LR
    source["source goroutine"] --> ingress["bounded ByteIngress"]
    ingress --> owner["interface owner"]
    owner --> open["open tail"]
    open --> committed["finished block"]
    committed --> terminal["terminal scrollback"]
```

Only the first edge may block. Backpressure at `ByteIngress.Write` bounds memory
without blocking the interface goroutine or imposing a limit on the runtime's
general dispatcher.

## Define a source at the byte boundary

Keep the producer independent from frames and components:

```go
type replySource func(
    ctx context.Context,
    prompt string,
    dst io.Writer,
) error
```

An HTTP response body, `exec.Cmd.Stdout`, or model stream can implement this shape.
The source must honor `ctx`, preserve byte order, and return the error that ended the
stream.

For a command source, give `exec.Cmd.Stdout` and `Stderr` the same ingress writer so
the command owns output copying and process waiting together. The run example sets
`WaitDelay` to one second: a descendant retaining an output pipe cannot keep the
source waiting indefinitely after command exit or cancellation. An incomplete
collection returns an error, including `exec.ErrWaitDelay` after successful process
exit; it is never reported as complete output.

## Start bounded ingress

Create ingress on the interface goroutine. The limit counts accepted bytes that the
owner has not consumed:

```go
ingress, err := program.NewByteIngress(program.ByteIngressConfig{
    Dispatcher: runtime.Dispatcher(),
    Limit:      64 << 10,
    Consume:    chat.accept,
})
if err != nil {
    chat.finish(err)
    return
}

ctx, cancel := context.WithCancel(context.Background())
chat.ingress = ingress
chat.cancel = cancel

go func() {
    err := chat.source(ctx, prompt, ingress)
    _ = ingress.CloseWithError(err)
}()
```

The producer closes ingress exactly once. `CloseWithError` delivers every accepted
byte before one final batch. If the interface stops first, writes return
`program.ErrStopped` and pending bytes are released.

## Transform only on the owner

The callback runs on the interface goroutine. Update components there without a
mutex:

```go
func (c *chat) accept(batch program.ByteBatch) {
    if len(batch.Data) > 0 {
        c.consumeMarkdown(string(batch.Data))
    }
    if batch.Final {
        c.finish(batch.Err)
    }
}
```

`markdown.Stream.Feed` separates stable blocks from the open tail. Append stable
blocks once. Replace only the open document while more bytes can still change it.
Call `Flush` when the final batch arrives.

An empty `Open` result replaces the existing tail too: a visible prefix can become
an invisible HTML comment. `Flush` ends an existing transcript tail even when its
result is empty; finish that block so it cannot prevent later scrollback publication.
Use the open document's presence to track its lifetime rather than a separate flag.

The stream owns its unpublished source and open cache until rendering returns.
Renderer callbacks may read `Look`; reentering `Feed`, `Open`, `Flush`, `Reset` or
`SetLook` on that same stream panics. Feed another stream or defer source changes
until the active operation returns. A renderer panic does not discard the source.

The parser keeps an unfinished list or quote in the open tail, including blank lines
between items. A long loose list can therefore have a long tail; publishing its items
independently would change numbering and nesting when chunks arrive differently.

## Publish finished ownership

Keep a bounded number of finished blocks in the live transcript for selection and
search. Commit older blocks with the transcript renderer's publication operation.
That operation measures and prints the same drawable, then releases its interactive
identity.

`headless.Transcript.Commit` retires each accepted block before invoking the callback
for the next block. That callback may only read the transcript. Mutating it, staging
layout or calling Commit again panics. An abnormal callback exit leaves its output
result unknown, so that transcript cannot be reused for mutation or publication;
earlier accepted blocks remain retired.

`grid.Inline` owns each Print, Append, Flush and Finish operation through its
callbacks. Read the canvas and draw into the supplied view; schedule further
canvas changes after the operation returns. Reentrant mutation or publication
panics, so nested output cannot replace the outer operation's scratch or anchor.

Do not rebuild published rows in application state. The terminal already owns them,
and rebuilding them would make memory and resize work grow with session age.

## Settle cancellation and failure

Cancellation is a source result, not a second delivery path:

- Cancel the source context from the interface owner
- Let the source return `context.Canceled`
- Close ingress with that result
- Settle UI state from the final `ByteBatch`
- Wait on `ByteIngress.Done` when another goroutine must release source resources

Treat a terminal write failure separately from a source failure. The source may have
completed successfully even when presentation could not finish.

## Verify the complete path

Test owner-side state with `programtest`. Add a PTY test for permanent publication,
idle output, resize, and terminal mode cleanup. The [testing guide](testing.md)
defines both boundaries. Continue with [Build a bounded agent interface](agent.md)
to add typed domain events and tool review around this byte path.
