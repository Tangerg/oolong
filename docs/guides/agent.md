---
title: Build an agent interface
description: Let human input and automation share typed actions, availability and result lifetimes.
contentType: How-to
---

# Build an agent interface

Language: English | [简体中文](../zh/guides/agent.md)

## Start from the real application

Run `go run ./examples/agent` and read the [agent example](https://github.com/Tangerg/oolong/tree/main/examples/agent). Domain tasks, tool events and persistence remain application policy. StreamDocument owns content semantics, Transcript owns reading and retention, and the runtime owns work delivery and terminal output.

## Observe without mutating

`automation.NewDriver(DriverConfig{Root: runtime.Scene(), Owner: runtime.Dispatcher()})` schedules queries and actions onto the UI owner. Construct it after the scene is attached. `Observe` reads the existing semantic projection and does not start painting, populate hidden content or create background work.

A `scene.Query` bounds depth, node count and text. Pagination is tied to a model revision and refuses a mixed-revision continuation. Sensitive values are redacted. `Progress` separately reports model, prepared layout and written-frame progress. An observation is not proof that its model has already appeared on the terminal.

## Locate and invoke one current target

Use `driver.Locate(query, scene.Match{Key: "submit"}).Invoke(ctx, scene.ActionSubmit, nil)` when an action takes no parameters. A locator stores intent, resolves exactly one current match, and invokes it in the same owner turn. Missing, ambiguous, truncated, detached and modal-blocked targets are explicit outcomes.

Actions requiring parameters receive their concrete parameter type. Mouse, key and semantic invocations share action availability and owner behavior. Avoid editing semantic values or controller internals as a shortcut around a disabled action. Keep received task and submission identifiers when later completion matters.

## Wait for the fact you need

`Driver.Wait` subscribes while observing on the owner, then rechecks on change. Its predicate states a specific requirement: a value changed, a particular target disappeared, a task result was applied, or a written watermark reached the relevant sequence. It does not wait for every future task an application could create.

Cancellation requests, worker completion, UI delivery, publication acceptance and terminal writing remain separate facts. A cancelled view cannot revive a detached target, and a successful background operation can still have revoked UI delivery.
