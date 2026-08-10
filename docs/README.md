# Oolong documentation

This directory separates learning material from architecture records. Start with a
tutorial, then use the task guide that matches the interface you are building.

| Type | Document | Use it to |
| --- | --- | --- |
| Tutorial | [Build your first interface](getting-started.md) ([简体中文](getting-started.zh-CN.md)) | Create and run a core-only application |
| How-to | [Build bounded streaming output](streaming.md) ([简体中文](streaming.zh-CN.md)) | Move ordered background bytes into an inline interface |
| How-to | [Test an interface](testing.md) ([简体中文](testing.zh-CN.md)) | Choose between in-process and real-PTY tests |
| How-to | [Prepare a coordinated release](releasing.md) | Validate and tag every public module |
| Conceptual | [Architecture](architecture.md) ([简体中文](architecture.zh-CN.md)) | Understand output lifetime, ownership, composition, and dependency rules |
| Conceptual | [Prior art](prior-art.md) ([简体中文](prior-art.zh-CN.md)) | See which ideas were adopted or rejected |
| Reference | [Brand](brand.md) ([简体中文](brand.zh-CN.md)) | Keep product language and visual direction consistent |

The [example catalog](../examples) maps runnable programs to the public APIs they
exercise. [DESIGN.md](../DESIGN.md) describes the current implementation;
[ROADMAP.md](../ROADMAP.md) records completed capability work.
