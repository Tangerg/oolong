---
layout: home
title: Oolong
titleTemplate: false
description: A streaming-first terminal UI substrate for Go.
contentType: Landing
hero:
  name: Oolong
  text: Streaming-first terminal UI for Go
  tagline: Streams can be long. The live interface stays bounded.
  image:
    src: /logo.svg
    alt: Oolong flow mark
  actions:
    - theme: brand
      text: Build your first interface
      link: /getting-started
    - theme: alt
      text: Browse examples
      link: /examples
features:
  - title: Streamed output is the primary lifetime
    details: Finished content becomes terminal scrollback while the interactive tail stays small.
  - title: Behavior and appearance compose
    details: Use headless controllers, the polished kit, or your own design system without changing runtime ownership.
  - title: Go-shaped from the bottom up
    details: Concrete values, consumer-owned interfaces, explicit errors, and one-way dependencies keep each layer replaceable.
---

Language: English | [简体中文](/zh/)

## Learn by building

Follow the path in order when adopting Oolong. Each step produces a runnable, tested slice and adds one layer without replacing the previous one.

| Level | Build | Outcome |
| --- | --- | --- |
| Beginner | [Your first interface](getting-started.md) | Run a core-only full-screen component |
| Beginner | [A themeable picker](components.md) | Compose headless behavior, kit appearance, layout, and input |
| Intermediate | [Markdown, code, and mathematics](content.md) | Use optional renderers independently and together |
| Intermediate | [Bounded streaming output](streaming.md) | Move ordered background bytes into an inline interface |
| Advanced | [A bounded agent interface](agent.md) | Join streaming text, domain events, review, and scrollback ownership |

[Browse the example catalog](examples.md) when you prefer to learn from complete commands. Every example includes an in-process test.

## Choose the smallest useful layer

Install only the modules your application imports:

```sh
go get github.com/Tangerg/oolong/core@latest
go get github.com/Tangerg/oolong/components@latest
```

Start with `core` for the runtime, terminal, grid, input, text, and layout. Add `components` for reusable headless behavior and the default theme. Optional Markdown, highlighting, LaTeX, SSH, and PTY testing remain separate modules.

[Choose modules for your interface](modules.md) lists the dependency and release rules.

## Validate and understand the system

- [Test an interface](testing.md) at the in-process and pseudoterminal boundaries
- [Troubleshoot an application](troubleshooting.md) by symptom and ownership boundary
- [Read the architecture](architecture.md) for normative lifetime and dependency rules
- [Compare prior systems](prior-art.md) to understand adopted and rejected ideas
- [Prepare a coordinated release](releasing.md) when maintaining the repository

Package contracts render on [pkg.go.dev](https://pkg.go.dev/github.com/Tangerg/oolong/core). The [GitHub repository](https://github.com/Tangerg/oolong) contains source, examples, changelog, and contribution policy.
