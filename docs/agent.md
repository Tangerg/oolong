---
title: Build a bounded agent interface
description: Assemble a bounded streaming agent CLI with reviewable tool calls and terminal scrollback.
contentType: Tutorial
---

# Build a bounded agent interface

Language: English | [简体中文](zh/agent.md)

This advanced tutorial assembles a streaming agent CLI without moving model, tool,
or product grammar into the framework. The resulting interface keeps ordered text
bounded, routes discrete domain events on the owner goroutine, reviews tool calls,
and releases finished history to terminal scrollback.

Complete implementation: [`examples/agent`](https://github.com/Tangerg/oolong/tree/main/examples/agent)

## Before you begin

Read these guides in order:

1. [Compose a themeable picker](components.md) for headless ownership
2. [Render Markdown, code, and mathematics](content.md) for passive content
3. [Build bounded streaming output](streaming.md) for byte ingress and publication

The example uses a deterministic mock backend. Replace it with a model or process
adapter in the application; the interface architecture stays the same.

## Define the application boundary first

Keep model concepts in the application. Continuous answer text is an `io.Writer`;
sparse transitions use named methods because they are not bytes.

```go
type Backend interface {
    Run(context.Context, string, Output) error
}

type Output interface {
    io.Writer
    Step(context.Context, StepUpdate) error
    Review(context.Context, ChangeProposal) (bool, error)
    Tool(context.Context, ToolResult) error
}
```

This boundary has no frames, components, themes, or terminal types. A backend can be
tested with an ordinary fake and can run outside a TUI.

Do not encode review requests or plan transitions inside the text stream. Bytes may
be chunked at any boundary; domain events need identity, acknowledgement, and typed
results.

## Give each lifetime an owner

An agent run crosses five ownership regions:

| State | Owner | Lifetime |
| --- | --- | --- |
| Model request and transport | Backend goroutine | One run |
| Accepted answer bytes | `program.ByteIngress` | Until owner delivery |
| Open Markdown tail | Application owner goroutine | Until the block finishes |
| Live transcript and review | Headless tree | While interactive |
| Committed rows | Terminal | Rest of the shell session |

Only the backend writes concurrently. Every component and application entity mutates
on the runtime owner goroutine.

## Model the conversation as an entity

Keep the invariant together: transcript content, scroll, selection, sticky prompts,
Markdown stream, and open block all describe one conversation lifetime.

```go
type conversation struct {
    content   headless.Transcript
    scroll    headless.Scroll
    selection headless.Selection
    sticky    headless.Sticky
    view      kit.Transcript

    stream  markdown.Stream
    open    *markdown.Doc
    openID  headless.BlockID
    hasOpen bool
}
```

Methods such as `Markdown`, `FlushMarkdown`, `Append`, `Retain`, and `Reset` belong on
this entity. A free function that changes half these fields would split the lifetime
rule across callers.

## Start one bounded run

Create ingress on the owner goroutine, then hand its writer end to the backend
adapter. The byte limit is an application policy based on expected chunking and
acceptable memory.

```go
ingress, err := program.NewByteIngress(program.ByteIngressConfig{
    Dispatcher: runtime.Dispatcher(),
    Limit:      64 << 10,
    Consume:    agent.accept,
})
if err != nil {
    agent.finishRun(err)
    return
}

ctx, cancel := context.WithCancel(context.Background())
run := &agentRun{cancel: cancel}
agent.run = run
output := &agentBridge{
    ingress: ingress, dispatch: runtime.Dispatcher(),
    owner: agent, run: run,
}
```

Start the backend in one goroutine and close ingress with the backend's terminal
error. `CloseWithError` preserves every accepted byte before the final batch.

```go
go func() {
    err := backend.Run(ctx, prompt, output)
    _ = ingress.CloseWithError(err)
}()
```

## Bridge discrete events with acknowledgement

`Dispatcher.Post` is non-blocking and intentionally general. For a sparse domain
transition, post the mutation and wait for either its acknowledgement, cancellation,
or runtime shutdown.

```go
func (b *agentBridge) post(ctx context.Context, fn func()) error {
    applied := make(chan error, 1)
    b.dispatch.Post(func() {
        if b.owner.run != b.run || ctx.Err() != nil {
            err := context.Cause(ctx)
            if err == nil {
                err = context.Canceled
            }
            applied <- err
            return
        }
        fn()
        applied <- nil
    })
    select {
    case err := <-applied:
        return err
    case <-ctx.Done():
        return context.Cause(ctx)
    case <-b.dispatch.Done():
        return program.ErrStopped
    }
}
```

The run identity rejects a late event from a cancelled backend after another run has
started. The acknowledgement also preserves ordering between earlier ingress work and
the transition that follows it.

## Turn answer chunks into stable blocks

The ingress callback runs on the owner. Feed bytes into `markdown.Stream`, finish
stable blocks once, and replace only the open document.

```go
func (a *agent) accept(batch program.ByteBatch) {
    if len(batch.Data) > 0 {
        a.conversation.Markdown(string(batch.Data))
        a.conversation.Retain(a.runtime)
    }
    if batch.Final {
        a.finishRun(batch.Err)
    }
}
```

An open block remains mutable because another chunk can change its meaning. A stable
block is marked finished and never parsed again.

## Release finished history

Keep a small interactive window for selection and search. Commit older finished
blocks through `kit.Transcript.Commit`; the same measured drawable is printed and
then removed from live ownership.

```go
const retainedBlocks = 8

func (c *conversation) Retain(printer kit.Printer) {
    finished := 0
    for i := range c.content.Len() {
        id := c.content.FirstBlock() + headless.BlockID(i)
        if !c.content.Finished(id) {
            break
        }
        finished++
    }
    if excess := finished - retainedBlocks; excess > 0 {
        c.view.Commit(printer, excess)
    }
    c.scroll.ToBottom()
}
```

Do not keep a second copy of committed rows for redraw. The terminal owns them now.
Live memory and resize work must depend on the active tail, not session age.

## Make tool review a domain handshake

A review request blocks the backend, not the interface owner:

1. The bridge posts a request to the owner
2. The owner opens a `headless.Form` inside a `headless.Stack`
3. Approval or denial sends one value on the request channel
4. The backend resumes or stops the proposed operation

```go
type reviewRequest struct {
    proposal ChangeProposal
    answer   chan bool
}

func (a *agent) answerReview(approved bool) {
    request := a.review
    if request == nil {
        return
    }
    a.review = nil
    a.reviewDialog.Dismiss()
    request.answer <- approved
}
```

Render a proposed change with `core/diff` and `kit.Diff`. Keep permission scope,
tool names, and the effect of approval in application types. They are product grammar,
not reusable terminal behavior.

## Settle every terminal path

One method should settle a run for success, cancellation, backend failure, and
runtime shutdown:

- Flush the Markdown tail
- Publish or retain every finished block according to policy
- Stop timers and progress updates
- Deny any unanswered review
- Cancel the backend context
- Clear the current run identity
- Present the final status on the owner goroutine

Treat terminal write failure separately from backend failure. A model may finish
successfully while the terminal cannot accept the final presentation.

## Test the invariants, not the mock prose

Use three boundaries:

| Harness | Prove |
| --- | --- |
| Backend unit test | Cancellation, event order, review result, short writes |
| `programtest` | Visible plan, completion, review, cancellation, final state |
| `ptytest` | Scrollback publication, resize, idle output, terminal cleanup |

Run the repository's advanced slice:

```sh
cd examples
go test ./agent
go test -update ./agent  # only when reviewing intended golden changes
```

The completed application should satisfy one decisive property: doubling a finished
session does not double the live transcript. Read [Architecture](architecture.md)
for the normative lifetime, dependency, failure, and v1 release rules behind that
property.
