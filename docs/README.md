# Learn Oolong

Follow the learning path in order when adopting the library. Each step ends with a
runnable, tested slice and introduces one new layer without replacing the previous
one.

## Learning path

| Level | Build | Outcome |
| --- | --- | --- |
| Beginner | [Your first interface](getting-started.md) ([简体中文](getting-started.zh-CN.md)) | Run a core-only full-screen component |
| Beginner | [A themeable picker](components.md) ([简体中文](components.zh-CN.md)) | Compose headless behavior, kit appearance, layout, and input |
| Intermediate | [Markdown, code, and mathematics](content.md) ([简体中文](content.zh-CN.md)) | Use optional renderers independently and together |
| Intermediate | [Bounded streaming output](streaming.md) ([简体中文](streaming.zh-CN.md)) | Move ordered background bytes into an inline interface |
| Advanced | [A bounded agent interface](agent.md) ([简体中文](agent.zh-CN.md)) | Join streaming text, domain events, review, and scrollback ownership |
| Any level | [Interface testing](testing.md) ([简体中文](testing.zh-CN.md)) | Choose between in-process and real-PTY tests |

The [example catalog](../examples) mirrors this order. Every example has an
in-process test, and the streaming slice also proves terminal behavior on a PTY.

## Architecture and project reference

| Document | Read it to |
| --- | --- |
| [Architecture](architecture.md) ([简体中文](architecture.zh-CN.md)) | Understand normative lifetime, ownership, dependency, failure, and v1 rules |
| [Prior art](prior-art.md) ([简体中文](prior-art.zh-CN.md)) | See which ideas were adopted or rejected |
| [Brand](brand.md) ([简体中文](brand.zh-CN.md)) | Keep product language and visual direction consistent |
| [Coordinated releases](releasing.md) | Validate and tag every public module |
| [Current design](../DESIGN.md) | Inspect the implementation's present shape and known limits |
| [Completed roadmap](../ROADMAP.md) | Trace capability work to the evidence that closed it |

Package-level contracts live beside the code and render on
[`pkg.go.dev`](https://pkg.go.dev/github.com/Tangerg/oolong/core). Start with the
guide for a workflow; use package documentation when selecting exact types and
methods.
