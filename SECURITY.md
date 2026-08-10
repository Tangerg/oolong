# Report a security vulnerability

Report vulnerabilities privately so a fix can be prepared before public details
make users easier to attack. Do not include exploit details in a public issue.

## Supported versions

Oolong is pre-1.0 and changes through coordinated releases. Security fixes target
the latest release train only:

| Version | Supported |
| --- | --- |
| Latest `0.x` release | Yes |
| Earlier releases | No |

Upgrade every public Oolong module together before reporting a problem that may
already be fixed.

## Send a private report

Use [GitHub private vulnerability reporting](https://github.com/Tangerg/oolong/security/advisories/new).
Include enough information to reproduce and bound the issue:

- Affected Oolong module versions
- Go version, operating system, terminal, and transport such as local PTY or SSH
- Minimal input, program, or protocol bytes that trigger the behavior
- Security impact and the boundary crossed
- Any known workaround

The report will be triaged privately. Publication, credit, and release timing will
be coordinated in the advisory after the impact and fix are understood.

## Scope

Security-sensitive areas include terminal escape handling, untrusted host geometry,
clipboard transport, SSH environment facts, subprocess handover, retained input,
and resource bounds. A normal application bug belongs in the public
[bug report form](https://github.com/Tangerg/oolong/issues/new?template=bug.yml).
